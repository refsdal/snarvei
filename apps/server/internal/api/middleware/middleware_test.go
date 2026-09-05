package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/refsdal/snarvei/server/internal/api/middleware"
	"github.com/refsdal/snarvei/server/internal/auth"
	"github.com/refsdal/snarvei/server/internal/clientip"
	"github.com/refsdal/snarvei/server/internal/db/gen"
	"github.com/refsdal/snarvei/server/internal/email"
	"github.com/refsdal/snarvei/server/internal/ratelimit"
	"github.com/refsdal/snarvei/server/internal/testrig"
)

const secret = "snarvei-middleware-test-secret-0123456789"
const password = "Testpass123"

type fx struct {
	t    *testing.T
	d    middleware.Deps
	rig  *testrig.Rig
	auth http.Handler
}

func setup(t *testing.T) *fx {
	t.Helper()
	rig := testrig.Setup(t)
	hasher := clientip.NewHasher("", secret)
	svc, err := auth.New(auth.Config{AppURL: "http://localhost:3000", Secret: secret, OpenSignup: true, Pool: rig.Pool, ClientIP: hasher.Extractor(0), Email: email.NewRecording()})
	if err != nil {
		t.Fatal(err)
	}
	q := gen.New(rig.Pool)
	return &fx{t: t, rig: rig, auth: svc.Handler(), d: middleware.Deps{Auth: svc, Q: q, RateLimit: ratelimit.NewPostgres(q), Hasher: hasher, TrustedProxyHops: 0}}
}

func (f *fx) user(name, addr string) (string, string) {
	f.t.Helper()
	id, err := f.d.Auth.CreateUser(context.Background(), name, addr, password)
	if err != nil {
		f.t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, auth.BasePath+"/signin/credential", strings.NewReader(`{"credential":"`+addr+`","password":"`+password+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://localhost:3000")
	rec := httptest.NewRecorder()
	f.auth.ServeHTTP(rec, req)
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.SessionCookieName {
			return id, c.Name + "=" + c.Value
		}
	}
	f.t.Fatalf("no cookie: %d %s", rec.Code, rec.Body.String())
	return "", ""
}

func get(h http.Handler, pattern, path, cookie string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	mux.Handle(pattern, h)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func ok() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
}

func TestSessionAndRequireSession(t *testing.T) {
	f := setup(t)
	_, cookie := f.user("Kari", "kari@example.com")
	var seen *auth.Session
	h := middleware.Session(f.d)(middleware.RequireSession()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = middleware.SessionFromContext(r.Context())
	})))
	if rec := get(h, "GET /x", "/x", ""); rec.Code != 401 || !strings.Contains(rec.Body.String(), "UNAUTHENTICATED") {
		t.Fatalf("anonymous: %d %s", rec.Code, rec.Body.String())
	}
	if rec := get(h, "GET /x", "/x", cookie); rec.Code != 200 || seen == nil || seen.Name != "Kari" {
		t.Fatalf("signed in: %d %+v", rec.Code, seen)
	}
}

