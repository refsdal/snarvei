package web_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/refsdal/snarvei/server/internal/api"
	"github.com/refsdal/snarvei/server/internal/testrig"
	"github.com/refsdal/snarvei/server/internal/web"
)

func composed(t *testing.T) http.Handler {
	t.Helper()
	rig := testrig.Setup(t)
	return web.Handler(api.NewHandler(api.Deps{Pool: rig.Pool, AppName: "Snarvei", OpenSignup: true, Version: "dev"}))
}

func TestProbesReachTheAPIThroughTheComposedStack(t *testing.T) {
	h := composed(t)
	for _, p := range []string{"/healthz", "/readyz", "/api/config"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
			t.Errorf("%s = %d %q", p, rec.Code, rec.Header().Get("Content-Type"))
		}
		if rec.Header().Get("Content-Security-Policy") != "" {
			t.Errorf("%s carries the SPA CSP", p)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app/anything", nil))
	if rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("SPA fallback = %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
}
