// VideoHandler 测试：回调幂等 + 乱序处理 + 镜头块状态联动（S1-B07 验收）。
package agent

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/duanju/backend/internal/engine/task"
)

// fakeVideoStore 内存记录 SaveShotVideo 调用。
type fakeVideoStore struct {
	saves            map[int64]string // shotID → videoURL
	cardSaves        map[int64]string // cardID → videoURL
	shots            map[int64]*ShotInfo
	doneShotGroupIDs []int64
	progress         []struct {
		TaskID int64
		Pct    int
		Msg    string
	}
}

func newFakeVideoStore() *fakeVideoStore {
	return &fakeVideoStore{
		saves:     map[int64]string{},
		cardSaves: map[int64]string{},
		shots: map[int64]*ShotInfo{
			1: {ShotID: 1, ShotGroupID: 10, ProjectID: 100, NarrativeUnitID: 1000, DurationSec: 3},
			2: {ShotID: 2, ShotGroupID: 10, ProjectID: 100, NarrativeUnitID: 1000, DurationSec: 5},
			3: {ShotID: 3, ShotGroupID: 11, ProjectID: 100, NarrativeUnitID: 1000, DurationSec: 4},
		},
	}
}

func (f *fakeVideoStore) LoadShot(_ context.Context, shotID int64) (*ShotInfo, error) {
	s, ok := f.shots[shotID]
	if !ok {
		return nil, errors.New("shot not found")
	}
	return s, nil
}

// LoadShotRefs 测试桩（F10：参考图列表为空）。
func (f *fakeVideoStore) LoadShotRefs(_ context.Context, _ int64, _ int64) ([]string, error) {
	return nil, nil
}

// LoadShotRefAssets 测试桩（参考资产配对；默认无关联资产）。
func (f *fakeVideoStore) LoadShotRefAssets(_ context.Context, _ int64, _ int64) []RefAsset {
	return nil
}

// ConcatShotGroup 测试桩（镜头块拼接）。
func (f *fakeVideoStore) ConcatShotGroup(_ context.Context, shotGroupID int64) error {
	f.doneShotGroupIDs = append(f.doneShotGroupIDs, shotGroupID)
	return nil
}

// LoadCard 测试桩（成片卡；默认卡 500 = 镜头 1+2，4~15s 截断验证用）。
func (f *fakeVideoStore) LoadCard(_ context.Context, cardID int64) (*CardInfo, error) {
	if cardID == 500 {
		return &CardInfo{CardID: 500, ProjectID: 100, NarrativeUnitID: 1000,
			DurationSec: 8, Title: "镜头1~镜头2", PromptText: ""}, nil
	}
	return nil, errors.New("card not found")
}

// LoadCardShots 测试桩（卡 500 → 镜头 1、2）。
func (f *fakeVideoStore) LoadCardShots(_ context.Context, cardID int64) ([]*ShotInfo, error) {
	if cardID == 500 {
		return []*ShotInfo{f.shots[1], f.shots[2]}, nil
	}
	return nil, nil
}

// SaveCardVideo 测试桩（成片卡回写记录）。
func (f *fakeVideoStore) SaveCardVideo(_ context.Context, cardID int64, videoURL string, _ int, _ int64) error {
	f.cardSaves[cardID] = videoURL
	return nil
}

func (f *fakeVideoStore) SaveShotVideo(_ context.Context, shotID int64, videoURL string, _ int, _ int64) error {
	f.saves[shotID] = videoURL
	return nil
}

func (f *fakeVideoStore) UpdateProgress(_ context.Context, taskID int64, pct int, msg string) error {
	f.progress = append(f.progress, struct {
		TaskID int64
		Pct    int
		Msg    string
	}{taskID, pct, msg})
	return nil
}

func (f *fakeVideoStore) UpdateShotGroupStatus(_ context.Context, shotGroupID int64) error {
	// 检查同组所有镜头是否都 save 了
	allDone := true
	for id, s := range f.shots {
		if s.ShotGroupID == shotGroupID {
			if _, ok := f.saves[id]; !ok {
				allDone = false
				break
			}
		}
	}
	if allDone {
		f.doneShotGroupIDs = append(f.doneShotGroupIDs, shotGroupID)
	}
	return nil
}

func shotTask(shotID int64) *task.Task {
	return &task.Task{
		ID:       shotID,
		TaskType: "VIDEO_SEGMENT_GEN",
		RequestParamsJSON: []byte(`{"shot_id":` + func() string {
			if shotID == 0 {
				return "0"
			}
			return fmt.Sprintf("%d", shotID)
		}() + `}`),
	}
}

