package auth

// knownRouteIDs is every route the registered Limen plugins can mount at
// the pinned versions. The HTTP surface is an ALLOWLIST: everything here that
// is not in allowedRouteIDs is disabled. Revisit on every Limen upgrade; a
// route added upstream and not named here would be silently enabled, which
// is why TestLimenRouteAllowlist probes concrete paths.
var knownRouteIDs = []string{
	// core
	"me", "list-sessions", "signout", "revoke-sessions", "verify-email", "email-verifications",
	// credential-password
	"signin", "signup", "passwords-request-reset", "passwords-reset", "passwords-change", "passwords-set", "usernames-check",
	// two-factor
	"two-factor-initiate-setup", "two-factor-finalize-setup", "two-factor-disable", "two-factor-verify",
	"get-backup-codes", "totp-uri", "otp-send",
	// organization
	"organizations:create", "organizations:list", "organizations:check-slug", "organizations:update", "organizations:delete",
	"organizations:members-list", "organizations:member-get", "organizations:get-active", "organizations:switch",
	"organizations:leave-organization", "organizations:invite-member", "organizations:respond-to-invitation",
	"organizations:get-invitation-by-token", "organizations:cancel-pending-invitation", "organizations:list-invitations",
	"organizations:revoke-member-role", "organizations:assign-member-role", "organizations:remove-member",
	"organizations:create-role", "organizations:list-roles", "organizations:update-role", "organizations:delete-role",
}

// allowedRouteIDs is what the SPA reaches on /api/auth/*. Sessions list and
// revoke are Snarvei's own (Limen's serialise the token); every organization
// and invitation route is Snarvei's own (invitations carry a team, roles are
// enforced in one place); email OTP is not offered.
func allowedRouteIDs(openSignup bool) []string {
	allowed := []string{
		"me", "signout", "signin",
		"passwords-change", "passwords-request-reset", "passwords-reset",
		"two-factor-initiate-setup", "two-factor-finalize-setup", "two-factor-disable", "two-factor-verify",
		"get-backup-codes", "totp-uri",
	}
	if openSignup {
		allowed = append(allowed, "signup")
	}
	return allowed
}

func disabledRouteIDs(openSignup bool) []string {
	allowed := map[string]struct{}{}
	for _, id := range allowedRouteIDs(openSignup) {
		allowed[id] = struct{}{}
	}
	var disabled []string
	for _, id := range knownRouteIDs {
		if _, ok := allowed[id]; !ok {
			disabled = append(disabled, id)
		}
	}
	return disabled
}
