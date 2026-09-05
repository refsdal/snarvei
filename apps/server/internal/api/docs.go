package api

import (
	"net/http"

	"github.com/refsdal/snarvei/server/internal/web"
)

const scalarCSP = "default-src 'self'; script-src 'self' https://cdn.jsdelivr.net; style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; img-src 'self' data: https:; font-src 'self' data: https://cdn.jsdelivr.net; connect-src 'self'; frame-ancestors 'none'"

// mountDocs serves the embedded spec as JSON and the Scalar reference page.
// Both are public, as the previous deployment's were.
func (d Deps) mountDocs(mux *http.ServeMux, specJSON []byte) {
	mux.HandleFunc("GET /openapi.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=300")
		_, _ = w.Write(specJSON)
	})
	mux.HandleFunc("GET /scalar", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", scalarCSP)
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		_, _ = w.Write(web.ScalarHTML())
	})
}
