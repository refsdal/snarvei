// Package web is the outermost HTTP layer: it serves the embedded SPA build
// with the security headers that go with it, and hands every server-owned
// path (API, probes, redirects, docs, images) to the API handler untouched.
package web

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
)

// distFS embeds the built SPA. dist/index.html is a committed placeholder;
// scripts/spa-embed-overlay.sh overlays the real Vite output before release
// binaries are built.
//
//go:embed all:dist
var distFS embed.FS

// scalarHTML is the static Scalar reference page served at GET /scalar; see
// docs.go in the api package for the CSP that goes with it.
//
//go:embed scalar.html
var scalarHTML []byte

// ScalarHTML returns the embedded Scalar reference page.
func ScalarHTML() []byte { return scalarHTML }

// CSP is the Content-Security-Policy on every non-API response. 'unsafe-inline'
// for styles is required by Emotion (MUI); data:/blob: images cover QR codes
// and profile-image previews.
const CSP = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'"

const robotsBody = "User-agent: *\nDisallow: /\n"

// serverOwnedPrefixes and serverOwnedExact are handed to the API handler
// without SPA headers: /api/, /l/ and /images/ are real API routes, and
// /openapi.json and /scalar are the API's own public docs routes.
var serverOwnedPrefixes = []string{"/api/", "/l/", "/images/"}
var serverOwnedExact = map[string]bool{"/api": true, "/healthz": true, "/readyz": true, "/openapi.json": true, "/scalar": true}

func serverOwned(path string) bool {
	if serverOwnedExact[path] {
		return true
	}
	for _, p := range serverOwnedPrefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

func securityHeaders(h http.Header) {
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
	h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
	h.Set("Cross-Origin-Opener-Policy", "same-origin")
	h.Set("Cross-Origin-Resource-Policy", "same-origin")
	h.Set("Content-Security-Policy", CSP)
}

// cacheControlFor classifies an embedded file: Vite's hashed bundles under
// assets/ never change, everything else at the root is a short-lived public file.
func cacheControlFor(name string) string {
	if strings.HasPrefix(name, "assets/") {
		return "public, max-age=31536000, immutable"
	}
	return "public, max-age=3600"
}

// Handler wraps api with static-asset serving and the SPA fallback.
func Handler(api http.Handler) http.Handler {
	assets, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(fmt.Sprintf("web: dist embed is broken: %v", err))
	}
	fileServer := http.FileServer(http.FS(assets))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serverOwned(r.URL.Path) {
			api.ServeHTTP(w, r)
			return
		}
		securityHeaders(w.Header())

		if r.URL.Path == "/robots.txt" {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte(robotsBody))
			return
		}

		name := strings.TrimPrefix(r.URL.Path, "/")
		if name != "" && name != "index.html" {
			if info, err := fs.Stat(assets, name); err == nil && !info.IsDir() {
				w.Header().Set("Cache-Control", cacheControlFor(name))
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		data, err := fs.ReadFile(assets, "index.html")
		if err != nil {
			http.Error(w, "index.html missing from embedded build", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	})
}