func TestRequireOrgAndAdmin(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	ownerID, ownerCookie := f.user("Owner", "owner@example.com")
	memberID, memberCookie := f.user("Member", "member@example.com")
	_, strangerCookie := f.user("Stranger", "stranger@example.com")
	org, err := f.d.Auth.CreateOrganization(ctx, ownerID, "Acme", "acme")
	if err != nil {
		t.Fatal(err)
	}
	inv, err := f.d.Auth.CreateInvitation(ctx, ownerID, org.ID, "member@example.com", "member")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.d.Auth.AcceptInvitation(ctx, memberID, inv.ID); err != nil {
		t.Fatal(err)
	}

	var seen middleware.OrgCtx
	member := middleware.Session(f.d)(middleware.RequireOrg(f.d)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen, _ = middleware.OrgFromContext(r.Context())
	})))
	admin := middleware.Session(f.d)(middleware.RequireOrg(f.d)(middleware.RequireOrgAdmin()(ok())))
	pattern := "GET /api/organizations/{orgId}/teams"
	path := "/api/organizations/" + org.ID + "/teams"

	if rec := get(member, pattern, path, ""); rec.Code != 401 {
		t.Fatalf("anonymous: %d", rec.Code)
	}
	if rec := get(member, pattern, path, strangerCookie); rec.Code != 403 || !strings.Contains(rec.Body.String(), "FORBIDDEN") {
		t.Fatalf("stranger: %d %s", rec.Code, rec.Body.String())
	}
	if rec := get(member, pattern, "/api/organizations/nope/teams", ownerCookie); rec.Code != 403 {
		t.Fatalf("unknown org must look like forbidden: %d", rec.Code)
	}
	if rec := get(member, pattern, path, memberCookie); rec.Code != 200 || seen.Role != "member" || seen.OrgID != org.ID || seen.UserID != memberID {
		t.Fatalf("member: %d %+v", rec.Code, seen)
	}
	if rec := get(member, pattern, path, ownerCookie); rec.Code != 200 || seen.Role != "owner" {
		t.Fatalf("owner: %d %+v", rec.Code, seen)
	}
	if rec := get(admin, pattern, path, memberCookie); rec.Code != 403 {
		t.Fatalf("member on admin route: %d", rec.Code)
	}
	if rec := get(admin, pattern, path, ownerCookie); rec.Code != 200 {
		t.Fatalf("owner on admin route: %d", rec.Code)
	}
}

func TestRequireTeam(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	ownerID, ownerCookie := f.user("Owner", "owner@example.com")
	memberID, memberCookie := f.user("Member", "member@example.com")
	outsiderID, outsiderCookie := f.user("Outsider", "outsider@example.com")
	org, _ := f.d.Auth.CreateOrganization(ctx, ownerID, "Acme", "acme")
	for _, u := range []struct{ id, mail string }{{memberID, "member@example.com"}, {outsiderID, "outsider@example.com"}} {
		inv, err := f.d.Auth.CreateInvitation(ctx, ownerID, org.ID, u.mail, "member")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.d.Auth.AcceptInvitation(ctx, u.id, inv.ID); err != nil {
			t.Fatal(err)
		}
	}
	team, err := f.d.Q.CreateTeam(ctx, gen.CreateTeamParams{ID: auth.NewID(), OrganizationID: org.ID, Name: "Marketing"})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.d.Q.AddTeamMember(ctx, gen.AddTeamMemberParams{TeamID: team.ID, UserID: memberID}); err != nil {
		t.Fatal(err)
	}

	var seen middleware.TeamCtx
	h := middleware.Session(f.d)(middleware.RequireTeam(f.d)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen, _ = middleware.TeamFromContext(r.Context())
	})))
	adminH := middleware.Session(f.d)(middleware.RequireTeam(f.d)(middleware.RequireTeamAdmin()(ok())))
	pattern := "GET /api/teams/{teamId}/members"
	path := "/api/teams/" + team.ID + "/members"

	if rec := get(h, pattern, "/api/teams/nope/members", ownerCookie); rec.Code != 404 {
		t.Fatalf("unknown team: %d", rec.Code)
	}
	if rec := get(h, pattern, path, outsiderCookie); rec.Code != 403 {
		t.Fatalf("org member outside the team: %d", rec.Code)
	}
	if rec := get(h, pattern, path, memberCookie); rec.Code != 200 || !seen.IsTeamMember || seen.Role != "member" || seen.OrgID != org.ID {
		t.Fatalf("team member: %d %+v", rec.Code, seen)
	}
	if rec := get(h, pattern, path, ownerCookie); rec.Code != 200 || seen.IsTeamMember || seen.Role != "owner" {
		t.Fatalf("owner outside the team: %d %+v", rec.Code, seen)
	}
	if rec := get(adminH, pattern, path, memberCookie); rec.Code != 403 {
		t.Fatalf("member on team admin route: %d", rec.Code)
	}
	if rec := get(adminH, pattern, path, ownerCookie); rec.Code != 200 {
		t.Fatalf("owner on team admin route: %d", rec.Code)
	}
}

