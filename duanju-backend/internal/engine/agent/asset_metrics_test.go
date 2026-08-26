package agent

import "testing"

// TestStripBodyMetrics 身体数据剥离（防"身高175"等数字被模型画进画面）。
func TestStripBodyMetrics(t *testing.T) {
	cases := []struct{ in, want string }{
		{
			"中国现代都市男性，28-30 岁，普通上班族，身高 175cm，中等身材；方形脸，五官端正",
			"中国现代都市男性，普通上班族，中等身材；方形脸，五官端正",
		},
		{
			"中国现代都市女性，25-28 岁，刑侦警察，身高 168cm，体重约 52kg，体脂偏低，肩颈线条薄",
			"中国现代都市女性，刑侦警察，体脂偏低，肩颈线条薄",
		},
		{
			"中国现代男性，30 岁左右，重案组队长，身高 183cm，体重约 78kg，肩背厚实",
			"中国现代男性，重案组队长，肩背厚实",
		},
		{
			"中国现代都市女性，26岁，普通公司文员，性格谨慎敏感。身高165cm，身材偏瘦。鹅蛋脸",
			// 剥离后残留"。，"连续标点归一为"，"
			"中国现代都市女性，普通公司文员，性格谨慎敏感，身材偏瘦。鹅蛋脸",
		},
		// 非角色数据不误伤：场景描述、视觉性数字描述
		{
			"古街夜景，红灯笼高挂，石板路湿润反光",
			"古街夜景，红灯笼高挂，石板路湿润反光",
		},
	}
	for _, c := range cases {
		got := stripBodyMetrics(c.in)
		if got != c.want {
			t.Errorf("stripBodyMetrics(%q)\n got=%q\nwant=%q", c.in, got, c.want)
		}
	}
}
