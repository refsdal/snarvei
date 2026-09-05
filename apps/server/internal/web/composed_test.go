package web_test

import (
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
	"github.com/refsdal/snarvei/server/internal/redirect"
	"github.com/refsdal/snarvei/server/internal/storage"
	"github.com/refsdal/snarvei/server/internal/testrig"
	"github.com/refsdal/snarvei/server/internal/web"
)

const composedTestSecret = "snarvei-web-composed-test-secret-0123"

func composed(t *testing.T) http.Handler {
	t.Helper()
	rig := testrig.Setup(t)
	hasher := clientip.NewHasher("", composedTestSecret)
	q := dbgen.New(rig.Pool)
	svc, err := auth.New(auth.Config{
		AppURL:     "http://localhost:3000",
		Secret:     composedTestSecret,
		OpenSignup: true,
		Pool:       rig.Pool,
		ClientIP:   hasher.Extractor(0),
		Email:      email.NewRecording(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return web.Handler(api.NewHandler(api.Deps{
		Pool:       rig.Pool,
		Q:          q,
		Auth:       svc,
		Storage:    storage.NewMemory(),
		Email:      email.NewRecording(),
		RateLimit:  ratelimit.NewPostgres(q),
		Hasher:     hasher,
		Clicks:     redirect.NewRecorder(q, nil),
		AppName:    "Snarvei",
		OpenSignup: true,
		Version:    "dev",
	}))
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
