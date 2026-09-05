package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/refsdal/snarvei/server/internal/testrig"
)

func TestOpenAPIAndScalarArePublic(t *testing.T) {
	a := testrig.App(t)
	spec := a.Do(http.MethodGet, "/openapi.json", nil, "")
	if spec.Code != 200 || !strings.HasPrefix(spec.Header.Get("Content-Type"), "application/json") || spec.JSON["openapi"] == nil {
		t.Fatalf("openapi.json: %d %s", spec.Code, spec.Header.Get("Content-Type"))
	}
	if paths := spec.JSON["paths"].(map[string]any); paths["/api/links"] == nil || paths["/api/auth/signin/credential"] != nil {
		t.Fatalf("paths: %v", paths)
	}
	page := a.Do(http.MethodGet, "/scalar", nil, "")
	if page.Code != 200 || !strings.HasPrefix(page.Header.Get("Content-Type"), "text/html") || !strings.Contains(string(page.Body), "/openapi.json") || !strings.Contains(string(page.Body), "cdn.jsdelivr.net") {
		t.Fatalf("scalar: %d %s", page.Code, page.Body)
	}
	csp := page.Header.Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src 'self' https://cdn.jsdelivr.net") || !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Fatalf("scalar CSP: %q", csp)
	}
}
