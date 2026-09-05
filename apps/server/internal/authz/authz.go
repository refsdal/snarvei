// Package authz holds Snarvei's tenancy rules as pure functions: owners and
// admins see and manage everything in their organization; members only the
// teams they belong to. No I/O, so every rule is unit-tested here and the
// middleware only gathers the facts.
package authz

const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
)

var rank = map[string]int{RoleOwner: 3, RoleAdmin: 2, RoleMember: 1}

// IsValidInviteRole reports whether an invitation may carry role.
func IsValidInviteRole(role string) bool { return role == RoleAdmin || role == RoleMember }

// IsOrgAdmin reports whether role sees every team in the organization.
func IsOrgAdmin(role string) bool { return role == RoleOwner || role == RoleAdmin }

// Highest picks the most privileged known role; "" when none is known.
func Highest(roles []string) string {
	best, bestRank := "", 0
	for _, r := range roles {
		if rank[r] > bestRank {
			best, bestRank = r, rank[r]
		}
	}
	return best
}

// CanAccessTeam: org admins always; members only when they belong to the team.
func CanAccessTeam(orgRole string, isTeamMember bool) bool {
	return IsOrgAdmin(orgRole) || (orgRole == RoleMember && isTeamMember)
}

// CanManageTeams: create teams and change team membership.
func CanManageTeams(orgRole string) bool { return IsOrgAdmin(orgRole) }

// CanInvite: create and cancel invitations.
func CanInvite(orgRole string) bool { return IsOrgAdmin(orgRole) }