func TestResolveTeamAccess(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	ownerID, _ := f.user("Owner", "owner@example.com")
	memberID, _ := f.user("Member", "member@example.com")
	outsiderID, _ := f.user("Outsider", "outsider@example.com")
	strangerID, _ := f.user("Stranger", "stranger@example.com")
	org, _ := f.d.Auth.CreateOrganization(ctx, ownerID, "Acme", "acme")
	for _, u := range []struct{ id, mail string }{{memberID, "member@example.com"}, {outsiderID, "outsider@example.com"}} {
		inv, _ := f.d.Auth.CreateInvitation(ctx, ownerID, org.ID, u.mail, "member")
		if _, err := f.d.Auth.AcceptInvitation(ctx, u.id, inv.ID); err != nil {
			t.Fatal(err)
		}
	}
	team, _ := f.d.Q.CreateTeam(ctx, gen.CreateTeamParams{ID: auth.NewID(), OrganizationID: org.ID, Name: "Marketing"})
	_ = f.d.Q.AddTeamMember(ctx, gen.AddTeamMemberParams{TeamID: team.ID, UserID: memberID})

	if _, err := middleware.ResolveTeamAccess(ctx, f.d, ownerID, "nope"); !errors.Is(err, middleware.ErrTeamNotFound) {
		t.Fatalf("unknown team: %v", err)
	}
	for _, uid := range []string{outsiderID, strangerID} {
		if _, err := middleware.ResolveTeamAccess(ctx, f.d, uid, team.ID); !errors.Is(err, middleware.ErrTeamForbidden) {
			t.Fatalf("%s: %v", uid, err)
		}
	}
	tc, err := middleware.ResolveTeamAccess(ctx, f.d, memberID, team.ID)
	if err != nil || !tc.IsTeamMember || tc.Role != "member" || tc.OrgID != org.ID {
		t.Fatalf("member: %+v %v", tc, err)
	}
	tc, err = middleware.ResolveTeamAccess(ctx, f.d, ownerID, team.ID)
	if err != nil || tc.IsTeamMember || tc.Role != "owner" {
		t.Fatalf("owner: %+v %v", tc, err)
	}
}

func TestTrustedProxyAndRateLimit(t *testing.T) {
	f := setup(t)
	var remote string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { remote = r.RemoteAddr })
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.9")
	middleware.TrustedProxy(0)(inner).ServeHTTP(httptest.NewRecorder(), req)
	if !strings.HasPrefix(remote, "10.0.0.1") {
		t.Fatalf("hops=0 rewrote RemoteAddr to %q", remote)
	}
	middleware.TrustedProxy(1)(inner).ServeHTTP(httptest.NewRecorder(), req)
	if !strings.HasPrefix(remote, "203.0.113.9") {
		t.Fatalf("hops=1 did not rewrite RemoteAddr: %q", remote)
	}

	limited := middleware.RateLimit(f.d, "test", 2, time.Minute)(ok())
	for i := 1; i <= 3; i++ {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/x", nil)
		r.RemoteAddr = "198.51.100.5:1"
		limited.ServeHTTP(rec, r)
		if i <= 2 && rec.Code != 200 {
			t.Fatalf("hit %d: %d", i, rec.Code)
		}
		if i == 3 && (rec.Code != 429 || rec.Header().Get("Retry-After") == "" || !strings.Contains(rec.Body.String(), "RATE_LIMITED")) {
			t.Fatalf("hit 3: %d %q %s", rec.Code, rec.Header().Get("Retry-After"), rec.Body.String())
		}
	}
	rec := httptest.NewRecorder()
	other := httptest.NewRequest(http.MethodGet, "/x", nil)
	other.RemoteAddr = "198.51.100.6:1"
	limited.ServeHTTP(rec, other)
	if rec.Code != 200 {
		t.Fatalf("another address must have its own bucket: %d", rec.Code)
	}
}
