package httpx

import (
	"encoding/json"
	"strings"
	"testing"
)

type diagAsset struct {
	Name string `json:"name"`
}

type diagShot struct {
	ShotID int64       `json:"shot_id"`
	Email  string      `json:"email,omitempty"`
	Assets []diagAsset `json:"linked_assets,omitempty"`
}

// TestMaskSensitiveKeepsStructSliceFields 回归：脱敏反射不得吞掉 struct DTO 的切片字段。
// 历史缺陷：Slice 分支返回 []any，Struct 分支 AssignableTo 检查失败 → Set 跳过 →
// linked_assets 等字段静默丢失（ListBeatShots 空资产根因，2026-08-26）。
func TestMaskSensitiveKeepsStructSliceFields(t *testing.T) {
	shot := diagShot{
		ShotID: 7,
		Email:  "user@example.com",
		Assets: []diagAsset{{Name: "主角"}, {Name: "办公室"}},
	}
	masked := maskSensitive(shot)
	b, err := json.Marshal(masked)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, `"linked_assets":[{"name":"主角"},{"name":"办公室"}]`) {
		t.Fatalf("struct slice 字段被吞: %s", s)
	}
	if !strings.Contains(s, `"email":"u***@example.com"`) {
		t.Fatalf("email 未脱敏: %s", s)
	}
	// 顶层 slice（gin.H 值路径）也应保留元素 struct
	slice := []diagAsset{{Name: "道具"}}
	b2, err := json.Marshal(maskSensitive(slice))
	if err != nil {
		t.Fatalf("marshal slice: %v", err)
	}
	if string(b2) != `[{"name":"道具"}]` {
		t.Fatalf("顶层 slice 序列化异常: %s", b2)
	}
}
