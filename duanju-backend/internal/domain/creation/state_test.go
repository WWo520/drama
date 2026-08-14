// 状态机测试（S1-B03 验收：非法状态转移被拒；成功主链可流转）。
package creation

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSuccessMainChain(t *testing.T) {
	// 03 附录 A.1 成功主链
	chain := []struct{ from, to string }{
		{StatusDraft, StatusScriptSplitting},
		{StatusScriptSplitting, StatusPartialAssets},
		{StatusPartialAssets, StatusAllAssetsReady},
		{StatusAllAssetsReady, StatusVideoGenerating},
		{StatusVideoGenerating, StatusPartialVideos},
		{StatusPartialVideos, StatusAllVideosReady},
		{StatusAllVideosReady, StatusExporting},
		{StatusExporting, StatusCompleted},
	}
	for _, step := range chain {
		err := ValidateTransition(step.from, step.to)
		require.NoError(t, err, "%s → %s 应合法", step.from, step.to)
	}
}

func TestInvalidTransitionRejected(t *testing.T) {
	// 非法转移：DRAFT 直接跳 COMPLETED；拆分中直接导出；已取消不可再流转
	cases := []struct{ from, to string }{
		{StatusDraft, StatusCompleted},
		{StatusScriptSplitting, StatusExporting},
		{StatusDraft, StatusAllVideosReady},
		{StatusCancelled, StatusDraft},
		{StatusCompleted, StatusScriptSplitting},
	}
	for _, c := range cases {
		err := ValidateTransition(c.from, c.to)
		require.Error(t, err, "%s → %s 应被拒", c.from, c.to)
		assert.True(t, errors.Is(err, ErrInvalidTransition))
	}
}

func TestFailureRollbackPaths(t *testing.T) {
	// 异常回滚：拆分失败回 DRAFT 可重试；导出失败回 ALL_VIDEOS_READY
	assert.NoError(t, ValidateTransition(StatusScriptSplitting, StatusScriptSplitFailed))
	assert.NoError(t, ValidateTransition(StatusScriptSplitFailed, StatusScriptSplitting))
	assert.NoError(t, ValidateTransition(StatusScriptSplitFailed, StatusDraft))
	assert.NoError(t, ValidateTransition(StatusExporting, StatusAllVideosReady))
}

func TestStepMapping(t *testing.T) {
	assert.Equal(t, StepScriptSplit, StepForStatus(StatusDraft))
	assert.Equal(t, StepAssetGen, StepForStatus(StatusAllAssetsReady))
	assert.Equal(t, StepVideoProd, StepForStatus(StatusPartialVideos))
	assert.Equal(t, StepFinalPreview, StepForStatus(StatusCompleted))
}
