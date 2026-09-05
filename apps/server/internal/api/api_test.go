package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/refsdal/snarvei/server/internal/api"
	"github.com/refsdal/snarvei/server/internal/testrig"
)

func handler(t *testing.T) (http.Handler, *testrig.Rig) {
	t.Helper()
	rig := testrig.Setup(t)
	return api.NewHandler(api.Deps{Pool: rig.Pool, AppName: "Snarvei Test", OpenSignup: false, Version: "test-sha"}), rig
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
	h := api.NewHandler(api.Deps{Pool: nil, AppName: "x", Version: "abc"}) // nil pool: healthz must not touch it
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
