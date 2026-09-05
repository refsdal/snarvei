// Package middleware puts identity and tenancy on the request context and
// rejects what the rules in internal/authz forbid. Nothing here imports
// internal/api; the error envelope comes from internal/api/respond.
package middleware

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/refsdal/snarvei/server/internal/api/respond"
	"github.com/refsdal/snarvei/server/internal/auth"
	"github.com/refsdal/snarvei/server/internal/authz"
	"github.com/refsdal/snarvei/server/internal/clientip"
	"github.com/refsdal/snarvei/server/internal/db/gen"
	"github.com/refsdal/snarvei/server/internal/ratelimit"
)

// Deps is what the chain needs from the composition root.
type Deps struct {
	Auth             auth.Service
	Q                *gen.Queries
	RateLimit        ratelimit.Store
	Hasher           *clientip.Hasher
	TrustedProxyHops int
}

// OrgCtx is the organization named in the path plus the caller's role in it.
type OrgCtx struct{ OrgID, UserID, Role string }

// TeamCtx is the team named in the path plus the caller's standing.
type TeamCtx struct {
	TeamID, OrgID, UserID, Role string
	IsTeamMember                bool
}

type ctxKey int

const (
	sessionKey ctxKey = iota
	orgKey
	teamKey
	httpKey
)

type httpPair struct {
	w http.ResponseWriter
	r *http.Request
}

// SessionFromContext returns the resolved session or nil.
func SessionFromContext(ctx context.Context) *auth.Session {
	s, _ := ctx.Value(sessionKey).(*auth.Session)
	return s
}

// OrgFromContext returns what RequireOrg resolved.
func OrgFromContext(ctx context.Context) (OrgCtx, bool) {
	o, ok := ctx.Value(orgKey).(OrgCtx)
	return o, ok
}

// TeamFromContext returns what RequireTeam resolved.
func TeamFromContext(ctx context.Context) (TeamCtx, bool) {
	t, ok := ctx.Value(teamKey).(TeamCtx)
	return t, ok
}

// HTTPFromContext returns the raw writer and request captured by CaptureHTTP.
func HTTPFromContext(ctx context.Context) (http.ResponseWriter, *http.Request, bool) {
	p, ok := ctx.Value(httpKey).(httpPair)
	return p.w, p.r, ok
}

// TrustedProxy rewrites RemoteAddr to the trusted client address so Limen's
// limiter, the session digest and every handler agree on who is calling.
func TrustedProxy(hops int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if hops <= 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			address := clientip.FromRequest(r, hops)
			if address == "unknown" {
				next.ServeHTTP(w, r)
				return
			}
			if _, port, err := net.SplitHostPort(r.RemoteAddr); err == nil {
				address = net.JoinHostPort(address, port)
			}
			rewritten := *r
			rewritten.RemoteAddr = address
			next.ServeHTTP(w, &rewritten)
		})
	}
}

// Session resolves the caller (never rejects) and refreshes the cookie.
func Session(d Deps) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s, err := d.Auth.SessionFromRequest(w, r)
			if err != nil {
				respond.Error(w, http.StatusInternalServerError, "INTERNAL", "session lookup failed")
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionKey, s)))
		})
	}
}

// RequireSession rejects anonymous callers.
func RequireSession() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if SessionFromContext(r.Context()) == nil {
				respond.Error(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Not signed in")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireOrg resolves {orgId} and the caller's role; non-members (and
// unknown organizations) get 403 so existence is never revealed.
func RequireOrg(d Deps) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s := SessionFromContext(r.Context())
			if s == nil {
				respond.Error(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Not signed in")
				return
			}
			orgID := r.PathValue("orgId")
			role, err := memberRole(r.Context(), d.Q, orgID, s.UserID)
			if err != nil {
				respond.Error(w, http.StatusInternalServerError, "INTERNAL", "membership lookup failed")
				return
			}
			if role == "" {
				respond.Error(w, http.StatusForbidden, "FORBIDDEN", "Organization access denied")
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), orgKey, OrgCtx{OrgID: orgID, UserID: s.UserID, Role: role})))
		})
	}
}

