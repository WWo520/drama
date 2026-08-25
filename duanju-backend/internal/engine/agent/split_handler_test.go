// SplitHandler 测试：坏 JSON 自动重试 + 结构校验 + 双 LLM 流程（S1-B05 验收）。
package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/duanju/backend/internal/engine/model"
	"github.com/duanju/backend/internal/engine/task"
)

// fakeLLM 可配置输出序列（按调用次数返回）。
type fakeLLM struct {
	responses []string // 按顺序返回（超出用最后一个）
	errs      []error
	calls     int
}

func (f *fakeLLM) Chat(_ context.Context, _ model.Capability, _ *model.ChatRequest) (*model.ChatResponse, error) {
	idx := f.calls
	f.calls++
	if idx < len(f.errs) && f.errs[idx] != nil {
		return nil, f.errs[idx]
	}
	content := "mock-echo"
	if idx < len(f.responses) {
		content = f.responses[idx]
	} else if len(f.responses) > 0 {
		content = f.responses[len(f.responses)-1]
	}
	return &model.ChatResponse{Content: content, ModelUsed: "mock"}, nil
}

// fakeSplitStore 内存记录保存。
type fakeSplitStore struct {
	saved        *SplitResult
	saveErr      error
	info         *ProjectInfo
	infoErr      error
	markedFailed bool
}

func (f *fakeSplitStore) SaveSplitResult(_ context.Context, _ int64, r *SplitResult) (*SplitStats, error) {
	if f.saveErr != nil {
		return nil, f.saveErr
	}
	f.saved = r
	return &SplitStats{Episodes: 1, Beats: 2, Groups: 4, Shots: 8}, nil
}

func (f *fakeSplitStore) GetProjectInfo(_ context.Context, _ int64) (*ProjectInfo, error) {
	if f.infoErr != nil {
		return nil, f.infoErr
	}
	return f.info, nil
}

func (f *fakeSplitStore) MarkSplitFailed(_ context.Context, _ int64) error {
	f.markedFailed = true
	return nil
}

const validJSON = `{"episodes":[{"title":"第1集","narrative_units":[{"title":"单元A","duration_sec":10,"script_original_text":"剧本","shot_groups":[{"title":"块1","duration_sec":5,"shots":[{"duration_sec":3,"shot_size":"CU","camera_movement":"SLOW_PUSH_IN","camera_angle":"EYE_LEVEL_FRONT","subject_description":"主体","background_description":"背景","local_light_description":"光影","dialogue_text":"台词","ambient_sound":"环境"}]}]}]}]}`

func taskWithParams(script string) *task.Task {
	projectID := int64(9)
	return &task.Task{ID: 1, TaskType: "SCRIPT_SPLIT", ProjectID: &projectID,
		RequestParamsJSON: []byte(`{"script_text":"` + script + `"}`)}
}

func TestSplitSuccess(t *testing.T) {
	llm := &fakeLLM{responses: []string{validJSON, "润色后提示词"}}
	store := &fakeSplitStore{}
	h := NewSplitHandler(llm, store)

	out, err := h.Execute(context.Background(), taskWithParams("剧本")) // 摘录须覆盖原文（覆盖率校验），validJSON 摘录为 "剧本"
	require.NoError(t, err)
	assert.Contains(t, string(out), `"shots":8`) // fakeStore 返回的统计
	require.NotNil(t, store.saved)
	assert.Equal(t, "第1集", store.saved.Episodes[0].Title)
	// 双 LLM：拆分 1 次 + 润色 1 镜头 = 2 次调用
	assert.Equal(t, 2, llm.calls)
}

func TestBadJSONRetries(t *testing.T) {
	// 前两次坏 JSON → 第三次成功（S1-B05 验收：坏 JSON 自动重试）
	llm := &fakeLLM{responses: []string{"不是JSON", "```json\n{broken", validJSON, "润色"}}
	store := &fakeSplitStore{}
	h := NewSplitHandler(llm, store)

	_, err := h.Execute(context.Background(), taskWithParams("剧本"))
	require.NoError(t, err)
	assert.Equal(t, 3+1, llm.calls, "2 次坏 JSON 重试 + 1 次成功 + 1 镜头润色")
}

func TestBadJSONAllRetriesFail(t *testing.T) {
	llm := &fakeLLM{responses: []string{"坏1", "坏2", "坏3", "坏4"}}
	h := NewSplitHandler(llm, &fakeSplitStore{})

	_, err := h.Execute(context.Background(), taskWithParams("剧本"))
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidJSON) || strings.Contains(err.Error(), "拆分失败"))
	assert.Equal(t, 4, llm.calls, "最多重试 4 次（maxTry=4，含首次）")
}

func TestInvalidEnumRejected(t *testing.T) {
	bad := `{"episodes":[{"title":"E","narrative_units":[{"title":"B","duration_sec":5,"script_original_text":"x","shot_groups":[{"title":"G","duration_sec":3,"shots":[{"duration_sec":2,"shot_size":"XXXX","camera_movement":"STATIC","camera_angle":"SIDE","subject_description":"a","background_description":"b","local_light_description":"c","dialogue_text":"","ambient_sound":""}]}]}]}]}`
	llm := &fakeLLM{responses: []string{bad, bad, bad}}
	h := NewSplitHandler(llm, &fakeSplitStore{})

	_, err := h.Execute(context.Background(), taskWithParams("剧本"))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidStructure)
}

func TestFencedJSONExtracted(t *testing.T) {
	fenced := "```json\n" + validJSON + "\n```"
	llm := &fakeLLM{responses: []string{fenced, "润色"}}
	store := &fakeSplitStore{}
	h := NewSplitHandler(llm, store)

	_, err := h.Execute(context.Background(), taskWithParams("剧本"))
	require.NoError(t, err)
	require.NotNil(t, store.saved)
}
