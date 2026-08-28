// polish_card.go 卡级聚合润色（提示词优化方案A）：
// 把润色单元从「单镜头独立调用」改为「一张成片卡内全部镜头一次调用」，
// 并携带相邻卡的末/首镜头摘要，让每镜头提示词自带「开场承接 + 连续动作时刻 +
// 收尾定格」三段结构与【转场】标注——解决用户实测反馈的两问题：
//  1. 跨镜头画面不连贯（此前各镜头独立润色，起止状态互相矛盾）
//  2. 无画面转变描述（旧铁律只拆镜头内动作时刻，不管镜头间衔接）
package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/duanju/backend/internal/engine/model"
)

// cardPolishSystemPrompt 卡级聚合润色系统提示词。
// 输出格式沿用 LuxReal 结构化【】逐行（video_handler 组装分镜脚本时整段嵌入【画面】主体，
// 且会剥掉头行秒数），新增【转场】行；主体描述强制三段结构。
const cardPolishSystemPrompt = `你是资深短剧分镜与视频提示词专家。把同一张成片卡内的多个镜头一次性润色为相互连贯的分镜提示词，直接输出给视频生成模型。
严格按以下格式逐镜头输出块（块的行数、顺序与输入镜头一一对应，头行秒数照抄输入值；不要 JSON、不要 markdown、不要任何解释）：

镜头 | <秒>秒
【景别】...
【运镜】...
【视角】...
【主体描述】...（100~180 字，必含三段结构：①开场承接句——从上一镜头结束瞬间的姿态/走位/视线状态自然起步；②按时间顺序拆解本镜头内 2~4 个连续动作时刻，逐一刻画肢体朝向变化、表情微表情、眼神方向、服饰外观与道具互动方式；③收尾定格句——明确本镜头结束瞬间人物的位置、朝向与情绪状态，作为下一镜头的开场起点）
【背景】...（环境层次：近景陈设/中景动线/远景氛围）
【光影】...（光源类型/方向/颜色/明暗对比/动态变化及面部落点）
【转场】<类型>：<一句话说明与本组下一镜头的衔接逻辑；最末镜头写与下一段开头的衔接>
【台词】...（保留"角色名："原文并标注情绪节奏；无台词填"无"，主体描述须以纯画面动作撑满时长）
【音效】<时序化音效>（如 <0-1s 键盘敲击声渐弱，1-2s 椅子转动吱呀>；无填"无"）

【连贯铁律——违反任意一条即为不合格输出】
1. 本卡各镜头是同一时空的连续叙事：人物位置、身体朝向、视线方向、光影分布必须逐镜衔接；第 N+1 镜的开场承接不得与第 N 镜的收尾定格矛盾；
2. 若输入含「承接上文」，首镜头的开场必须从该给定状态自然延续；若含「下文预告」，末镜头的收尾须与其顺畅衔接；
3. 拼接稳定规则：每镜头开场第 1 秒保持构图平稳（不用甩镜/快速变焦等剧烈运镜），收尾最后 1 秒动作节奏放缓、画面趋于稳定，保证与相邻视频段拼接处动量连续；
4. 动作时刻的时间戳必须为整数秒（如 0-1秒、1-2秒），禁止出现 0.5 秒等小数刻度；
5. 忠于给定的景别/运镜/视角原值，不得改写；景别决定描写层次：特写→面部微表情与皮肤质感，中景→上半身动作与姿态，全景→走位与环境关系；
6. 同场景内各镜头的光源方向与色温保持物理连续；
7. 角色/场景/道具名保持给定原文一字不改（供参考图引用匹配）；
8. 【转场】类型只能取：硬切、跳切、匹配剪辑、动作衔接、视线引导、同景延续。`

