// S1-Q01 E2E 核心链路自动化：注册→登录→项目→拆分→支付→任务→画布→广场。
// 用法：go run ./cmd/e2e-smoke（全链路断言，失败非零退出）
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const base = "http://127.0.0.1:8080"

var failures int

func call(method, path, token string, body any) (int, map[string]any) {
	var b []byte
	if body != nil {
		b, _ = json.Marshal(body)
	}
	req, _ := http.NewRequest(method, base+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func check(name string, ok bool, detail string) {
	if ok {
		fmt.Printf("  ✅ %s\n", name)
	} else {
		fmt.Printf("  ❌ %s：%s\n", name, detail)
		failures++
	}
}

func main() {
	ts := time.Now().Format("20060102150405")
	email := fmt.Sprintf("e2e%s@test.com", ts)

	fmt.Println("S1-Q01 E2E 核心链路")
	// 1. 注册
	code, reg := call("POST", "/api/v1/auth/register", "", map[string]any{
		"email": email, "password": "Passw0rd123", "nickname": "E2E",
	})
	check("注册", code == 200 && reg["code"].(float64) == 0, fmt.Sprint(reg["message"]))
	token := reg["data"].(map[string]any)["access_token"].(string)

	// 2. 登录
	code, login := call("POST", "/api/v1/auth/login", "", map[string]any{"email": email, "password": "Passw0rd123"})
	check("登录", code == 200 && login["code"].(float64) == 0, fmt.Sprint(login["message"]))

	// 3. 新建项目
	code, proj := call("POST", "/api/v1/projects", token, map[string]any{"title": "E2E项目"})
	check("创建项目", code == 200 && proj["code"].(float64) == 0, fmt.Sprint(proj["message"]))
	projID := proj["data"].(map[string]any)["project_id"].(string)

	// 4. 拆分触发（剧本字段）
	code, split := call("POST", "/api/v1/projects/"+projID+"/split-script", token, map[string]any{"script_text": "第一集：主角登场，冲突爆发。"})
	check("拆分触发", code == 200 && split["code"].(float64) == 0, fmt.Sprint(split["message"]))

	// 5. 项目状态推进（DRAFT → SCRIPT_SPLITTING）
	time.Sleep(500 * time.Millisecond)
	code, detail := call("GET", "/api/v1/projects/"+projID, token, nil)
	st := detail["data"].(map[string]any)["status"].(string)
	check("项目状态推进", st == "SCRIPT_SPLITTING" || st == "PARTIAL_ASSETS" || st == "SCRIPT_SPLIT_FAILED", "status="+st)

	// 6. 下单（POINTS）
	code, order := call("POST", "/api/v1/orders", token, map[string]any{
		"order_type": "POINTS", "package_code": "PACK_100", "amount_yuan": "9.90", "points": 100, "idempotency_key": "e2e-"+ts,
	})
	check("下单", code == 200 && order["code"].(float64) == 0, fmt.Sprint(order["message"]))
	orderNo := order["data"].(map[string]any)["order_no"].(string)

	// 7. 假签名回调拒绝（H01 验签）
	cbBody := fmt.Sprintf("app_id=2021000000000000&out_trade_no=%s&trade_no=TRADE%s&total_amount=9.90&sign_type=RSA2&timestamp=2026-08-14 10:00:00&sign=fake", orderNo, ts)
	req, _ := http.NewRequest("POST", base+"/api/v1/orders/"+orderNo+"/pay-callback", bytes.NewReader([]byte(cbBody)))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, _ := http.DefaultClient.Do(req)
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	check("假签名回调拒绝", string(raw) == "fail", string(raw))

	// 8. 任务列表
	code, tasks := call("GET", "/api/v1/tasks", token, nil)
	check("任务列表", code == 200 && tasks["code"].(float64) == 0, fmt.Sprint(tasks["message"]))

	// 9. 画布创建 + 执行
	code, canvas := call("POST", "/api/v1/canvases", token, map[string]any{"name": "E2E画布"})
	check("画布创建", code == 200 && canvas["code"].(float64) == 0, fmt.Sprint(canvas["message"]))
	cid := canvas["data"].(map[string]any)["canvas_id"].(string)
	code, exe := call("POST", "/api/v1/canvases/"+cid+"/execute", token, map[string]any{
		"nodes": []map[string]any{{"id": "n1", "node_type": "INPUT"}, {"id": "n2", "node_type": "LLM"}},
		"edges": []map[string]any{{"source": "n1", "target": "n2"}},
	})
	check("画布执行", code == 200 && exe["code"].(float64) == 0, fmt.Sprint(exe["message"]))

	// 10. 广场信息流
	code, feed := call("GET", "/api/v1/works", token, nil)
	check("广场信息流", code == 200 && feed["code"].(float64) == 0, fmt.Sprint(feed["message"]))

	fmt.Printf("\nE2E 结果：10 条链路，失败 %d\n", failures)
	if failures > 0 {
		os.Exit(1)
	}
	fmt.Println("✅ S1-Q01 全部通过")
}