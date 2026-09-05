package auth_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/refsdal/snarvei/server/internal/auth"
	"github.com/refsdal/snarvei/server/internal/clientip"
	"github.com/refsdal/snarvei/server/internal/email"
	"github.com/refsdal/snarvei/server/internal/testrig"
)

const testSecret = "snarvei-test-auth-secret-value-0123456789"
const password = "Testpass123"

type fixture struct {
	t    *testing.T
	svc  auth.Service
	rig  *testrig.Rig
	mux  *http.ServeMux
	mail *email.Recording
}

func newFixture(t *testing.T, openSignup bool) *fixture {
	t.Helper()
	rig := testrig.Setup(t)
	mail := email.NewRecording()
	svc, err := auth.New(auth.Config{
		AppURL: "http://localhost:3000", AppName: "Snarvei", Secret: testSecret, OpenSignup: openSignup,
		Pool: rig.Pool, ClientIP: clientip.NewHasher("", testSecret).Extractor(0), Email: mail,
	})
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	mux := http.NewServeMux()
	mux.Handle(auth.BasePath+"/", svc.Handler())
	return &fixture{t: t, svc: svc, rig: rig, mux: mux, mail: mail}
}

func (f *fixture) do(method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	f.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://localhost:3000")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	f.mux.ServeHTTP(rec, req)
	return rec
}

func (f *fixture) sessionCookie(rec *httptest.ResponseRecorder) *http.Cookie {
	f.t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.SessionCookieName && c.Value != "" {
			return c
		}
	}
	f.t.Fatalf("no session cookie: %d %s", rec.Code, rec.Body.String())
	return nil
}

func (f *fixture) signIn(name, emailAddr string) (string, *http.Cookie) {
	f.t.Helper()
	id, err := f.svc.CreateUser(context.Background(), name, emailAddr, password)
	if err != nil {
		f.t.Fatalf("CreateUser: %v", err)
	}
	rec := f.do(http.MethodPost, auth.BasePath+"/signin/credential", `{"credential":"`+emailAddr+`","password":"`+password+`"}`, nil)
	if rec.Code != http.StatusOK {
		f.t.Fatalf("sign in: %d %s", rec.Code, rec.Body.String())
	}
	return id, f.sessionCookie(rec)
}

func (f *fixture) session(cookie *http.Cookie) *auth.Session {
	f.t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.AddCookie(cookie)
	s, err := f.svc.SessionFromRequest(httptest.NewRecorder(), req)
	if err != nil {
		f.t.Fatalf("SessionFromRequest: %v", err)
	}
	return s
}

func TestSignInThenSessionFromRequest(t *testing.T) {
	f := newFixture(t, false)
	id, cookie := f.signIn("Kari", "kari@example.com")
	s := f.session(cookie)
	if s == nil || s.UserID != id || s.Name != "Kari" || s.Email != "kari@example.com" || s.Token == "" || s.SessionID == "" {
		t.Fatalf("session: %+v", s)
	}
	if s.ActiveOrganizationID != "" || s.TwoFactorEnabled {
		t.Fatalf("fresh session: %+v", s)
	}
	if got := f.session(&http.Cookie{Name: auth.SessionCookieName, Value: "bogus"}); got != nil {
		t.Fatal("bogus token must be signed out")
	}
}

func TestSignupRouteFollowsOpenSignup(t *testing.T) {
	closed := newFixture(t, false)
	rec := closed.do(http.MethodPost, auth.BasePath+"/signup/credential", `{"name":"X","email":"x@example.com","password":"`+password+`"}`, nil)
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("closed signup reachable: %d %s", rec.Code, rec.Body.String())
	}
	open := newFixture(t, true)
	rec = open.do(http.MethodPost, auth.BasePath+"/signup/credential", `{"name":"Ola","email":"ola@example.com","password":"`+password+`"}`, nil)
	if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		t.Fatalf("open signup: %d %s", rec.Code, rec.Body.String())
	}
	cookie := open.sessionCookie(rec)
	if s := open.session(cookie); s == nil || s.Name != "Ola" {
		t.Fatalf("signup must store the name: %+v", s)
	}
}

