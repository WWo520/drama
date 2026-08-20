// Package agent 智能体引擎（E7）：Step1 剧本拆分 Worker（双 LLM：拆分/润色）+ JSON Schema 结构化输出。
// 对齐 04 J 章（StoryboardAgent 拆分 + PolishAgent 润色 + 技能激活）。
package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// SplitResult LLM 拆分输出的四层结构 + 集级资产（03 A.2：集 → 叙事单元 → 镜头块 → 单镜头 + 角色/场景/道具抽取）。
// 枚举值对齐 02 A3（景别/运镜/视角）——校验器强制。
// 资产粒度：仅到集级（每集自有角色/场景/道具），不下沉到叙事单元。
type SplitResult struct {
	Episodes   []EpisodeSplit   `json:"episodes"`
	Characters []CharacterSplit `json:"characters,omitempty"`
	Scenes     []SceneSplit     `json:"scenes,omitempty"`
	Props      []PropSplit      `json:"props,omitempty"`
}

// CharacterSplit 角色抽取（Step1 产出，供 Step2 角色卡片；集级粒度）。
type CharacterSplit struct {
	Code            string `json:"code"`
	Name            string `json:"name"`
	Tags            string `json:"tags,omitempty"`
	DescriptionNotes string `json:"description_notes,omitempty"`
	OriginEpisodeID int    `json:"origin_episode_id"` // 1-based 集序号
}

// SceneSplit 场景抽取（Step1 产出，供 Step2 场景卡片；集级粒度）。
type SceneSplit struct {
	Code            string `json:"code"`
	BaseName        string `json:"base_name"`
	BasePrompt      string `json:"base_prompt,omitempty"`
	OriginEpisodeID int    `json:"origin_episode_id"` // 1-based 集序号
}

// PropSplit 道具抽取（Step1 产出，供 Step2 道具卡片；集级粒度）。
type PropSplit struct {
	Code            string `json:"code"`
	Name            string `json:"name"`
	Category        string `json:"category,omitempty"` // prop_category_enum，非法值落库时归一为 OTHER
	Description     string `json:"description,omitempty"`
	OriginEpisodeID int    `json:"origin_episode_id"` // 1-based 集序号
}

type EpisodeSplit struct {
	Title           string          `json:"title"`
	NarrativeUnits  []NarrativeUnit `json:"narrative_units"`
}

type NarrativeUnit struct {
	Title             string        `json:"title"`
	DurationSec       int           `json:"duration_sec"`
	ScriptOriginalText string       `json:"script_original_text"`
	ShotGroups        []ShotGroup   `json:"shot_groups"`
}

type ShotGroup struct {
	Title      string `json:"title"`
	DurationSec int   `json:"duration_sec"`
	Shots      []Shot `json:"shots"`
}

type Shot struct {
	DurationSec          int    `json:"duration_sec"`
	ShotSize             string `json:"shot_size"`
	CameraMovement       string `json:"camera_movement"`
	CameraAngle          string `json:"camera_angle"`
	SubjectDescription   string `json:"subject_description"`
	BackgroundDescription string `json:"background_description"`
	LocalLightDescription string `json:"local_light_description"`
	DialogueText         string `json:"dialogue_text"`
	AmbientSound         string `json:"ambient_sound"`
	PromptRefined        string `json:"prompt_refined,omitempty"`
}

// 合法枚举集合（02 A3 唯一真源；validator 用）。
var (
	validShotSizes      = setOf("ECU", "CU", "MCU", "MS", "FS", "LS", "ELS", "TWO_SHOT", "OTS")
	validCameraMoves    = setOf("STATIC", "SLOW_PUSH_IN", "SLOW_PULL_OUT", "FAST_PUSH_IN", "DOLLY_IN", "DOLLY_OUT",
		"PAN_LEFT", "PAN_RIGHT", "TRACK_LEFT", "TRACK_RIGHT", "TILT_UP", "TILT_DOWN", "CRANE_UP", "CRANE_DOWN",
		"HANDHELD_SHAKY", "WIPE_CLIP", "ZOOM_IN_OPTICAL", "SLOW_360_ORBIT")
	validCameraAngles = setOf("EYE_LEVEL_FRONT", "SIDE", "BACK", "SLIGHT_LOW_ANGLE", "LOW_ANGLE", "SLIGHT_HIGH",
		"HIGH_BIRDVIEW", "OTS_LEFT", "OTS_RIGHT", "DUTCH_ANGLE")
)

