package testrig

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/refsdal/snarvei/server/internal/api"
	"github.com/refsdal/snarvei/server/internal/auth"
	"github.com/refsdal/snarvei/server/internal/clientip"
	"github.com/refsdal/snarvei/server/internal/db/gen"
	"github.com/refsdal/snarvei/server/internal/email"
	"github.com/refsdal/snarvei/server/internal/ratelimit"
	"github.com/refsdal/snarvei/server/internal/storage"
)

const (
	appURL   = "http://127.0.0.1"
	secret   = "snarvei-testrig-auth-secret-value-0123456789"
	Password = "Testpass123"
)

// AppRig is api.NewHandler over a migrated Postgres, memory storage and a
// recording mailer, driven with real requests.
type AppRig struct {
	T       *testing.T
	Rig     *Rig
	Deps    api.Deps
	Mail    *email.Recording
	Store   *storage.Memory
	Handler http.Handler
}

// App builds the rig with open sign-up and test hooks on.
func App(t *testing.T) *AppRig {
	t.Helper()
	rig := Setup(t)
	mail := email.NewRecording()
	hasher := clientip.NewHasher("", secret)
	svc, err := auth.New(auth.Config{AppURL: appURL, AppName: "Snarvei", Secret: secret, OpenSignup: true, Pool: rig.Pool, ClientIP: hasher.Extractor(0), Email: mail})
	if err != nil {
		t.Fatalf("testrig: auth.New: %v", err)
	}
	q := gen.New(rig.Pool)
	store := storage.NewMemory()
	a := &AppRig{T: t, Rig: rig, Mail: mail, Store: store}
	a.Deps = api.Deps{
		Pool: rig.Pool, Q: q, Auth: svc, Storage: store, Email: mail, Mail: mail,
		RateLimit: ratelimit.NewPostgres(q), Hasher: hasher,
		AppURL: appURL, AppName: "Snarvei", OpenSignup: true, Version: "test", TestHooks: true,
	}
	a.Handler = api.NewHandler(a.Deps)
	return a
}

// Response is a decoded reply.
type Response struct {
	Code   int
	Header http.Header
	Body   []byte
	JSON   map[string]any
	Array  []map[string]any
}

// Do sends a request through the handler. body may be nil, a string, or any
// value (marshalled as JSON).
func (a *AppRig) Do(method, path string, body any, cookie string) Response {
	a.T.Helper()
	var reader io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		reader = strings.NewReader(b)
	default:
		raw, err := json.Marshal(b)
		if err != nil {
			a.T.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	if reader != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Origin", appURL)
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	rec := a.DoRaw(req)
	resp := Response{Code: rec.Code, Header: rec.Header(), Body: rec.Body.Bytes()}
	trimmed := bytes.TrimSpace(resp.Body)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		_ = json.Unmarshal(trimmed, &resp.JSON)
	} else if len(trimmed) > 0 && trimmed[0] == '[' {
		_ = json.Unmarshal(trimmed, &resp.Array)
	}
	return resp
}

// DoRaw serves req and returns the recorder.
func (a *AppRig) DoRaw(req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	a.Handler.ServeHTTP(rec, req)
	return rec
}

// SignUp creates a user with Password and returns its id.
func (a *AppRig) SignUp(name, addr string) string {
	a.T.Helper()
	id, err := a.Deps.Auth.CreateUser(context.Background(), name, addr, Password)
	if err != nil {
		a.T.Fatalf("testrig: SignUp(%q): %v", addr, err)
	}
	return id
}

// SignIn drives the real credential route and returns a Cookie header value.
func (a *AppRig) SignIn(addr string) string {
	a.T.Helper()
	resp := a.Do(http.MethodPost, auth.BasePath+"/signin/credential", map[string]string{"credential": addr, "password": Password}, "")
	if resp.Code != http.StatusOK {
		a.T.Fatalf("testrig: sign in %q: %d %s", addr, resp.Code, resp.Body)
	}
	for _, c := range (&http.Response{Header: resp.Header}).Cookies() {
		if c.Name == auth.SessionCookieName && c.Value != "" {
			return c.Name + "=" + c.Value
		}
	}
	a.T.Fatalf("testrig: sign in %q: no session cookie", addr)
	return ""
}

func token(cookie string) string { return strings.TrimPrefix(cookie, auth.SessionCookieName+"=") }

// NewOrg signs up ownerEmail, creates an organization and returns its id
// plus a cookie whose session has that organization active.
func (a *AppRig) NewOrg(name, slug, ownerEmail string) (string, string) {
	a.T.Helper()
	ctx := context.Background()
	ownerID := a.SignUp("Owner "+name, ownerEmail)
	org, err := a.Deps.Auth.CreateOrganization(ctx, ownerID, name, slug)
	if err != nil {
		a.T.Fatalf("testrig: CreateOrganization(%q): %v", name, err)
	}
	cookie := a.SignIn(ownerEmail)
	if err := a.Deps.Auth.SetActiveOrganization(ctx, token(cookie), org.ID); err != nil {
		a.T.Fatalf("testrig: SetActiveOrganization: %v", err)
	}
	return org.ID, cookie
}

// Join invites addr into orgID as role from ownerID, signs the invitee up,
// accepts, signs in and activates the organization.
func (a *AppRig) Join(orgID, ownerID, addr, role string) (string, string) {
	a.T.Helper()
	ctx := context.Background()
	inv, err := a.Deps.Auth.CreateInvitation(ctx, ownerID, orgID, addr, role)
	if err != nil {
		a.T.Fatalf("testrig: CreateInvitation(%q): %v", addr, err)
	}
	userID := a.SignUp("Member "+addr, addr)
	if _, err := a.Deps.Auth.AcceptInvitation(ctx, userID, inv.ID); err != nil {
		a.T.Fatalf("testrig: AcceptInvitation: %v", err)
	}
	cookie := a.SignIn(addr)
	if err := a.Deps.Auth.SetActiveOrganization(ctx, token(cookie), orgID); err != nil {
		a.T.Fatalf("testrig: SetActiveOrganization: %v", err)
	}
	return userID, cookie
}