// RequireOrgAdmin runs after RequireOrg.
func RequireOrgAdmin() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			o, ok := OrgFromContext(r.Context())
			if !ok || !authz.IsOrgAdmin(o.Role) {
				respond.Error(w, http.StatusForbidden, "FORBIDDEN", "Organization admin required")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

var (
	ErrTeamNotFound  = errors.New("middleware: team not found")
	ErrTeamForbidden = errors.New("middleware: team access denied")
)

// ResolveTeamAccess loads the team, the caller's org role and team
// membership, and applies authz.CanAccessTeam. On ErrTeamForbidden the
// returned TeamCtx is still populated (callers use it to tell a non-member
// from a lookup failure).
func ResolveTeamAccess(ctx context.Context, d Deps, userID, teamID string) (TeamCtx, error) {
	team, err := d.Q.GetTeam(ctx, teamID)
	if errors.Is(err, pgx.ErrNoRows) {
		return TeamCtx{}, ErrTeamNotFound
	}
	if err != nil {
		return TeamCtx{}, fmt.Errorf("middleware: load team: %w", err)
	}
	role, err := memberRole(ctx, d.Q, team.OrganizationID, userID)
	if err != nil {
		return TeamCtx{}, fmt.Errorf("middleware: membership lookup: %w", err)
	}
	n, err := d.Q.IsTeamMember(ctx, gen.IsTeamMemberParams{TeamID: teamID, UserID: userID})
	if err != nil {
		return TeamCtx{}, fmt.Errorf("middleware: team membership lookup: %w", err)
	}
	tc := TeamCtx{TeamID: teamID, OrgID: team.OrganizationID, UserID: userID, Role: role, IsTeamMember: n > 0}
	if !authz.CanAccessTeam(tc.Role, tc.IsTeamMember) {
		return tc, ErrTeamForbidden
	}
	return tc, nil
}

// RequireTeam resolves {teamId} via ResolveTeamAccess for the signed-in
// caller.
func RequireTeam(d Deps) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s := SessionFromContext(r.Context())
			if s == nil {
				respond.Error(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Not signed in")
				return
			}
			teamID := r.PathValue("teamId")
			tc, err := ResolveTeamAccess(r.Context(), d, s.UserID, teamID)
			switch {
			case errors.Is(err, ErrTeamNotFound):
				respond.Error(w, http.StatusNotFound, "NOT_FOUND", "Team not found")
				return
			case errors.Is(err, ErrTeamForbidden):
				respond.Error(w, http.StatusForbidden, "FORBIDDEN", "Team access denied")
				return
			case err != nil:
				respond.Error(w, http.StatusInternalServerError, "INTERNAL", "team lookup failed")
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), teamKey, tc)))
		})
	}
}

// RequireTeamAdmin runs after RequireTeam.
func RequireTeamAdmin() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tc, ok := TeamFromContext(r.Context())
			if !ok || !authz.CanManageTeams(tc.Role) {
				respond.Error(w, http.StatusForbidden, "FORBIDDEN", "Organization admin required")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CaptureHTTP makes the raw writer and request reachable from a strict
// handler's context (needed to set a session cookie).
func CaptureHTTP() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), httpKey, httpPair{w: w, r: r})))
		})
	}
}

// RateLimit allows limit requests per window per hashed client address.
func RateLimit(d Deps, name string, limit int, window time.Duration) func(http.Handler) http.Handler {
	if d.RateLimit == nil || d.Hasher == nil {
		panic("middleware: RateLimit needs a store and a hasher")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			bucket := d.Hasher.Hash(clientip.FromRequest(r, d.TrustedProxyHops))[:32]
			key, _ := ratelimit.Key(name, bucket, time.Now(), window)
			count, retry, err := d.RateLimit.Hit(r.Context(), key, window)
			if err != nil {
				respond.Error(w, http.StatusInternalServerError, "INTERNAL", "rate limit check failed")
				return
			}
			if count > limit {
				w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(retry.Seconds()))))
				respond.Error(w, http.StatusTooManyRequests, "RATE_LIMITED", "Too many requests, try again later")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// memberRole returns the caller's highest role in orgID, "" when not a member.
func memberRole(ctx context.Context, q *gen.Queries, orgID, userID string) (string, error) {
	roles, err := q.GetMemberRoles(ctx, gen.GetMemberRolesParams{OrganizationID: orgID, UserID: userID})
	if err != nil {
		return "", err
	}
	return authz.Highest(roles), nil
}
