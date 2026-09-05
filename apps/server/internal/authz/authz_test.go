package authz

import "testing"

func TestRoles(t *testing.T) {
	if !IsOrgAdmin(RoleOwner) || !IsOrgAdmin(RoleAdmin) || IsOrgAdmin(RoleMember) || IsOrgAdmin("") {
		t.Fatal("IsOrgAdmin")
	}
	if Highest([]string{"member", "admin"}) != RoleAdmin || Highest([]string{"member", "owner", "admin"}) != RoleOwner || Highest(nil) != "" || Highest([]string{"bogus"}) != "" {
		t.Fatal("Highest")
	}
	if !IsValidInviteRole(RoleAdmin) || !IsValidInviteRole(RoleMember) || IsValidInviteRole(RoleOwner) || IsValidInviteRole("x") {
		t.Fatal("IsValidInviteRole")
	}
}

func TestTeamAccess(t *testing.T) {
	cases := []struct {
		role   string
		member bool
		want   bool
	}{
		{RoleOwner, false, true}, {RoleAdmin, false, true}, {RoleMember, true, true}, {RoleMember, false, false}, {"", true, false},
	}
	for _, c := range cases {
		if got := CanAccessTeam(c.role, c.member); got != c.want {
			t.Errorf("CanAccessTeam(%q,%v)=%v", c.role, c.member, got)
		}
	}
	if !CanManageTeams(RoleAdmin) || CanManageTeams(RoleMember) || !CanInvite(RoleOwner) || CanInvite(RoleMember) {
		t.Fatal("manage/invite")
	}
}
