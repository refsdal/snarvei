package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/refsdal/snarvei/server/internal/api"
	"github.com/refsdal/snarvei/server/internal/auth"
	"github.com/refsdal/snarvei/server/internal/clientip"
	dbgen "github.com/refsdal/snarvei/server/internal/db/gen"
	"github.com/refsdal/snarvei/server/internal/email"
	"github.com/refsdal/snarvei/server/internal/ratelimit"
	"github.com/refsdal/snarvei/server/internal/testrig"
)

const testSecret = "snarvei-api-test-secret-0123456789012"

func handler(t *testing.T) (http.Handler, *testrig.Rig) {
	t.Helper()
	rig := testrig.Setup(t)
	return api.NewHandler(deps(t, rig, "Snarvei Test", false, "test-sha")), rig
}

// deps builds a full Deps over rig's pool: a real auth.Service backed by a
// Recording email sender, real db/gen queries and a Postgres rate limiter.
func deps(t *testing.T, rig *testrig.Rig, appName string, openSignup bool, version string) api.Deps {
	t.Helper()
	hasher := clientip.NewHasher("", testSecret)
	q := dbgen.New(rig.Pool)
	svc, err := auth.New(auth.Config{
		AppURL:     "http://localhost:3000",
		AppName:    appName,
		Secret:     testSecret,
		OpenSignup: openSignup,
		Pool:       rig.Pool,
		ClientIP:   hasher.Extractor(0),
		Email:      email.NewRecording(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return api.Deps{
		Pool:       rig.Pool,
		Q:          q,
		Auth:       svc,
		RateLimit:  ratelimit.NewPostgres(q),
		Hasher:     hasher,
		AppName:    appName,
		OpenSignup: openSignup,
		Version:    version,
	}
}

func getJSON(t *testing.T, h http.Handler, path string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("GET %s Content-Type = %q (body %q)", path, ct, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("GET %s: not JSON: %v (%q)", path, err, rec.Body.String())
	}
	return rec, body
}

func TestHealthz(t *testing.T) {
	rig := testrig.Setup(t)
	d := deps(t, rig, "x", false, "abc")
	d.Pool = nil // healthz must not touch the pool
	h := api.NewHandler(d)
	rec, body := getJSON(t, h, "/healthz")
	if rec.Code != 200 || body["ok"] != true || body["service"] != "snarvei" || body["version"] != "abc" {
		t.Fatalf("healthz = %d %v", rec.Code, body)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("healthz Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
}

func TestReadyzTracksTheDatabase(t *testing.T) {
	h, rig := handler(t)
	rec, body := getJSON(t, h, "/readyz")
	if rec.Code != 200 || body["ok"] != true {
		t.Fatalf("readyz healthy = %d %v", rec.Code, body)
	}
	rig.Pool.Close()
	rec, body = getJSON(t, h, "/readyz")
	if rec.Code != 503 || body["ok"] != false || body["error"] == "" {
		t.Fatalf("readyz after close = %d %v", rec.Code, body)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("readyz Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
}

func TestConfigIsPublic(t *testing.T) {
	h, _ := handler(t)
	rec, body := getJSON(t, h, "/api/config")
	if rec.Code != 200 || body["appName"] != "Snarvei Test" || body["openSignup"] != false {
		t.Fatalf("config = %d %v", rec.Code, body)
	}
}

func TestUnknownAPIPathIsJSON404(t *testing.T) {
	h, _ := handler(t)
	for _, p := range []string{"/api/nope", "/api", "/l/abc", "/openapi.json", "/scalar", "/images/profile/x"} {
		rec, body := getJSON(t, h, p)
		if rec.Code != 404 || body["code"] != "NOT_FOUND" {
			t.Errorf("%s = %d %v, want 404 NOT_FOUND", p, rec.Code, body)
		}
	}
}

func TestWrongMethodIsRejectedBySpec(t *testing.T) {
	h, _ := handler(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/config", strings.NewReader("{}")))
	if rec.Code != 404 && rec.Code != 405 {
		t.Fatalf("POST /api/config = %d, want 404 or 405", rec.Code)
	}
	if !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("expected a JSON error body, got %q", rec.Body.String())
	}
}

func TestGetMeWithoutSessionIsUnauthenticated(t *testing.T) {
	h, _ := handler(t)
	rec, body := getJSON(t, h, "/api/me")
	if rec.Code != http.StatusUnauthorized || body["code"] != "UNAUTHENTICATED" {
		t.Fatalf("GET /api/me anonymous = %d %v, want 401 UNAUTHENTICATED", rec.Code, body)
	}
}
