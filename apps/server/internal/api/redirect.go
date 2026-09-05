package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/refsdal/snarvei/server/internal/api/middleware"
	"github.com/refsdal/snarvei/server/internal/clientip"
	"github.com/refsdal/snarvei/server/internal/redirect"
)

const (
	redirectLimit  = 100
	redirectWindow = time.Minute
)

// mountRedirect registers GET /l/{slug}: outside the session chain, rate
// limited per hashed address, never cached.
func (d Deps) mountRedirect(mux *http.ServeMux) {
	limited := middleware.RateLimit(d.mwDeps(), "redirect", redirectLimit, redirectWindow)
	noStore := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			next.ServeHTTP(w, r)
		})
	}
	mux.Handle("GET /l/{slug}", noStore(limited(http.HandlerFunc(d.followLink))))
}

func (d Deps) followLink(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	link, err := d.Q.GetActiveLinkBySlug(r.Context(), slug)
	if errors.Is(err, pgx.ErrNoRows) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("Link not found"))
		return
	}
	if err != nil {
		d.responseErrorHandler(w, r, err)
		return
	}
	status := int(link.RedirectStatus)
	http.Redirect(w, r, link.TargetUrl, status)

	if r.Method == http.MethodHead {
		// Go 1.22 GET patterns also match HEAD; link checkers and previewers
		// must not inflate click analytics.
		return
	}

	// Recorded after the redirect is written; the recorder owns the goroutine
	// so this never delays the response to the caller.
	var country *string
	if c := clientip.Country(r, d.TrustedProxyHops); c != "" {
		country = &c
	}
	d.Clicks.Record(redirect.ClickEvent{
		LinkID: link.ID, Slug: slug,
		IPHash:      d.Hasher.Hash(clientip.FromRequest(r, d.TrustedProxyHops)),
		UserAgent:   redirect.SanitizeUserAgent(r.UserAgent()),
		Referer:     redirect.SanitizeReferer(r.Referer()),
		QueryString: redirect.SanitizeQueryString(r.URL.RawQuery),
		Country:     country,
		Host:        r.Host, Path: r.URL.Path, RedirectStatus: status,
	})
}
