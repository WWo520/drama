// 画布域补齐（S5 接口覆盖率）：画布项目级别名/节点校验/工作流执行/剧集操作台 + 厂商异步回调。
package creation

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/duanju/backend/internal/pkg/httpx"
)

// CanvasExtraService 画布补齐服务。
type CanvasExtraService struct {
	repo *Repo
}

func NewCanvasExtraService(repo *Repo) *CanvasExtraService {
	return &CanvasExtraService{repo: repo}
}

// CanvasByProject 4.1 加载画布数据（GET /canvas/:project_id —— 项目视角别名）。
func (s *CanvasExtraService) CanvasByProject(c *gin.Context, projectUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	proj, err := s.repo.GetProject(c.Request.Context(), projectUUID, uid)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "项目不存在或无权限")
		return
	}
	var canvasUUID string
	var dataJSON string
	var version int
	if err := s.repo.pool.QueryRow(c.Request.Context(), `
		SELECT canvas_uuid::text, COALESCE(data::text,'{}'), COALESCE(version,0)
		FROM canvas_projects WHERE project_id=$1 AND deleted_at IS NULL
		ORDER BY id DESC LIMIT 1`, proj.ID).Scan(&canvasUUID, &dataJSON, &version); err != nil {
		// 无画布：返回空画布
		httpx.OK(c, gin.H{"canvas_id": "", "project_id": projectUUID, "data": gin.H{"nodes": []any{}, "edges": []any{}}, "version": 0})
		return
	}
	httpx.OK(c, gin.H{"canvas_id": canvasUUID, "project_id": projectUUID, "data": dataJSON, "version": version})
}

// CanvasNodesValidate 4.3 画布节点合法性校验（POST /canvas/:project_id/nodes-validate）。
func (s *CanvasExtraService) CanvasNodesValidate(c *gin.Context, projectUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	if _, err := s.repo.GetProject(c.Request.Context(), projectUUID, uid); err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "项目不存在或无权限")
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
	problems := []map[string]any{}
	// 环检测（复用 TopoSort）
	nodeIDs := make([]string, 0, len(req.Nodes))
	for _, n := range req.Nodes {
		id := ""
		if v, ok := n["id"]; ok {
			id, _ = v.(string)
		}
		if id == "" {
			problems = append(problems, map[string]any{"type": "MISSING_ID", "message": "存在缺少 id 的节点"})
			continue
		}
		nodeIDs = append(nodeIDs, id)
	}
	edges := make([][2]string, 0, len(req.Edges))
	for _, e := range req.Edges {
		src, _ := e["source"].(string)
		dst, _ := e["target"].(string)
		if src != "" && dst != "" {
			edges = append(edges, [2]string{src, dst})
		}
	}
	if _, err := TopoSort(nodeIDs, edges); err != nil {
		problems = append(problems, map[string]any{"type": "CYCLE", "message": err.Error()})
	}
	valid := len(problems) == 0
	httpx.OK(c, gin.H{"valid": valid, "problems": problems})
}

// CanvasWorkflowExecute 4.4 执行画布工作流（POST /canvas/:project_id/workflow-execute）。
func (s *CanvasExtraService) CanvasWorkflowExecute(c *gin.Context, projectUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	proj, err := s.repo.GetProject(c.Request.Context(), projectUUID, uid)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "项目不存在或无权限")
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
	nodeIDs := make([]string, 0, len(req.Nodes))
	typeMap := map[string]string{}
	for _, n := range req.Nodes {
		id, _ := n["id"].(string)
		t, _ := n["node_type"].(string)
		nodeIDs = append(nodeIDs, id)
		typeMap[id] = t
	}
	edgeList := make([][2]string, 0, len(req.Edges))
	for _, e := range req.Edges {
		src, _ := e["source"].(string)
		dst, _ := e["target"].(string)
		edgeList = append(edgeList, [2]string{src, dst})
	}
	order, err := TopoSort(nodeIDs, edgeList)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, 90006, err.Error())
		return
	}
	results := make([]map[string]any, 0, len(order))
	for _, id := range order {
		r, err := ExecuteNode(typeMap[id], nil)
		if err != nil {
			httpx.Fail(c, http.StatusBadRequest, 90006, err.Error())
			return
		}
		r["node_id"] = id
		results = append(results, r)
	}
	_ = proj
	httpx.OK(c, gin.H{"results": results, "is_mock": true})
}