// shotSeedLine 单镜头确定性要素行（转中文；供卡级润色的用户消息组装）。
func shotSeedLine(no int, s *Shot) string {
	return fmt.Sprintf("[镜头%d] 时长%d秒。景别:%s。运镜:%s。视角:%s。\n主体描述:%s\n背景:%s\n光影:%s\n台词:%s\n环境音:%s",
		no, int(s.DurationSec), shotSizeCn(s.ShotSize), cameraMoveCn(s.CameraMovement), cameraAngleCn(s.CameraAngle),
		s.SubjectDescription, s.BackgroundDescription, s.LocalLightDescription, s.DialogueText, s.AmbientSound)
}

// neighborBrief 相邻镜头短摘要（截断主体描述+台词，作为跨卡承接上下文）。
func neighborBrief(s *Shot) string {
	sub := strings.TrimSpace(s.SubjectDescription)
	if len([]rune(sub)) > 80 {
		sub = string([]rune(sub)[:80]) + "…"
	}
	d := strings.TrimSpace(s.DialogueText)
	if d == "" {
		d = "无"
	}
	return "主体：" + sub + "｜台词：" + d
}

// buildCardPrompt 组装单卡聚合润色的用户消息（镜头要素 + 前后邻接摘要）。
func buildCardPrompt(g *ShotGroup, prevBrief, nextBrief string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "本卡共 %d 个镜头，请整体润色：\n", len(g.Shots))
	for i := range g.Shots {
		b.WriteString(shotSeedLine(i+1, &g.Shots[i]))
		b.WriteString("\n")
	}
	if prevBrief != "" {
		fmt.Fprintf(&b, "\n承接上文（上一卡最后一个镜头的结束状态）：%s\n", prevBrief)
	}
	if nextBrief != "" {
		fmt.Fprintf(&b, "\n下文预告（下一卡第一个镜头的开头内容）：%s\n", nextBrief)
	}
	return b.String()
}

// splitLuxBlocks 解析卡级润色输出：按「镜头 | N秒」头行切块。
// 返回块列表（保序）；少于输入镜头数或头行缺失时返回空切片由调用方回退。
func splitLuxBlocks(content string) []string {
	lines := strings.Split(strings.TrimSpace(content), "\n")
	blocks := make([]string, 0, 4)
	cur := strings.Builder{}
	started := false
	for _, ln := range lines {
		t := strings.TrimSpace(ln)
		isHeader := strings.HasPrefix(t, "镜头") && strings.Contains(t, "秒")
		if isHeader {
			if started {
				blocks = append(blocks, strings.TrimSpace(cur.String()))
				cur.Reset()
			}
			started = true
			cur.WriteString(t)
			continue
		}
		if started {
			cur.WriteString("\n")
			cur.WriteString(ln)
		}
	}
	if started {
		blocks = append(blocks, strings.TrimSpace(cur.String()))
	}
	for i := range blocks {
		if blocks[i] == "" {
			return nil
		}
	}
	return blocks
}

// PolishShotGroupAsCard 卡级聚合润色入口：一次 LLM 调用完成整卡所有镜头的连贯润色。
// 成功时把各块写入对应 Shot.PromptRefined 并返回 true；失败/解析不齐返回 false，
// 由调用方对受影响镜头做 luxShotPrompt 确定性回退。
func PolishShotGroupAsCard(ctx context.Context, llm LLMClient, g *ShotGroup, prevBrief, nextBrief string) bool {
	if llm == nil || len(g.Shots) == 0 {
		return false
	}
	resp, err := llm.Chat(ctx, model.CapLLM, &model.ChatRequest{
		Prompt:      buildCardPrompt(g, prevBrief, nextBrief),
		System:      cardPolishSystemPrompt,
		MaxTokens:   4096, // 2~4 镜 × ~350 token 富文本
		Temperature: 0.55,
		NoThinking:  true, // 推理模式烧光预算致 content 空，润色无需推理
	})
	if err != nil || resp.Content == "" {
		return false
	}
	blocks := splitLuxBlocks(resp.Content)
	if len(blocks) != len(g.Shots) {
		return false // 块数不齐 → 整卡回退，避免错位污染
	}
	for i := range g.Shots {
		g.Shots[i].PromptRefined = blocks[i]
	}
	return true
}