func TestLimenRouteAllowlist(t *testing.T) {
	f := newFixture(t, false)
	_, cookie := f.signIn("Kari", "kari@example.com")
	disabled := []struct{ method, path string }{
		{http.MethodGet, "/sessions"}, {http.MethodPost, "/revoke-sessions"},
		{http.MethodPost, "/signup/credential"}, {http.MethodPut, "/passwords"}, {http.MethodPost, "/usernames/check"},
		{http.MethodPost, "/two-factor/otp/send"},
		{http.MethodGet, "/organizations"}, {http.MethodPost, "/organizations"}, {http.MethodPost, "/organizations/switch"},
		{http.MethodGet, "/organizations/members"}, {http.MethodGet, "/organizations/active"}, {http.MethodPost, "/organizations/leave"},
		{http.MethodPost, "/organizations/check-slug"}, {http.MethodPost, "/organizations/invitations"},
		{http.MethodPost, "/organizations/invitations/respond"}, {http.MethodPost, "/organizations/invitations/cancel"},
		{http.MethodGet, "/organizations/invitations"}, {http.MethodDelete, "/organizations/members/m1"},
		{http.MethodPost, "/organizations/members/m1/roles/assign"}, {http.MethodPatch, "/organizations/o1"}, {http.MethodDelete, "/organizations/o1"},
	}
	for _, r := range disabled {
		rec := f.do(r.method, auth.BasePath+r.path, "{}", cookie)
		if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s reachable: %d %s", r.method, r.path, rec.Code, rec.Body.String())
		}
	}
	kept := []struct{ method, path string }{
		{http.MethodGet, "/me"}, {http.MethodPost, "/passwords/change"}, {http.MethodPost, "/passwords/request-reset"},
		{http.MethodPost, "/passwords/reset"}, {http.MethodPost, "/two-factor/initiate-setup"},
		{http.MethodPost, "/two-factor/verify"},
	}
	for _, r := range kept {
		rec := f.do(r.method, auth.BasePath+r.path, "{}", cookie)
		if rec.Code == http.StatusNotFound || rec.Code == http.StatusMethodNotAllowed {
			t.Errorf("%s %s should be reachable: %d", r.method, r.path, rec.Code)
		}
	}
	// GET totp/uri and GET backup-codes 404 with "record not found" for a
	// user who has never called initiate-setup — Limen keys both reads off
	// the two_factors row that only initiate-setup creates (totp_handlers.go,
	// backup_codes_handlers.go). That is real, sensible Limen behaviour, not
	// the allowlist; seed the row first (and before /signout below revokes
	// this session) so this checks routing, not state.
	rec := f.do(http.MethodPost, auth.BasePath+"/two-factor/initiate-setup", `{"password":"`+password+`"}`, cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("initiate-setup: %d %s", rec.Code, rec.Body.String())
	}
	for _, path := range []string{"/two-factor/totp/uri", "/two-factor/backup-codes"} {
		rec := f.do(http.MethodGet, auth.BasePath+path, "{}", cookie)
		if rec.Code == http.StatusNotFound || rec.Code == http.StatusMethodNotAllowed {
			t.Errorf("GET %s should be reachable: %d %s", path, rec.Code, rec.Body.String())
		}
	}
	rec = f.do(http.MethodPost, auth.BasePath+"/signout", "{}", cookie)
	if rec.Code == http.StatusNotFound || rec.Code == http.StatusMethodNotAllowed {
		t.Errorf("POST /signout should be reachable: %d", rec.Code)
	}
}

