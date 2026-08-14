// 权限矩阵（S2-B02）：03 E.1 六角色（OWNER/ADMIN/EDITOR/MEMBER/VIEWER/BILLING_ONLY）。
// 权限判断统一按 space_members.role，禁止硬编码 4 角色比较（02 A1 真源）。
package space

import "fmt"

// 操作枚举（03 E.1 矩阵行）
const (
	PermViewResources    = "view_resources"    // 查看空间资源
	PermEditProject      = "edit_project"      // 创建/编辑/克隆项目
	PermDeleteProject    = "delete_project"    // 删除项目（全员/自己）
	PermEditStep         = "edit_step"         // Step1~3 编辑
	PermExport           = "export"            // Step4 合成/下载 MP4
	PermManageMembers    = "manage_members"    // 空间配置/成员管理/协作审计
	PermTeamBilling      = "team_billing"      // 团队积分充值/全员报表
	PermViewBillingOwn   = "view_billing_own"  // 个人空间账户/消费明细
	PermDissolveSpace    = "dissolve_space"    // 解散空间
	PermInviteMembers    = "invite_members"    // 邀请成员
)

// matrix 角色 → 操作白名单（03 E.1 六列真值）
var matrix = map[string]map[string]bool{
	"OWNER": {
		PermViewResources: true, PermEditProject: true, PermDeleteProject: true, PermEditStep: true,
		PermExport: true, PermManageMembers: true, PermTeamBilling: true, PermViewBillingOwn: true,
		PermDissolveSpace: true, PermInviteMembers: true,
	},
	"ADMIN": {
		PermViewResources: true, PermEditProject: true, PermDeleteProject: true, PermEditStep: true,
		PermExport: true, PermManageMembers: true, PermTeamBilling: true, PermViewBillingOwn: true,
		PermInviteMembers: true,
	},
	"EDITOR": {
		PermViewResources: true, PermEditProject: true, PermDeleteProject: true, PermEditStep: true,
		PermExport: true, PermTeamBilling: true, PermViewBillingOwn: true,
	},
	"MEMBER": {
		PermViewResources: true, PermEditProject: true, PermDeleteProject: true, PermEditStep: true,
		PermExport: true, PermTeamBilling: true, PermViewBillingOwn: true,
	},
	"VIEWER": {
		PermViewResources: true, PermViewBillingOwn: true,
	},
	"BILLING_ONLY": {
		PermViewResources: true, PermViewBillingOwn: true, PermTeamBilling: true,
	},
}

// Can 角色是否允许操作。
func Can(role, perm string) bool {
	if perms, ok := matrix[role]; ok {
		return perms[perm]
	}
	return false
}

// Require 越权返回错误（403 code=10004 由调用方映射）。
func Require(role, perm string) error {
	if !Can(role, perm) {
		return fmt.Errorf("角色 %s 无权限执行 %s", role, perm)
	}
	return nil
}

// AllRoles 六角色列表（供邀请/改角色校验）。
var AllRoles = []string{"OWNER", "ADMIN", "EDITOR", "MEMBER", "VIEWER", "BILLING_ONLY"}