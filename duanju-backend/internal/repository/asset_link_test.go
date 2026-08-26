// 资产建链匹配单测：显式引用（LuxReal shots.ref_assets）精确/包含兜底 + 文本匹配并集。
package repository

import "testing"

func TestMatchShotAssets(t *testing.T) {
	assets := []AssetLinkItem{
		{Kind: "CHARACTER", ID: 1, Name: "林晚"},
		{Kind: "SCENE", ID: 2, Name: "林晚公寓"},
		{Kind: "SCENE", ID: 3, Name: "古玩街"},
		{Kind: "PROP", ID: 4, Name: "引魂佩"},
	}

	t.Run("显式引用精确命中（场景复合名，文本匹配必失配）", func(t *testing.T) {
		s := ShotTextInput{
			Text:     "她警惕地环顾四周，公寓内部一片凌乱", // 文本无"林晚公寓"全名
			RefNames: []string{"林晚", "林晚公寓"},
		}
		got := matchShotAssets(s, assets)
		if len(got) != 2 || got[0].Name != "林晚" || got[1].Name != "林晚公寓" {
			t.Fatalf("期望命中 林晚+林晚公寓，实得 %v", got)
		}
	})

	t.Run("显式引用包含兜底（公寓 → 唯一包含的场景林晚公寓）", func(t *testing.T) {
		s := ShotTextInput{Text: "", RefNames: []string{"公寓"}}
		got := matchShotAssets(s, assets)
		if len(got) != 1 || got[0].Name != "林晚公寓" {
			t.Fatalf("期望包含兜底命中 林晚公寓，实得 %v", got)
		}
	})

	t.Run("显式引用歧义放弃（林 同时含于多资产名）", func(t *testing.T) {
		s := ShotTextInput{Text: "", RefNames: []string{"林"}}
		if got := matchShotAssets(s, assets); len(got) != 0 {
			t.Fatalf("单字引用应跳过，实得 %v", got)
		}
		s.RefNames = []string{"玩"} // "古玩街"唯一包含？不——"古玩街"含"玩"，单字跳过
		if got := matchShotAssets(s, assets); len(got) != 0 {
			t.Fatalf("单字引用应跳过，实得 %v", got)
		}
	})

	t.Run("别名道具：ref 写玉佩无命中，由 LLM 规则改写为清单名", func(t *testing.T) {
		s := ShotTextInput{Text: "玉佩特写", RefNames: []string{"玉佩"}}
		if got := matchShotAssets(s, assets); len(got) != 0 {
			t.Fatalf("玉佩非清单名且无包含关系，应不命中，实得 %v", got)
		}
	})

	t.Run("文本匹配兜底并集去重", func(t *testing.T) {
		s := ShotTextInput{Text: "林晚在古玩街漫步，手持引魂佩", RefNames: []string{"林晚"}}
		got := matchShotAssets(s, assets)
		if len(got) != 3 {
			t.Fatalf("期望 林晚(ref) + 古玩街/引魂佩(文本) 共3，实得 %v", got)
		}
	})

	t.Run("长场景名截断简称文本命中", func(t *testing.T) {
		longAssets := []AssetLinkItem{{Kind: "SCENE", ID: 10, Name: "茶水间，空间紧凑，一侧放"}}
		s := ShotTextInput{Text: "主角走过茶水间"}
		got := matchShotAssets(s, longAssets)
		if len(got) != 1 || got[0].ID != 10 {
			t.Fatalf("截断简称应命中，实得 %v", got)
		}
	})
}