// ConsoleEpisodes 4.6 剧集操作台数据（GET /canvas/console/episodes）。
func (s *CanvasExtraService) ConsoleEpisodes(c *gin.Context) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	projectUUID := c.Query("project_id")
	proj, err := s.repo.GetProject(c.Request.Context(), projectUUID, uid)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "项目不存在或无权限")
		return
	}
	rows, err := s.repo.pool.Query(c.Request.Context(), `
		SELECT e.id, e.episode_uuid::text, e.title, e.sort_order,
		       COALESCE(e.status::text,''), COALESCE(e.narrative_unit_count,0),
		       COALESCE(e.actual_duration_sec,0)
		FROM episodes e WHERE e.project_id=$1 AND e.deleted_at IS NULL ORDER BY e.sort_order ASC`, proj.ID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取剧集失败")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id int64
		var euuid, title, status string
		var order, nuCount, dur int
		if err := rows.Scan(&id, &euuid, &title, &order, &status, &nuCount, &dur); err != nil {
			continue
		}
		// 单元完成数
		var done int
		_ = s.repo.pool.QueryRow(c.Request.Context(), `
			SELECT count(*) FROM narrative_units WHERE episode_id=$1 AND status='COMPLETED' AND deleted_at IS NULL`, id).Scan(&done)
		items = append(items, map[string]any{
			"episode_id": euuid, "title": title, "sort_order": order,
			"status": status, "narrative_unit_count": nuCount,
			"completed_unit_count": done, "actual_duration_sec": dur,
		})
	}
	// 资产总览
	var charCount, sceneCount, propCount int
	_ = s.repo.pool.QueryRow(c.Request.Context(), `SELECT count(*) FROM characters WHERE project_id=$1 AND deleted_at IS NULL`, proj.ID).Scan(&charCount)
	_ = s.repo.pool.QueryRow(c.Request.Context(), `SELECT count(*) FROM scenes WHERE project_id=$1 AND deleted_at IS NULL`, proj.ID).Scan(&sceneCount)
	_ = s.repo.pool.QueryRow(c.Request.Context(), `SELECT count(*) FROM props WHERE project_id=$1 AND deleted_at IS NULL`, proj.ID).Scan(&propCount)
	httpx.OK(c, gin.H{
		"episodes": items,
		"assets_overview": gin.H{"characters": charCount, "scenes": sceneCount, "props": propCount},
	})
}

// ModelCallback 厂商异步生成回调（POST /internal/model-callback/:provider）。
// 记录回调日志；开发期无真实厂商接入，返回 success 并记录。
func (s *CanvasExtraService) ModelCallback(c *gin.Context, provider string) {
	var req struct {
		ExternalTaskID string `json:"external_task_id"`
		Status         string `json:"status"`
		ResultURL      string `json:"result_url"`
		ErrorMsg       string `json:"error_message"`
	}
	_ = c.ShouldBindJSON(&req)
	var taskID int64
	if req.ExternalTaskID != "" {
		_ = s.repo.pool.QueryRow(c.Request.Context(), `
			SELECT id FROM generation_tasks WHERE external_task_id=$1 AND deleted_at IS NULL`, req.ExternalTaskID).Scan(&taskID)
	}
	// 记录回调日志（表结构：provider_id/task_id/external_task_id/callback_status/payload_json）
	payload := mustJSON(map[string]any{"status": req.Status, "error": req.ErrorMsg, "result_url": req.ResultURL})
	_, _ = s.repo.pool.Exec(c.Request.Context(), `
		INSERT INTO model_callback_logs (provider_id, task_id, external_task_id, callback_status, signature_valid, payload_json)
		VALUES ($1, $2, $3, $4, TRUE, $5::jsonb)`,
		0, nullInt(taskID), req.ExternalTaskID, req.Status, payload)
	// 任务状态回写（成功/失败）
	if taskID > 0 {
		if req.Status == "SUCCESS" {
			_, _ = s.repo.pool.Exec(c.Request.Context(), `
				UPDATE generation_tasks SET status='SUCCESS', completed_at=NOW(),
					result_output_json=jsonb_build_object('url',$2) WHERE id=$1`, taskID, req.ResultURL)
		} else if req.Status == "FAILED" {
			_, _ = s.repo.pool.Exec(c.Request.Context(), `
				UPDATE generation_tasks SET status='FAILED', error_message=$2, updated_at=NOW() WHERE id=$1`, taskID, req.ErrorMsg)
		}
	}
	httpx.OK(c, gin.H{"received": true})
}

func nullInt(n int64) *int64 {
	if n <= 0 {
		return nil
	}
	return &n
}

func mustJSON(v map[string]any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