func setOf(vals ...string) map[string]bool {
	m := make(map[string]bool, len(vals))
	for _, v := range vals {
		m[v] = true
	}
	return m
}

// ErrInvalidJSON 坏 JSON（LLM 输出无法解析）→ 触发重试（S1-B05 验收：坏 JSON 自动重试）。
var ErrInvalidJSON = errors.New("split: llm 输出非合法 JSON")

// ErrInvalidStructure 结构校验失败（枚举非法/缺字段）→ 触发重试。
var ErrInvalidStructure = errors.New("split: 结构校验失败")

// ErrProjectNotFound 项目不存在。
var ErrProjectNotFound = errors.New("split: project not found")

// ErrStateConflict 项目状态不允许写入拆分结果（重复回调/状态非法）。
var ErrStateConflict = errors.New("split: project state conflict")

// ParseAndValidate 解析并校验 LLM 输出；坏 JSON/非法枚举返回可重试错误。
func ParseAndValidate(raw string) (*SplitResult, error) {
	// 提取首个 JSON 对象（LLM 可能夹带 ```json 代码块）
	clean := extractJSON(raw)
	var result SplitResult
	if err := json.Unmarshal([]byte(clean), &result); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}
	if err := validateStructure(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func validateStructure(r *SplitResult) error {
	if len(r.Episodes) == 0 {
		return fmt.Errorf("%w: 缺少 episodes", ErrInvalidStructure)
	}
	for ei, ep := range r.Episodes {
		if len(ep.NarrativeUnits) == 0 {
			return fmt.Errorf("%w: 第%d集缺少 narrative_units", ErrInvalidStructure, ei+1)
		}
		for bi, beat := range ep.NarrativeUnits {
			if strings.TrimSpace(beat.Title) == "" {
				return fmt.Errorf("%w: 第%d集第%d个叙事单元缺标题", ErrInvalidStructure, ei+1, bi+1)
			}
			if len(beat.ShotGroups) == 0 {
				return fmt.Errorf("%w: 叙事单元 %q 缺少 shot_groups", ErrInvalidStructure, beat.Title)
			}
			for gi, sg := range beat.ShotGroups {
				if len(sg.Shots) == 0 {
					return fmt.Errorf("%w: 镜头块 %q 缺少 shots", ErrInvalidStructure, sg.Title)
				}
				for si, s := range sg.Shots {
					if s.DurationSec <= 0 || s.DurationSec > 30 {
						return fmt.Errorf("%w: 镜头时长非法 %d", ErrInvalidStructure, s.DurationSec)
					}
					if !validShotSizes[s.ShotSize] {
						return fmt.Errorf("%w: 景别 %q 非法（第%d集/单元%d/块%d/镜头%d）", ErrInvalidStructure, s.ShotSize, ei+1, bi+1, gi+1, si+1)
					}
					if !validCameraMoves[s.CameraMovement] {
						return fmt.Errorf("%w: 运镜 %q 非法", ErrInvalidStructure, s.CameraMovement)
					}
					if !validCameraAngles[s.CameraAngle] {
						return fmt.Errorf("%w: 视角 %q 非法", ErrInvalidStructure, s.CameraAngle)
					}
				}
			}
		}
	}
	// 集级资产轻量校验：code/name 非空（缺省字段容忍，落库时兜底）
	for i, sc := range r.Scenes {
		if strings.TrimSpace(sc.Code) == "" || strings.TrimSpace(sc.BaseName) == "" {
			return fmt.Errorf("%w: 第%d个场景缺少 code/base_name", ErrInvalidStructure, i+1)
		}
	}
	for i, p := range r.Props {
		if strings.TrimSpace(p.Code) == "" || strings.TrimSpace(p.Name) == "" {
			return fmt.Errorf("%w: 第%d个道具缺少 code/name", ErrInvalidStructure, i+1)
		}
	}
	return nil
}

// extractJSON 从 LLM 输出中提取第一个 { ... } 对象（容忍 ```json 围栏与前缀文本）。
func extractJSON(raw string) string {
	start := strings.Index(raw, "{")
	if start < 0 {
		return raw
	}
	end := strings.LastIndex(raw, "}")
	if end <= start {
		return raw
	}
	return raw[start : end+1]
}
