// 权限矩阵全量用例（S2-Q01）：6 角色 × 关键操作。
package space

import "testing"

func TestPermissionMatrix(t *testing.T) {
	cases := []struct {
		role string
		perm string
		want bool
	}{
		// 查看空间资源：全角色 ✓
		{"OWNER", PermViewResources, true},
		{"ADMIN", PermViewResources, true},
		{"EDITOR", PermViewResources, true},
		{"MEMBER", PermViewResources, true},
		{"VIEWER", PermViewResources, true},
		{"BILLING_ONLY", PermViewResources, true},
		// 成员管理：仅 OWNER/ADMIN
		{"OWNER", PermManageMembers, true},
		{"ADMIN", PermManageMembers, true},
		{"EDITOR", PermManageMembers, false},
		{"MEMBER", PermManageMembers, false},
		{"VIEWER", PermManageMembers, false},
		{"BILLING_ONLY", PermManageMembers, false},
		// 邀请：仅 OWNER/ADMIN
		{"OWNER", PermInviteMembers, true},
		{"ADMIN", PermInviteMembers, true},
		{"MEMBER", PermInviteMembers, false},
		// 解散空间：仅 OWNER
		{"OWNER", PermDissolveSpace, true},
		{"ADMIN", PermDissolveSpace, false},
		{"MEMBER", PermDissolveSpace, false},
		// Step1~3 编辑：除 VIEWER 外全 ✓
		{"EDITOR", PermEditStep, true},
		{"MEMBER", PermEditStep, true},
		{"VIEWER", PermEditStep, false},
		// 导出：VIEWER 不可
		{"MEMBER", PermExport, true},
		{"VIEWER", PermExport, false},
		// 团队计费：VIEWER 不可
		{"MEMBER", PermTeamBilling, true},
		{"VIEWER", PermTeamBilling, false},
	}
	for _, c := range cases {
		if got := Can(c.role, c.perm); got != c.want {
			t.Errorf("Can(%s, %s) = %v, want %v", c.role, c.perm, got, c.want)
		}
	}
}

func TestRequire(t *testing.T) {
	if err := Require("MEMBER", PermManageMembers); err == nil {
		t.Error("MEMBER 管理成员应被拒绝")
	}
	if err := Require("OWNER", PermDissolveSpace); err != nil {
		t.Error("OWNER 解散空间应被允许")
	}
}

func TestAllRolesCovered(t *testing.T) {
	for _, role := range AllRoles {
		if _, ok := matrix[role]; !ok {
			t.Errorf("角色 %s 未覆盖矩阵", role)
		}
	}
	if len(AllRoles) != 6 {
		t.Errorf("应 6 角色，实际 %d", len(AllRoles))
	}
}