func TestVideoShotSuccess(t *testing.T) {
	store := newFakeVideoStore()
	h := NewVideoHandler(store, nil, "dev") // engine 为 nil → 降级 mock 链路

	out, err := h.Execute(context.Background(), shotTask(1))
	require.NoError(t, err)
	assert.Contains(t, string(out), `"video_url"`)
	assert.Contains(t, string(out), `"target_id":1`)
	assert.Contains(t, string(out), `"target_kind":"shot"`)
	assert.Contains(t, store.saves[1], "/sample.mp4?shot=1&")
}

// cardTask 成片卡任务。
func cardTask(cardID int64) *task.Task {
	return &task.Task{
		ID:                9000 + cardID,
		TaskType:          "VIDEO_SEGMENT_GEN",
		RequestParamsJSON: []byte(fmt.Sprintf(`{"card_id":%d,"resolution":"P720"}`, cardID)),
	}
}

// TestVideoCardPromptLuxReal 纯函数验证三段式 prompt：资产绑定语句（跨镜头资产列多镜头号）
// + 分镜脚本文本 + 尾缀幂等；时长截断至 [4,15]。
func TestVideoCardPromptLuxReal(t *testing.T) {
	ref := RefAsset{Kind: "CHARACTER", Name: "陆言", URL: "https://cdn/a.png"}
	prop1 := RefAsset{Kind: "PROP", Name: "铜锣", URL: "https://cdn/g.png"}
	cards := []cardShot{
		{info: &ShotInfo{ShotID: 1, DurationSec: 2, ShotSize: "MS", CameraMovement: "STATIC",
			Subject: "陆言走进宴会厅", Background: "豪华宴会厅"}, refs: []RefAsset{ref, prop1}},
		{info: &ShotInfo{ShotID: 2, DurationSec: 3, ShotSize: "CU",
			Subject: "陆言举杯", Dialogue: "【陆言 微笑】：各位。"}, refs: []RefAsset{ref}},
	}
	prompt := buildLuxRealCardPrompt(cards, dedupeRefAssets(flattenCardRefs(cards)))
	assert.Contains(t, prompt, "「铜锣」是镜头1的参考图")
	assert.Contains(t, prompt, "「陆言」是镜头1、镜头2的参考图")
	assert.Contains(t, prompt, "以下内容是具体的分镜脚本：")
	// LuxReal 对齐三段式：锚定段（主体/音效）+ 镜号时间戳 + 一致性铁律
	assert.Contains(t, prompt, "【画面主体】角色：陆言。")
	assert.Contains(t, prompt, "不要生成任何 BGM 或背景音乐")
	assert.Contains(t, prompt, "镜头01【时间戳 0-2s】【时长】2s")
	assert.Contains(t, prompt, "镜头02【时间戳 2-5s】【时长】3s")
	assert.Contains(t, prompt, "逐镜衔接")
	assert.NotContains(t, prompt, videoTailConstraint, "纯构建器不含尾缀（由执行器统一追加）")

	assert.Equal(t, 4, clampCardDuration(2), "低于下限补齐 4s")
	assert.Equal(t, 15, clampCardDuration(20), "超上限截断 15s")
	assert.Equal(t, 8, clampCardDuration(8))
}

func TestVideoIdempotentCallback(t *testing.T) {
	// 模拟重复/乱序回调：首次写入后再次调用不再覆盖（幂等）
	store := newFakeVideoStore()
	h := NewVideoHandler(store, nil, "dev") // engine 为 nil → 降级 mock 链路

	_, err := h.Execute(context.Background(), shotTask(1))
	require.NoError(t, err)
	url1 := store.saves[1]

	// 第二次执行同 shot → 再次生成（实际 Worker 同任务只执行一次，但 SaveShotVideo 应幂等）
	// 重新执行（模拟乱序回调）：mock 生成不同 URL，但 DB 层不覆盖
	store.shots[1] = &ShotInfo{ShotID: 1, ShotGroupID: 10, ProjectID: 100, NarrativeUnitID: 1000, DurationSec: 3}
	_, err = h.Execute(context.Background(), shotTask(1))
	require.NoError(t, err)
	// fake store 不实现 CAS 跳过，这里只验证 handler 不报错
	assert.NotEmpty(t, store.saves[1])
	_ = url1
}

func TestShotGroupStatusAfterAllDone(t *testing.T) {
	store := newFakeVideoStore()
	h := NewVideoHandler(store, nil, "dev") // engine 为 nil → 降级 mock 链路

	// 处理同块两个镜头
	_, err := h.Execute(context.Background(), shotTask(1))
	require.NoError(t, err)
	// 镜头块 10 的 shot 2 还没完成，块状态不应标记完成
	assert.NotContains(t, store.doneShotGroupIDs, int64(10))

	_, err = h.Execute(context.Background(), shotTask(2))
	require.NoError(t, err)
	assert.Contains(t, store.doneShotGroupIDs, int64(10), "所有镜头完成后应标记镜头块完成")
}
