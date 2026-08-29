// diag-pipeline：按项目监测 AI 拆解 → 资产图 → 镜头参考图 → 视频生成 → 合成导出全流程耗时。
// 用法：go run ./cmd/diag-pipeline [project_id]（缺省=最近一条 SCRIPT_SPLIT 任务所属项目）。
package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// 流程阶段展示顺序（task_type → 中文名）
var stageNames = map[string]string{
	"SCRIPT_SPLIT":        "① 剧本AI拆解",
	"CHAR_IMAGE_GEN":      "② 角色图生成",
	"SCENE_IMAGE_GEN":     "② 场景图生成",
	"PROP_IMAGE_GEN":      "② 道具图生成",
	"SHOT_REF_IMAGE_GEN":  "③ 镜头参考图",
	"VIDEO_SEGMENT_GEN":   "④ 卡级视频生成",
	"VIDEO_GROUP_CONCAT":  "④.5 卡内拼接",
	"FINAL_VIDEO_COMPOSE": "⑤ 最终合成导出",
}

type stageAgg struct {
	typ        string
	total      int
	byStatus   map[string]int
	firstAt    time.Time
	lastAt     time.Time
	execMs     int64
	runningIDs []int64
}

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://duanju:duanju_dev_pwd@127.0.0.1:5432/duanju?sslmode=disable"
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "连接数据库失败:", err)
		os.Exit(1)
	}
	defer pool.Close()
	ctx := context.Background()

	// 定位项目：命令行参数 > 最近一条 SCRIPT_SPLIT 任务的所属项目
	var projectID int64
	if len(os.Args) > 1 {
		fmt.Sscan(os.Args[1], &projectID)
	} else {
		err := pool.QueryRow(ctx, `
			SELECT project_id FROM generation_tasks
			WHERE task_type='SCRIPT_SPLIT' AND project_id IS NOT NULL
			ORDER BY created_at DESC LIMIT 1`).Scan(&projectID)
		if err != nil {
			fmt.Fprintln(os.Stderr, "未找到拆分任务:", err)
			os.Exit(1)
		}
	}

	var puuid, title, pstatus string
	var createdAt, updatedAt time.Time
	if err := pool.QueryRow(ctx, `
		SELECT project_uuid::text, title, status::text, created_at, updated_at
		FROM projects WHERE id=$1`, projectID).
		Scan(&puuid, &title, &pstatus, &createdAt, &updatedAt); err != nil {
		fmt.Fprintln(os.Stderr, "项目不存在:", err)
		os.Exit(1)
	}
	fmt.Printf("═══ 项目 #%d「%s」(%s) ═══\n", projectID, title, puuid[:8])
	fmt.Printf("项目状态: %s\n\n", pstatus)

	rows, err := pool.Query(ctx, `
		SELECT task_type::text, status::text, count(*),
		       min(created_at), max(updated_at), COALESCE(sum(execution_ms),0)
		FROM generation_tasks WHERE project_id=$1
		GROUP BY 1,2`, projectID)
	if err != nil {
		fmt.Fprintln(os.Stderr, "查询任务失败:", err)
		os.Exit(1)
	}
	defer rows.Close()

	stages := map[string]*stageAgg{}
	for rows.Next() {
		var typ, status string
		var cnt int
		var first, last time.Time
		var execMs int64
		_ = rows.Scan(&typ, &status, &cnt, &first, &last, &execMs)
		s, ok := stages[typ]
		if !ok {
			s = &stageAgg{typ: typ, byStatus: map[string]int{}, firstAt: first, lastAt: last}
			stages[typ] = s
		}
		s.total += cnt
		s.byStatus[status] += cnt
		s.execMs += execMs
		if first.Before(s.firstAt) {
			s.firstAt = first
		}
		if last.After(s.lastAt) {
			s.lastAt = last
		}
	}

	// 排序：按阶段序号（未知名排最后）
	keys := make([]string, 0, len(stages))
	for k := range stages {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		oi, oki := stageNames[keys[i]]
		oj, okj := stageNames[keys[j]]
		if oki != okj {
			return oki // 有名字的排前面
		}
		return oi < oj
	})

	now := time.Now()
	fmt.Printf("%-22s %5s %-12s %-20s %-20s %-10s\n", "阶段", "任务数", "状态分布", "开始时间", "最后更新", "累计执行")
	var overallStart time.Time
	for _, k := range keys {
		s := stages[k]
		if overallStart.IsZero() || s.firstAt.Before(overallStart) {
			overallStart = s.firstAt
		}
		name := stageNames[k]
		if name == "" {
			name = k
		}
		statusStr := ""
		// 稳定顺序输出状态
		for _, st := range []string{"SUCCESS", "RUNNING", "PENDING", "FAILED_RETRYING", "FAILED_DEAD"} {
			if c := s.byStatus[st]; c > 0 {
				if statusStr != "" {
					statusStr += "+"
				}
				statusStr += fmt.Sprintf("%s×%d", shortStatus(st), c)
			}
		}
		for st, c := range s.byStatus {
			switch st {
			case "SUCCESS", "RUNNING", "PENDING", "FAILED_RETRYING", "FAILED_DEAD":
			default:
				statusStr += fmt.Sprintf(" %s×%d", st, c)
			}
		}
		lastStr := s.lastAt.Format("15:04:05")
		if hasActive(s) {
			lastStr = "进行中"
		}
		fmt.Printf("%-22s %5d %-12s %-20s %-20s %-10s\n",
			name, s.total, statusStr,
			s.firstAt.Format("15:04:05"), lastStr, dur(s.execMs))
	}
	fmt.Println()
	if !overallStart.IsZero() {
		fmt.Printf("全程用时（首次拆分任务创建 → 现在）: %s\n", dur(int64(now.Sub(overallStart)/time.Millisecond)))
	}
	// 各阶段串行耗时估算：按阶段 firstAt 排序输出间隔
	fmt.Println("\n── 阶段启动间隔（上一阶段首任务 → 本阶段首任务）──")
	starts := make([]struct {
		name string
		at   time.Time
	}, 0, len(keys))
	for _, k := range keys {
		starts = append(starts, struct {
			name string
			at   time.Time
		}{stageNames[k] + " " + k, stages[k].firstAt})
	}
	sort.Slice(starts, func(i, j int) bool { return starts[i].at.Before(starts[j].at) })
	prev := time.Time{}
	for _, st := range starts {
		if !prev.IsZero() {
			fmt.Printf("  %-28s +%-10s (%s)\n", st.name, dur(int64(st.at.Sub(prev)/time.Millisecond)), st.at.Format("15:04:05"))
		} else {
			fmt.Printf("  %-28s %-10s (%s)\n", st.name, "起点", st.at.Format("15:04:05"))
		}
		prev = st.at
	}
}

func hasActive(s *stageAgg) bool {
	return s.byStatus["RUNNING"] > 0 || s.byStatus["PENDING"] > 0
}

func shortStatus(st string) string {
	switch st {
	case "SUCCESS":
		return "完成"
	case "RUNNING":
		return "运行"
	case "PENDING":
		return "排队"
	case "FAILED_RETRYING":
		return "重试"
	case "FAILED_DEAD":
		return "失败"
	}
	return st
}

func dur(ms int64) string {
	d := time.Duration(ms) * time.Millisecond
	if d < time.Minute {
		return d.Truncate(time.Millisecond).String()
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}
