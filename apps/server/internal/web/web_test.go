package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// stubAPI records that it was reached and answers JSON.
func stubAPI(t *testing.T, hits *[]string) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*hits = append(*hits, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"stub":true}`))
	})
}

func get(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestServerOwnedPathsBypassTheSPA(t *testing.T) {
	var hits []string
	h := Handler(stubAPI(t, &hits))
	for _, p := range []string{"/api", "/api/links", "/api/auth/signin", "/healthz", "/readyz", "/l/abc", "/openapi.json", "/scalar", "/images/profile/x.png"} {
		rec := get(h, p)
		if rec.Header().Get("Content-Security-Policy") != "" {
			t.Errorf("%s got SPA headers", p)
		}
		if !strings.Contains(rec.Body.String(), `"stub":true`) {
			t.Errorf("%s did not reach the API handler", p)
		}
	}
	if len(hits) != 9 {
		t.Fatalf("api hits = %d, want 9", len(hits))
	}
}

func TestSPAFallbackAndHeaders(t *testing.T) {
	h := Handler(stubAPI(t, new([]string)))
	for _, p := range []string{"/", "/app", "/app/acme/links/abc", "/reset-password", "/healthzz"} {
		rec := get(h, p)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<title>Snarvei") {
			t.Errorf("%s: status %d body %q", p, rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
			t.Errorf("%s: index Cache-Control = %q", p, got)
		}
		assertSecurityHeaders(t, rec.Header())
	}
}

func TestRobots(t *testing.T) {
	rec := get(Handler(stubAPI(t, new([]string))), "/robots.txt")
	if rec.Body.String() != "User-agent: *\nDisallow: /\n" {
		t.Fatalf("robots body = %q", rec.Body.String())
	}
	assertSecurityHeaders(t, rec.Header())
}

func TestAssetsAreImmutable(t *testing.T) {
	// The placeholder tree has only index.html; a hashed asset path that does
	// not exist must fall back to the SPA, and cacheControlFor must classify
	// real hashed assets as immutable.
	if got := cacheControlFor("assets/index-abc123.js"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("assets cache-control = %q", got)
	}
	if got := cacheControlFor("favicon.svg"); got != "public, max-age=3600" {
		t.Fatalf("root asset cache-control = %q", got)
	}
	rec := get(Handler(stubAPI(t, new([]string))), "/assets/missing.js")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<title>Snarvei") {
		t.Fatalf("missing asset should fall back to index: %d", rec.Code)
	}
}

func assertSecurityHeaders(t *testing.T, h http.Header) {
	t.Helper()
	want := map[string]string{
		"X-Content-Type-Options":       "nosniff",
		"X-Frame-Options":              "DENY",
		"Referrer-Policy":              "strict-origin-when-cross-origin",
		"Permissions-Policy":           "camera=(), microphone=(), geolocation=()",
		"Cross-Origin-Opener-Policy":   "same-origin",
		"Cross-Origin-Resource-Policy": "same-origin",
		"Content-Security-Policy":      CSP,
	}
	for k, v := range want {
		if got := h.Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
}
