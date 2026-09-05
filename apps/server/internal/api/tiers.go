package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/refsdal/snarvei/server/internal/api/gen"
	"github.com/refsdal/snarvei/server/internal/api/middleware"
)

type tier int

const (
	tierPublic        tier = iota
	tierPublicCapture      // public + CaptureHTTP (sets a cookie)
	tierSession
	tierSessionRateLimited
	tierOrg
	tierOrgAdmin
	tierTeam
	tierTeamAdmin
)

// operationTiers names the chain every spec operation runs behind. A spec
// operation missing here panics at NewHandler time (assertTierCoverage), so
// a new endpoint can never ship unguarded by accident.
var operationTiers = map[string]tier{
	"Healthz": tierPublic, "Readyz": tierPublic, "GetConfig": tierPublic,
	"GetMe": tierSession, "UpdateMe": tierSession, "DeleteMe": tierSession,
	"RequestEmailChange": tierSessionRateLimited, "ConfirmEmailChange": tierSession,
	"ListMySessions": tierSession, "RevokeOtherSessions": tierSession, "RevokeMySession": tierSession,
	"ListOrganizations": tierSession, "CreateOrganization": tierSession,
	"SwitchOrganization": tierOrg, "ListOrganizationMembers": tierOrg,
	"ListInvitations": tierOrgAdmin, "CreateInvitation": tierOrgAdmin, "CancelInvitation": tierOrgAdmin,
	"GetInvitation": tierPublic, "AcceptInvitation": tierSession, "RejectInvitation": tierSession,
	"RegisterWithInvitation": tierPublicCapture,
	"ListTeams":              tierOrg, "CreateTeam": tierOrgAdmin,
	"ListTeamMembers": tierTeam, "AddTeamMember": tierTeamAdmin, "RemoveTeamMember": tierTeamAdmin,
}

const (
	writeLimit  = 30
	writeWindow = time.Minute
)

func (d Deps) mwDeps() middleware.Deps {
	return middleware.Deps{Auth: d.Auth, Q: d.Q, RateLimit: d.RateLimit, Hasher: d.Hasher, TrustedProxyHops: d.TrustedProxyHops}
}

// chain returns the http middleware for a tier.
func (d Deps) chain(t tier) func(http.Handler) http.Handler {
	md := d.mwDeps()
	session, require := middleware.Session(md), middleware.RequireSession()
	org, orgAdmin := middleware.RequireOrg(md), middleware.RequireOrgAdmin()
	team, teamAdmin := middleware.RequireTeam(md), middleware.RequireTeamAdmin()
	capture := middleware.CaptureHTTP()
	limited := middleware.RateLimit(md, "write", writeLimit, writeWindow)
	switch t {
	case tierPublic:
		return func(h http.Handler) http.Handler { return h }
	case tierPublicCapture:
		return func(h http.Handler) http.Handler { return limited(capture(h)) }
	case tierSession:
		return func(h http.Handler) http.Handler { return session(require(capture(h))) }
	case tierSessionRateLimited:
		return func(h http.Handler) http.Handler { return session(require(limited(h))) }
	case tierOrg:
		return func(h http.Handler) http.Handler { return session(org(h)) }
	case tierOrgAdmin:
		return func(h http.Handler) http.Handler { return session(org(orgAdmin(h))) }
	case tierTeam:
		return func(h http.Handler) http.Handler { return session(team(h)) }
	case tierTeamAdmin:
		return func(h http.Handler) http.Handler { return session(team(teamAdmin(h))) }
	}
	panic(fmt.Sprintf("api: unknown tier %d", t))
}

// tierMiddleware adapts the per-operation chain to the strict-server shape.
func (d Deps) tierMiddleware() gen.StrictMiddlewareFunc {
	chains := map[tier]func(http.Handler) http.Handler{}
	for t := tierPublic; t <= tierTeamAdmin; t++ {
		chains[t] = d.chain(t)
	}
	return func(f gen.StrictHandlerFunc, operationID string) gen.StrictHandlerFunc {
		t, ok := operationTiers[operationID]
		if !ok {
			panic("api: no tier for operation " + operationID)
		}
		mw := chains[t]
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request, request any) (any, error) {
			var resp any
			var err error
			terminal := http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
				resp, err = f(req.Context(), w, req, request)
			})
			mw(terminal).ServeHTTP(w, r)
			return resp, err
		}
	}
}

// assertTierCoverage panics when a spec operation has no tier.
func assertTierCoverage(spec *openapi3.T) {
	for _, path := range spec.Paths.InMatchingOrder() {
		item := spec.Paths.Find(path)
		for _, op := range item.Operations() {
			id := exportName(op.OperationID)
			if _, ok := operationTiers[id]; !ok {
				panic("api: operation " + op.OperationID + " has no entry in operationTiers")
			}
		}
	}
}

// exportName mirrors oapi-codegen's operationId → Go method name rule (first
// letter upper-cased) for the ids this spec uses.
func exportName(id string) string {
	if id == "" {
		return id
	}
	return string(id[0]-'a'+'A') + id[1:]
}
