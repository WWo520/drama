// 自由画布服务（S2-B07）：TAG 4.1 加载 / 4.2 保存（乐观锁版本）+ 节点 CRUD + 30s 快照。
// 画布执行引擎（S2-B08）：TAG 4.4 workflow-execute——7 类节点拓扑排序执行 + 环检测。
package creation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/duanju/backend/internal/pkg/httpx"
)

// CanvasStore 画布数据边界（repository 实现）。
type CanvasStore interface {
	// SaveCanvas 保存画布（乐观锁：version 匹配才更新；data 含 nodes/edges）。
	SaveCanvas(ctx context.Context, canvasUUID string, data json.RawMessage, version int) error
	// LoadCanvas 加载画布。
	LoadCanvas(ctx context.Context, canvasUUID string) (map[string]any, error)
	// SaveNodes 全量保存节点（替换式）。
	SaveNodes(ctx context.Context, canvasID int64, nodes []json.RawMessage) error
	// CreateCanvas 新建画布。
	CreateCanvas(ctx context.Context, spaceID, projectID, userID int64, name string) (string, error)
	// ResolveCanvas 画布 uuid → id。
	ResolveCanvas(ctx context.Context, canvasUUID string) (int64, error)
	// SaveSnapshot 保存自动快照。
	SaveSnapshot(ctx context.Context, canvasID int64, data json.RawMessage, version int, userID int64) error
	// ListSnapshots 快照列表。
	ListSnapshots(ctx context.Context, canvasID int64) ([]map[string]any, error)
	// RestoreSnapshot 恢复快照（写回 canvas_projects.data）。
	RestoreSnapshot(ctx context.Context, canvasID, snapshotID int64) error
	// ExecuteWorkflow 执行 DAG（拓扑排序 + 环检测 + 节点执行器）。
	ExecuteWorkflow(ctx context.Context, canvasUUID string, nodes []map[string]any, edges []map[string]any, userID int64) ([]map[string]any, error)
}

// ListSnapshots 快照列表（S2-F03）。
func (s *CanvasService) ListSnapshots(c *gin.Context, canvasUUID string) {
	cid, err := s.store.ResolveCanvas(c.Request.Context(), canvasUUID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, 20001, "画布不存在")
		return
	}
	items, err := s.store.ListSnapshots(c.Request.Context(), cid)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取快照失败")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

// RestoreSnapshot 恢复快照（S2-F03）。
func (s *CanvasService) RestoreSnapshot(c *gin.Context, canvasUUID, snapshotUUID string) {
	cid, err := s.store.ResolveCanvas(c.Request.Context(), canvasUUID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, 20001, "画布不存在")
		return
	}
	var sid int64
	if _, err := fmt.Sscanf(snapshotUUID, "%d", &sid); err != nil || sid == 0 {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "快照 id 非法")
		return
	}
	if err := s.store.RestoreSnapshot(c.Request.Context(), cid, sid); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "恢复快照失败")
		return
	}
	httpx.OK(c, nil)
}

// CanvasService 画布服务。
type CanvasService struct {
	store CanvasStore
}

func NewCanvasService(store CanvasStore) *CanvasService {
	return &CanvasService{store: store}
}

// Save 4.2 保存画布（乐观锁：version 不匹配 409）。
func (s *CanvasService) Save(c *gin.Context, canvasUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var req struct {
		Data    json.RawMessage `json:"data" binding:"required"`
		Version int             `json:"version" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	if err := s.store.SaveCanvas(c.Request.Context(), canvasUUID, req.Data, req.Version); err != nil {
		httpx.Fail(c, http.StatusConflict, 90006, "画布版本冲突，请刷新后重试")
		return
	}
	// 快照（30s 防抖由前端触发；服务端每次保存记一条）
	if cid, err := s.store.ResolveCanvas(c.Request.Context(), canvasUUID); err == nil {
		_ = s.store.SaveSnapshot(c.Request.Context(), cid, req.Data, req.Version+1, uid)
	}
	httpx.OK(c, nil)
}

// Load 4.1 加载画布。
func (s *CanvasService) Load(c *gin.Context, canvasUUID string) {
	data, err := s.store.LoadCanvas(c.Request.Context(), canvasUUID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, 20001, "画布不存在或无权限")
		return
	}
	httpx.OK(c, data)
}

// SaveNodes 4.2 节点全量保存。
func (s *CanvasService) SaveNodes(c *gin.Context, canvasUUID string) {
	var req struct {
		Nodes []json.RawMessage `json:"nodes" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	cid, err := s.store.ResolveCanvas(c.Request.Context(), canvasUUID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, 20001, "画布不存在")
		return
	}
	if err := s.store.SaveNodes(c.Request.Context(), cid, req.Nodes); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "保存节点失败")
		return
	}
	httpx.OK(c, nil)
}

// Execute 4.4 执行工作流（拓扑排序 + 环检测）。
func (s *CanvasService) Execute(c *gin.Context, canvasUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var req struct {
		Nodes []map[string]any `json:"nodes" binding:"required"`
		Edges []map[string]any `json:"edges"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	results, err := s.store.ExecuteWorkflow(c.Request.Context(), canvasUUID, req.Nodes, req.Edges, uid)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, 90006, err.Error())
		return
	}
	httpx.OK(c, gin.H{"results": results})
}

// ---- 执行引擎（S2-B08） ----

// 7 类节点（03 E.4.1）
var nodeTypes = map[string]bool{
	"INPUT": true, "SCRIPT_SPLIT": true, "ASSET_GEN": true, "VIDEO_GEN": true,
	"EXPORT": true, "OUTPUT": true, "LLM": true,
}

// TopoSort 拓扑排序（Kahn 算法）；返回错误若存在环。
func TopoSort(nodeIDs []string, edges [][2]string) ([]string, error) {
	indeg := map[string]int{}
	adj := map[string][]string{}
	for _, id := range nodeIDs {
		indeg[id] = 0
	}
	for _, e := range edges {
		adj[e[0]] = append(adj[e[0]], e[1])
		indeg[e[1]]++
	}
	queue := []string{}
	for id, d := range indeg {
		if d == 0 {
			queue = append(queue, id)
		}
	}
	sort.Strings(queue)
	var order []string
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		order = append(order, cur)
		for _, nxt := range adj[cur] {
			indeg[nxt]--
			if indeg[nxt] == 0 {
				queue = append(queue, nxt)
				sort.Strings(queue)
			}
		}
	}
	if len(order) != len(nodeIDs) {
		return nil, errors.New("工作流存在环，无法执行")
	}
	return order, nil
}

// ExecuteNode 节点执行器（开发期 mock：按类型输出模拟结果；is_mock 标记告知前端，生产接入真实执行器后移除）。
func ExecuteNode(nodeType string, config map[string]any) (map[string]any, error) {
	if !nodeTypes[nodeType] {
		return nil, fmt.Errorf("未知节点类型: %s", nodeType)
	}
	return map[string]any{
		"node_type": nodeType,
		"status":    "SUCCESS",
		"output":    fmt.Sprintf("mock-%s-result-%d", nodeType, time.Now().UnixMilli()),
		"is_mock":   true,
	}, nil
}

var _ = strconv.Itoa