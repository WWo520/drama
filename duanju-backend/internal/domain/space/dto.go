// Package space 空间模块：个人空间列表/详情/切换/配额（对齐 05 TAG 2 · 2.1/2.3/2.5/2.12）。
package space

// SpaceCardDTO 我的空间列表项（05 SpaceCardDTO）。
type SpaceCardDTO struct {
	SpaceID     string `json:"space_id"`
	Name        string `json:"name"`
	AvatarURL   string `json:"avatar_url"`
	Type        string `json:"type"` // PERSONAL / TEAM
	MyRole      string `json:"my_role"`
	MemberCount int    `json:"member_count"`
	UpdatedAt   string `json:"updated_at"`
}

// SpaceDetailDTO 空间详情（05 SpaceDetailDTO）。
type SpaceDetailDTO struct {
	SpaceCardDTO
	OwnerUserID string `json:"owner_user_id"`
	Description string `json:"description"`
	Quota       SpaceQuotaDTO `json:"quota"`
	CreatedAt   string `json:"created_at"`
}

// SpaceContextDTO 切换工作空间响应（05 SpaceContextDTO）。
type SpaceContextDTO struct {
	SpaceID string        `json:"space_id"`
	Name    string        `json:"name"`
	MyRole  string        `json:"my_role"`
	Quota   SpaceQuotaDTO `json:"quota"`
}

// SpaceQuotaDTO 配额与用量（05 SpaceQuotaDTO）。
type SpaceQuotaDTO struct {
	Storage struct {
		LimitBytes int64 `json:"limit_bytes"`
		UsedBytes  int64 `json:"used_bytes"`
	} `json:"storage"`
	Member struct {
		LimitCount int `json:"limit_count"`
		UsedCount  int `json:"used_count"`
	} `json:"member"`
	Compute struct {
		DailyPointsQuota *int `json:"daily_points_quota"`
		DailyPointsUsed  *int `json:"daily_points_used"`
	} `json:"compute"`
}
