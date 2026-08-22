// creation DTO（05 TAG 3 · 3.0.x/3.1.x 子集）。
package creation

// ProjectDTO 项目（3.0.1/3.0.2/3.0.3 响应）。
type ProjectDTO struct {
	ProjectID      string `json:"project_id"` // project_uuid
	SpaceID        string `json:"space_id"`
	Title          string `json:"title"`
	CoverImageURL  string `json:"cover_image_url"`
	SourceScriptText string `json:"source_script_text"` // 剧本原文（Step1 预填/重拆）
	ProjectType    string `json:"project_type"`
	CurrentStep    string `json:"current_step"`
	Status         string `json:"status"`
	TotalEpisodeCount int `json:"total_episode_count"`
	TotalDurationSec  int `json:"total_duration_sec"`
	LastOpenedAt   string `json:"last_opened_at"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

// CreateProjectReq 创建项目（3.0.2）。
type CreateProjectReq struct {
	Title            string `json:"title" binding:"required,min=1,max=256"`
	SourceScriptText string `json:"source_script_text" binding:"omitempty,max=50000"` // ≤5 万字（20004）
	SpaceID          string `json:"space_id" binding:"omitempty"`                     // 缺省用默认空间
}

// UpdateProjectReq 更新项目（3.0.4）。
type UpdateProjectReq struct {
	Title          *string `json:"title" binding:"omitempty,min=1,max=256"`
	CoverImageURL  *string `json:"cover_image_url" binding:"omitempty,max=512"`
	SourceScriptText *string `json:"source_script_text" binding:"omitempty,max=50000"`
}

// EpisodeDTO 剧集（3.1.2/3.1.3）。
type EpisodeDTO struct {
	EpisodeID          string `json:"episode_id"`
	SortOrder          int    `json:"sort_order"`
	Title              string `json:"title"`
	Status             string `json:"status"`
	CurrentStep        string `json:"current_step"`
	NarrativeUnitCount int    `json:"narrative_unit_count"`
	ActualDurationSec  int    `json:"actual_duration_sec"`
}

// BeatDTO 叙事单元（3.1.4/3.1.5）。
type BeatDTO struct {
	BeatID           string `json:"beat_id"` // nu_uuid
	SortOrder        int    `json:"sort_order"`
	Title            string `json:"title"`
	PresetDurationSec *int  `json:"preset_duration_sec"`
	ActualDurationSec int   `json:"actual_duration_sec"`
	Status           string `json:"status"`
	VersionNum       int    `json:"version_num"`
	Content          string `json:"content"`
	ScriptOriginalText string `json:"script_original_text"`
	ScriptText       string `json:"script_text"`       // 前端展示用剧本原文（script_original_text 优先，content 兜底）
	ShotGroupCount   int    `json:"shot_group_count"`  // 镜头块数（Step1 单元卡展示）
}

// SkillDTO 智能体技能（用户端 /skills 列表项；全局 agent_skills 表）。
type SkillDTO struct {
	SkillCode       string `json:"skill_code"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	TemplateContent string `json:"template_content"` // 技能提示词模板（弹窗预览用）
}
