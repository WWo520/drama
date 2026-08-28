// packVideoCards 语义化编组单测（对齐 LuxReal 视频任务规划技能）：
// 1. 同 shot_group 镜头不拆散（剧情驱动镜头数，不再固定 2 镜/卡）
// 2. 超长组按时长 ≤10s 二次切分
// 3. 尾卡 <4s 并入前一卡
package creation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPackVideoCardsGroupBoundary(t *testing.T) {
	shots := []ShotVideoDTO{
		{ShotID: 1, DurationSec: 2, ShotGroupID: 100},
		{ShotID: 2, DurationSec: 2, ShotGroupID: 100}, // 组100 共 4s → 整组一卡（2镜）
		{ShotID: 3, DurationSec: 6, ShotGroupID: 200},
		{ShotID: 4, DurationSec: 2, ShotGroupID: 200}, // 组200 共 8s → 整组一卡（2镜）
		{ShotID: 5, DurationSec: 9, ShotGroupID: 300}, // 组300 单镜 9s ≤10s → 单镜一卡
	}
	_, _, groups := packVideoCards(shots)
	assert.Equal(t, [][]int64{{1, 2}, {3, 4}, {5}}, groups)
}

func TestPackVideoCardsOversizeGroupSplit(t *testing.T) {
	shots := []ShotVideoDTO{
		{ShotID: 1, DurationSec: 5, ShotGroupID: 100},
		{ShotID: 2, DurationSec: 5, ShotGroupID: 100},
		{ShotID: 3, DurationSec: 5, ShotGroupID: 100}, // 组内累计 10+5 → 第二段起封卡
		{ShotID: 4, DurationSec: 3, ShotGroupID: 200}, // 组200 3s <4s → 尾卡并入前一卡? 不，合并仅尾卡；此处作为最终尾卡 3s 并入前卡(5s)=8s
	}
	_, _, groups := packVideoCards(shots)
	assert.Equal(t, [][]int64{{1, 2}, {3, 4}}, groups)
}

func TestPackVideoCardsZeroGroupFallback(t *testing.T) {
	// 无组信息（历史数据）时退化为连续块逐镜成卡（每镜一组 → 卡=单镜），不 panic
	shots := []ShotVideoDTO{
		{ShotID: 1, DurationSec: 4},
		{ShotID: 2, DurationSec: 4},
	}
	globalOrder, _, groups := packVideoCards(shots)
	assert.Len(t, groups, 2)
	assert.Equal(t, 1, globalOrder[1])
	assert.Equal(t, 2, globalOrder[2])
}
