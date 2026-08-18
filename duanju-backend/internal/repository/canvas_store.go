// canvas_store 实现 creation.CanvasStore：画布 CRUD（乐观锁）+ 快照 + 工作流执行。
package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/duanju/backend/internal/domain/creation"
)

// CanvasStore 画布数据访问。
type CanvasStore struct {
	pool *pgxpool.Pool
}

func NewCanvasStore(pool *pgxpool.Pool) *CanvasStore {
	return &CanvasStore{pool: pool}
}

var _ creation.CanvasStore = (*CanvasStore)(nil)

func (s *CanvasStore) CreateCanvas(ctx context.Context, spaceID, projectID, userID int64, name string) (string, error) {
	var canvasUUID string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO canvas_projects (space_id, project_id, name, data, version, created_by)
		VALUES ($1, $2, $3, '{}', 1, $4) RETURNING canvas_uuid::text`,
		spaceID, projectID, name, userID).Scan(&canvasUUID)
	return canvasUUID, err
}

func (s *CanvasStore) ResolveCanvas(ctx context.Context, canvasUUID string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `SELECT id FROM canvas_projects WHERE canvas_uuid=$1 AND deleted_at IS NULL`, canvasUUID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("画布不存在")
	}
	return id, err
}

func (s *CanvasStore) SaveCanvas(ctx context.Context, canvasUUID string, data json.RawMessage, version int) error {
	// 乐观锁：WHERE version=$2
	tag, err := s.pool.Exec(ctx, `
		UPDATE canvas_projects SET data=$3, version=version+1, updated_at=NOW()
		WHERE canvas_uuid=$1 AND version=$2 AND deleted_at IS NULL`,
		canvasUUID, version, string(data))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("版本冲突")
	}
	return nil
}

func (s *CanvasStore) LoadCanvas(ctx context.Context, canvasUUID string) (map[string]any, error) {
	var data string
	var version int
	var name string
	err := s.pool.QueryRow(ctx, `
		SELECT name, data::text, version FROM canvas_projects
		WHERE canvas_uuid=$1 AND deleted_at IS NULL`, canvasUUID).Scan(&name, &data, &version)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	json.Unmarshal([]byte(data), &out)
	if out == nil {
		out = map[string]any{}
	}
	out["name"] = name
	out["version"] = version
	return out, nil
}

func (s *CanvasStore) SaveNodes(ctx context.Context, canvasID int64, nodes []json.RawMessage) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE canvas_nodes SET deleted_at=NOW() WHERE canvas_id=$1`, canvasID); err != nil {
		return err
	}
	for _, n := range nodes {
		var node struct {
			NodeType string          `json:"node_type"`
			PosX     float64         `json:"position_x"`
			PosY     float64         `json:"position_y"`
			Config   json.RawMessage `json:"config"`
		}
		json.Unmarshal(n, &node)
		if node.NodeType == "" {
			node.NodeType = "LLM"
		}
		if len(node.Config) == 0 {
			node.Config = json.RawMessage("{}")
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO canvas_nodes (canvas_id, node_type, position_x, position_y, config)
			VALUES ($1, $2, $3, $4, $5::jsonb)`,
			canvasID, node.NodeType, node.PosX, node.PosY, string(node.Config)); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// SaveSnapshot 保存自动快照（服务端 30s 防抖：同一画布近 30s 已有快照则跳过，防表膨胀）。
func (s *CanvasStore) SaveSnapshot(ctx context.Context, canvasID int64, data json.RawMessage, version int, userID int64) error {
	// 30s 防抖检查
	var lastSec float64
	if err := s.pool.QueryRow(ctx, `
		SELECT EXTRACT(EPOCH FROM (NOW() - created_at))
		FROM canvas_auto_save_snapshots WHERE canvas_id=$1 ORDER BY id DESC LIMIT 1`, canvasID).
		Scan(&lastSec); err == nil && lastSec < 30 {
		return nil // 防抖期内跳过
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO canvas_auto_save_snapshots (canvas_id, data, version, created_by)
		VALUES ($1, $2::jsonb, $3, $4)`, canvasID, string(data), version, userID)
	return err
}

func (s *CanvasStore) ListSnapshots(ctx context.Context, canvasID int64) ([]map[string]any, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, version, to_char(created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM canvas_auto_save_snapshots WHERE canvas_id=$1 ORDER BY id DESC LIMIT 20`, canvasID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var id, ver int64
		var created string
		rows.Scan(&id, &ver, &created)
		out = append(out, map[string]any{"snapshot_id": id, "version": ver, "created_at": created})
	}
	return out, rows.Err()
}

func (s *CanvasStore) RestoreSnapshot(ctx context.Context, canvasID, snapshotID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE canvas_projects cp SET data=s.data, version=s.version
		FROM canvas_auto_save_snapshots s
		WHERE s.id=$2 AND s.canvas_id=$1 AND cp.id=$1`, canvasID, snapshotID)
	return err
}

// ExecuteWorkflow 执行 DAG：拓扑排序 + 环检测 + 节点执行器（mock）。
func (s *CanvasStore) ExecuteWorkflow(ctx context.Context, canvasUUID string, nodes []map[string]any, edges []map[string]any, userID int64) ([]map[string]any, error) {
	// 解析节点 ID + 类型
	nodeIDs := make([]string, 0, len(nodes))
	typeMap := map[string]string{}
	for _, n := range nodes {
		id := fmt.Sprint(n["id"])
		t := fmt.Sprint(n["node_type"])
		nodeIDs = append(nodeIDs, id)
		typeMap[id] = t
	}
	edgeList := make([][2]string, 0, len(edges))
	for _, e := range edges {
		edgeList = append(edgeList, [2]string{fmt.Sprint(e["source"]), fmt.Sprint(e["target"])})
	}
	// 拓扑排序（环检测）
	order, err := creation.TopoSort(nodeIDs, edgeList)
	if err != nil {
		return nil, err
	}
	// 按拓扑序执行
	results := make([]map[string]any, 0, len(order))
	for _, id := range order {
		cfg := map[string]any{}
		for _, n := range nodes {
			if fmt.Sprint(n["id"]) == id {
				if c, ok := n["config"]; ok {
					if m, ok := c.(map[string]any); ok {
						cfg = m
					}
				}
			}
		}
		r, err := creation.ExecuteNode(typeMap[id], cfg)
		if err != nil {
			return nil, err
		}
		r["node_id"] = id
		results = append(results, r)
	}
	return results, nil
}