func TestPasswordResetSendsMailAndRevokesSessions(t *testing.T) {
	f := newFixture(t, false)
	_, cookie := f.signIn("Kari", "kari@example.com")
	rec := f.do(http.MethodPost, auth.BasePath+"/passwords/request-reset", `{"email":"kari@example.com"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("request-reset: %d %s", rec.Code, rec.Body.String())
	}
	msg, ok := f.mail.Last("kari@example.com")
	if !ok || !strings.Contains(msg.Text, "/reset-password?token=") {
		t.Fatalf("reset mail: %+v", msg)
	}
	token := msg.Text[strings.Index(msg.Text, "token=")+len("token="):]
	token = strings.Fields(token)[0]
	rec = f.do(http.MethodPost, auth.BasePath+"/passwords/reset", `{"token":"`+token+`","new_password":"Newpass456"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("reset: %d %s", rec.Code, rec.Body.String())
	}
	if f.session(cookie) != nil {
		t.Fatal("old session must be revoked after a reset")
	}
	rec = f.do(http.MethodPost, auth.BasePath+"/signin/credential", `{"credential":"kari@example.com","password":"Newpass456"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("sign in with new password: %d %s", rec.Code, rec.Body.String())
	}
}

func TestOrganizationsAndInvitations(t *testing.T) {
	f := newFixture(t, false)
	ctx := context.Background()
	ownerID, ownerCookie := f.signIn("Owner", "owner@example.com")
	org, err := f.svc.CreateOrganization(ctx, ownerID, "Acme", "acme")
	if err != nil || org.Slug != "acme" {
		t.Fatalf("CreateOrganization: %v %+v", err, org)
	}
	if _, err := f.svc.CreateOrganization(ctx, ownerID, "Acme 2", "acme"); !errors.Is(err, auth.ErrSlugTaken) {
		t.Fatalf("duplicate slug: %v", err)
	}
	if err := f.svc.SetActiveOrganization(ctx, ownerCookie.Value, org.ID); err != nil {
		t.Fatalf("SetActiveOrganization: %v", err)
	}
	if s := f.session(ownerCookie); s.ActiveOrganizationID != org.ID {
		t.Fatalf("active org not reflected: %+v", s)
	}

	inv, err := f.svc.CreateInvitation(ctx, ownerID, org.ID, "member@example.com", "member")
	if err != nil || inv.Role != "member" || inv.Status != "pending" {
		t.Fatalf("CreateInvitation: %v %+v", err, inv)
	}
	if msg, ok := f.mail.Last("member@example.com"); !ok || !strings.Contains(msg.Text, "/app/invitations/"+inv.ID) || !strings.Contains(msg.Text, "invited by Owner") {
		t.Fatalf("invitation mail: %+v", msg)
	}
	if _, err := f.svc.CreateInvitation(ctx, ownerID, org.ID, "member@example.com", "member"); !errors.Is(err, auth.ErrInvitationExists) {
		t.Fatalf("duplicate invitation: %v", err)
	}
	if _, err := f.svc.CreateInvitation(ctx, ownerID, org.ID, "x@example.com", "owner"); !errors.Is(err, auth.ErrUnknownRole) {
		t.Fatalf("owner invite must be refused: %v", err)
	}

	strangerID, _ := f.signIn("Stranger", "stranger@example.com")
	if _, err := f.svc.AcceptInvitation(ctx, strangerID, inv.ID); !errors.Is(err, auth.ErrInvitationEmailMismatch) {
		t.Fatalf("mismatch: %v", err)
	}
	if _, err := f.svc.CreateInvitation(ctx, strangerID, org.ID, "y@example.com", "member"); !errors.Is(err, auth.ErrForbidden) && !errors.Is(err, auth.ErrNotFound) {
		t.Fatalf("non-member inviting: %v", err)
	}

	memberID, memberCookie := f.signIn("Member", "member@example.com")
	if err := f.svc.SetActiveOrganization(ctx, memberCookie.Value, org.ID); !errors.Is(err, auth.ErrNotMember) {
		t.Fatalf("switch before accepting: %v", err)
	}
	accepted, err := f.svc.AcceptInvitation(ctx, memberID, inv.ID)
	if err != nil || accepted.Status != "accepted" {
		t.Fatalf("AcceptInvitation: %v %+v", err, accepted)
	}
	if err := f.svc.SetActiveOrganization(ctx, memberCookie.Value, org.ID); err != nil {
		t.Fatalf("switch after accepting: %v", err)
	}
	if _, err := f.svc.AcceptInvitation(ctx, memberID, inv.ID); !errors.Is(err, auth.ErrInvitationInvalid) {
		t.Fatalf("second accept: %v", err)
	}

	inv2, _ := f.svc.CreateInvitation(ctx, ownerID, org.ID, "z@example.com", "admin")
	if err := f.svc.CancelInvitation(ctx, memberID, org.ID, inv2.ID); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("member cancelling: %v", err)
	}
	if err := f.svc.CancelInvitation(ctx, ownerID, org.ID, inv2.ID); err != nil {
		t.Fatalf("owner cancelling: %v", err)
	}
}

func TestPasswordsSessionsAndDeletion(t *testing.T) {
	f := newFixture(t, false)
	ctx := context.Background()
	id, cookie := f.signIn("Kari", "kari@example.com")
	if err := f.svc.VerifyPassword(ctx, id, password); err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if err := f.svc.VerifyPassword(ctx, id, "wrong"); !errors.Is(err, auth.ErrInvalidPassword) {
		t.Fatalf("wrong password: %v", err)
	}
	if _, err := f.svc.CreateUser(ctx, "Dup", "kari@example.com", password); !errors.Is(err, auth.ErrEmailTaken) {
		t.Fatalf("duplicate email: %v", err)
	}
	var pe *auth.PasswordPolicyError
	if _, err := f.svc.CreateUser(ctx, "Weak", "weak@example.com", "short"); !errors.As(err, &pe) {
		t.Fatalf("weak password: %v", err)
	}

	// StartSession issues a cookie without a password round trip.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/invitations/x/register", nil)
	if err := f.svc.StartSession(ctx, rec, req, id); err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	started := f.sessionCookie(rec)
	if s := f.session(started); s == nil || s.UserID != id {
		t.Fatalf("started session: %+v", s)
	}

	s := f.session(cookie)
	if err := f.svc.RevokeSession(ctx, s.Token); err != nil {
		t.Fatalf("RevokeSession: %v", err)
	}
	if f.session(cookie) != nil {
		t.Fatal("revoked session still valid")
	}
	if f.session(started) == nil {
		t.Fatal("other session must survive a single revoke")
	}
	if err := f.svc.DeleteUser(ctx, id); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if f.session(started) != nil {
		t.Fatal("deleted user still has a session")
	}
	var n int
	if err := f.rig.Pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE id = $1`, id).Scan(&n); err != nil || n != 0 {
		t.Fatalf("user row remains: %v %d", err, n)
	}
}

func TestSessionMetadataStoresNoRawAddress(t *testing.T) {
	f := newFixture(t, false)
	_, err := f.svc.CreateUser(context.Background(), "Kari", "kari@example.com", password)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, auth.BasePath+"/signin/credential", strings.NewReader(`{"credential":"kari@example.com","password":"`+password+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://localhost:3000")
	req.RemoteAddr = "203.0.113.7:4444"
	rec := httptest.NewRecorder()
	f.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("sign in: %d %s", rec.Code, rec.Body.String())
	}
	var metadata string
	if err := f.rig.Pool.QueryRow(context.Background(), `SELECT COALESCE(metadata,'') FROM sessions ORDER BY created_at DESC LIMIT 1`).Scan(&metadata); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(metadata, "203.0.113.7") {
		t.Fatalf("raw address stored: %s", metadata)
	}
	var m map[string]any
	_ = json.Unmarshal([]byte(metadata), &m)
	if ip, _ := m["ip_address"].(string); len(ip) != 64 {
		t.Fatalf("expected a 64-hex digest, got %q", ip)
	}
}
