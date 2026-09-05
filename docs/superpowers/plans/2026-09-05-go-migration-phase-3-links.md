# Go Migration Phase 3: Links, Redirect and Analytics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the product core on the Go server: links owned by teams (create with generated or custom slug, read, update with target history, delete), the public redirect `GET /l/{slug}` with async, privacy-minimised click recording and a rate limit, per-link analytics, and the public OpenAPI document with a Scalar page.

**Architecture:** Pure link rules (slug generation, custom-slug and target-URL validation) live in `internal/links`; click sanitisation and the drained async recorder live in `internal/redirect`; sqlc queries carry the tenancy scope; handlers in `internal/api` reuse the phase-2 middleware, resolving a link's team access through a shared `middleware.ResolveTeamAccess` helper because link routes carry no org or team in the path. The redirect, `/openapi.json` and `/scalar` are hand-routed (non-JSON responses); everything else is spec-first.

**Tech Stack:** Go 1.27, pgx v5, sqlc, oapi-codegen v2.8.0, kin-openapi, Playwright.

**Spec:** `docs/superpowers/specs/2026-09-04-go-backend-migration-design.md` (sections 2 (Links, `/openapi.json`, `/scalar`, error envelope), 3 (rate limits, IP hashing, logging), 4 (`links`, `link_target_history`, `click_events`), 5, 9, 11 phase 3)

## Global Constraints

- Branch `feat/go-migration-phase-3` is stacked on `feat/go-migration-phase-2` (PR #80); its PR targets that branch until #80 merges, then `main`.
- Slugs: generated slugs are 8 characters from the alphabet `ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789` (no `0OIl1`), regenerated up to 10 times on collision; custom slugs are trimmed, lower-cased, `^[a-z0-9]+(?:-[a-z0-9]+)*$`, 3..64 chars; a taken custom slug is `409 SLUG_TAKEN`; slugs never change after creation (a `slug` on update is ignored by the spec: it is not in the update body schema).
- Target URL: trimmed, 1..2048 chars, absolute `http`/`https` with a host, no embedded credentials; `javascript:`, `data:`, `file:`, `mailto:`, protocol-relative and scheme-less values are rejected with `400 VALIDATION_FAILED`.
- Redirect status is one of 301, 302, 307 (default 302). `GET /l/{slug}`: `Cache-Control: no-store` on every response; unknown or inactive slug → `404` `text/plain` body `Link not found`; rate limit `100` per `60 s` per hashed IP → `429` with `Retry-After`.
- Click events store only: link id, time, keyed IP hash (`clientip.Hasher`), user agent capped at 256 chars, referer reduced to origin + path (null when unparsable), country from `CF-IPCountry` when trusted, host, path, query string reduced to `utm_*` parameters (values capped at 200 chars, keys lower-cased, original order; null when none), redirect status used. Never a raw IP, never non-utm query parameters.
- The recorder inserts after the response is written, one goroutine per click with a 5 s context, tracked by a `sync.WaitGroup`; failures log `event=click.record_failed` with link id and slug; on shutdown `Drain(5 s)` runs after `http.Server.Shutdown`.
- Tenancy: every link route resolves the link's team and applies `authz.CanAccessTeam` (owner/admin everything, member only own teams); list filters to accessible teams; mutations check both org role and team ownership. Unknown link and inaccessible link both answer `404 NOT_FOUND` for non-members of the organization; an org member outside the team gets `403 FORBIDDEN`.
- Pagination is page-based per spec section 2: `page` ≥ 1 (default 1), `pageSize` 1..500 (default 100); responses are `{items, page, pageSize, total}`. History uses the same shape. Analytics takes `days` 1..365 (default 30) and returns `{totalClicks, uniqueVisitorApproximation, clicksByDay[{day,clicks}], topReferrers[{referer,clicks}] (10), topCountries[{country,clicks}] (10), range{from,to}}`.
- `POST /api/links` shares the `write` rate limit (30 per minute per hashed IP) with the phase-2 write endpoints.
- `GET /openapi.json` and `GET /scalar` are public; the Scalar page loads `https://cdn.jsdelivr.net/npm/@scalar/api-reference` and carries its own CSP (`default-src 'self'; script-src 'self' https://cdn.jsdelivr.net; style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; img-src 'self' data: https:; font-src 'self' data: https://cdn.jsdelivr.net; connect-src 'self'; frame-ancestors 'none'`).
- Error envelope codes used: `VALIDATION_FAILED` 400, `UNAUTHENTICATED` 401, `FORBIDDEN` 403, `NOT_FOUND` 404, `SLUG_TAKEN` 409, `RATE_LIMITED` 429, `INTERNAL` 500.
- Generated code committed; `go generate ./...` from `apps/server`; drift guards stay green. Tests: `go test -p 1 -count=1 ./...` against `TEST_DATABASE_URL`; TDD per task; Conventional Commits with the two trailers `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>` and `Claude-Session: https://claude.ai/code/session_01UdGgRFBUoiwkd9PLH7zUJE`.
- Run commands from the repo root unless a step says otherwise; Go through `mise exec --`; the test Postgres via `docker compose -f docker-compose.test.yml up -d --wait`.

---

## File Structure

```
apps/server/internal/links/{links.go,links_test.go}               slug generation, custom slug + target URL validation (pure)
apps/server/internal/redirect/{privacy.go,recorder.go,redirect_test.go}   sanitisers, ClickEvent, Recorder with Drain
apps/server/internal/db/queries/links.sql  (+ regenerated gen/)
apps/server/internal/api/middleware/middleware.go                  + ResolveTeamAccess (RequireTeam uses it)
apps/server/internal/api/{links.go,analytics.go,redirect.go,docs.go}
apps/server/internal/api/{links_test.go,analytics_test.go,redirect_test.go,docs_test.go}
apps/server/internal/web/scalar.html  (+ ScalarHTML() accessor in web.go)
apps/server/cmd/snarvei/main.go                                    recorder in buildDeps, Drain after Shutdown
openapi/snarvei.yaml (+ generated copies)                          links, history, analytics operations and schemas
e2e/links-api.spec.ts
.github/workflows/ci.yml                                           smoke: /l/nope 404 no-store, /openapi.json, /scalar
AGENTS.md                                                          banner sentence
```

---

### Task 1: Pure link rules (`internal/links`)

**Files:**
- Create: `apps/server/internal/links/links.go`, `apps/server/internal/links/links_test.go`

**Interfaces:**
- Produces:
  ```go
  package links
  const SlugLength = 8
  func GenerateSlug() string                                  // 8 chars from Alphabet
  func NormalizeCustomSlug(raw string) (string, error)        // trim + lower + validate; ErrInvalidSlug
  func ValidateTargetURL(raw string) (string, error)          // trimmed value; ErrInvalidTargetURL
  func ValidRedirectStatus(s int) bool
  var ErrInvalidSlug, ErrInvalidTargetURL error
  ```

- [ ] **Step 1: Write the failing tests**

```go
package links

import (
	"errors"
	"regexp"
	"strings"
	"testing"
)

func TestGenerateSlug(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 500; i++ {
		s := GenerateSlug()
		if len(s) != SlugLength {
			t.Fatalf("length %d: %q", len(s), s)
		}
		if strings.ContainsAny(s, "0OIl1") {
			t.Fatalf("ambiguous character in %q", s)
		}
		if !regexp.MustCompile(`^[A-Za-z2-9]+$`).MatchString(s) {
			t.Fatalf("unexpected character in %q", s)
		}
		seen[s] = true
	}
	if len(seen) < 495 {
		t.Fatalf("slugs are not random enough: %d distinct of 500", len(seen))
	}
}

func TestNormalizeCustomSlug(t *testing.T) {
	ok := map[string]string{"  Summer-2026 ": "summer-2026", "abc": "abc", "a1-b2-c3": "a1-b2-c3"}
	for in, want := range ok {
		got, err := NormalizeCustomSlug(in)
		if err != nil || got != want {
			t.Errorf("%q: got %q %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "ab", "Hello World!", "-lead", "trail-", "double--hyphen", strings.Repeat("a", 65), "under_score", "ünïcode"} {
		if _, err := NormalizeCustomSlug(bad); !errors.Is(err, ErrInvalidSlug) {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestValidateTargetURL(t *testing.T) {
	for _, good := range []string{"https://example.com", "http://example.com/path?query=1#frag", "https://sub.example.co.uk:8443/a/b", "https://xn--bcher-kva.example/", "  https://example.com/x  "} {
		if _, err := ValidateTargetURL(good); err != nil {
			t.Errorf("%q rejected: %v", good, err)
		}
	}
	if got, _ := ValidateTargetURL("  https://example.com/x  "); got != "https://example.com/x" {
		t.Errorf("not trimmed: %q", got)
	}
	for _, bad := range []string{"javascript:alert(1)", "data:text/html,<script>alert(1)</script>", "file:///etc/passwd", "intent://scan/#Intent;scheme=zxing;end", "mailto:someone@example.com", "ftp://example.com/file", "//example.com/path", "https://user:pass@example.com/", "example.com/path", "not a url", "", "   ", "https://example.com/" + strings.Repeat("a", 2048)} {
		if _, err := ValidateTargetURL(bad); !errors.Is(err, ErrInvalidTargetURL) {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestValidRedirectStatus(t *testing.T) {
	for s, want := range map[int]bool{301: true, 302: true, 307: true, 308: false, 200: false, 0: false} {
		if ValidRedirectStatus(s) != want {
			t.Errorf("%d", s)
		}
	}
}
```

- [ ] **Step 2: Run to verify failure, then write `links.go`**

```go
// Package links holds the pure rules for short links: slug generation, custom
// slug normalisation and target-URL validation. No I/O.
package links

import (
	"crypto/rand"
	"errors"
	"net/url"
	"regexp"
	"strings"
)

// Alphabet omits 0/O, I/l/1 so a printed slug cannot be misread.
const Alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"

// SlugLength is the generated slug length.
const SlugLength = 8

const (
	minSlug          = 3
	maxSlug          = 64
	maxTargetURLSize = 2048
)

var (
	ErrInvalidSlug      = errors.New("links: slug may only contain lowercase letters, digits and single hyphens (3-64 characters)")
	ErrInvalidTargetURL = errors.New("links: target URL must be an absolute http(s) URL without credentials (at most 2048 characters)")
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// GenerateSlug returns a random slug; the modulo bias over a 57-letter
// alphabet is negligible for this purpose.
func GenerateSlug() string {
	buf := make([]byte, SlugLength)
	if _, err := rand.Read(buf); err != nil {
		panic("links: crypto/rand failed: " + err.Error())
	}
	out := make([]byte, SlugLength)
	for i, b := range buf {
		out[i] = Alphabet[int(b)%len(Alphabet)]
	}
	return string(out)
}

// NormalizeCustomSlug trims, lower-cases and validates a user-chosen slug.
func NormalizeCustomSlug(raw string) (string, error) {
	slug := strings.ToLower(strings.TrimSpace(raw))
	if len(slug) < minSlug || len(slug) > maxSlug || !slugPattern.MatchString(slug) {
		return "", ErrInvalidSlug
	}
	return slug, nil
}

// ValidateTargetURL trims and checks a redirect target.
func ValidateTargetURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" || len(value) > maxTargetURLSize {
		return "", ErrInvalidTargetURL
	}
	u, err := url.Parse(value)
	if err != nil {
		return "", ErrInvalidTargetURL
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return "", ErrInvalidTargetURL
	}
	return value, nil
}

// ValidRedirectStatus reports whether s is one of the supported statuses.
func ValidRedirectStatus(s int) bool { return s == 301 || s == 302 || s == 307 }
```

Run: `cd apps/server && mise exec -- go vet ./... && mise exec -- go test ./internal/links/ -v`
Expected: 4 PASS.

- [ ] **Step 3: Commit**

```bash
git add apps/server/internal/links
git commit -m "feat(server): pure slug and target-URL rules for links"
```

---

### Task 2: Click privacy and the drained async recorder (`internal/redirect`)

**Files:**
- Create: `apps/server/internal/redirect/privacy.go`, `recorder.go`, `redirect_test.go`
- Modify: `apps/server/internal/db/queries/links.sql` is Task 3; this task needs only `InsertClick`, so add it here first (Task 3 adds the rest to the same file).

**Interfaces:**
- Produces:
  ```go
  package redirect
  func SanitizeQueryString(raw string) *string   // utm_* only, lower-cased keys, values ≤200, original order; nil when none
  func SanitizeReferer(raw string) *string       // origin + path; nil when empty/unparsable
  func SanitizeUserAgent(raw string) *string     // ≤256 chars; nil when empty
  type ClickEvent struct { LinkID, Slug, IPHash string; UserAgent, Referer, QueryString *string; Country *string; Host, Path string; RedirectStatus int }
  type Recorder struct{ ... }
  func NewRecorder(q *gen.Queries, log *slog.Logger) *Recorder
  func (r *Recorder) Record(e ClickEvent)         // async; never blocks the caller
  func (r *Recorder) Drain(timeout time.Duration) bool   // true when every in-flight insert finished
  ```

- [ ] **Step 1: Add the insert query**

`apps/server/internal/db/queries/links.sql` (new file, first query):

```sql
-- name: InsertClick :exec
INSERT INTO "click_events" ("id", "link_id", "clicked_at", "ip_hash", "user_agent", "referer", "country", "host", "path", "query_string", "redirect_status_used")
VALUES ($1, $2, now(), $3, $4, $5, $6, $7, $8, $9, $10);
```

Run: `cd apps/server && mise exec -- go generate ./... && mise exec -- go vet ./...`

- [ ] **Step 2: Write the failing tests**

```go
package redirect_test

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/refsdal/snarvei/server/internal/auth"
	"github.com/refsdal/snarvei/server/internal/db/gen"
	"github.com/refsdal/snarvei/server/internal/redirect"
	"github.com/refsdal/snarvei/server/internal/testrig"
)

func str(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

func TestSanitizers(t *testing.T) {
	cases := map[string]string{
		"utm_source=news&token=secret&UTM_Medium=email": "utm_source=news&utm_medium=email",
		"token=secret":     "<nil>",
		"":                 "<nil>",
		"utm_campaign=" + strings.Repeat("x", 300): "utm_campaign=" + strings.Repeat("x", 200),
		"utm_source=a&utm_source=b": "utm_source=a&utm_source=b",
	}
	for in, want := range cases {
		if got := str(redirect.SanitizeQueryString(in)); got != want {
			t.Errorf("query %q: got %q want %q", in, got, want)
		}
	}
	refs := map[string]string{
		"https://user:pw@example.com/path?q=1#frag": "https://example.com/path",
		"https://example.com":                      "https://example.com/",
		"not a url":                                "<nil>",
		"":                                         "<nil>",
	}
	for in, want := range refs {
		if got := str(redirect.SanitizeReferer(in)); got != want {
			t.Errorf("referer %q: got %q want %q", in, got, want)
		}
	}
	if got := str(redirect.SanitizeUserAgent(strings.Repeat("u", 300))); len(got) != 256 {
		t.Errorf("user agent not capped: %d", len(got))
	}
	if redirect.SanitizeUserAgent("") != nil {
		t.Error("empty user agent must be nil")
	}
}

func TestRecorderInsertsAndDrains(t *testing.T) {
	rig := testrig.Setup(t)
	q := gen.New(rig.Pool)
	ctx := context.Background()
	// A link needs an org and a team; insert the minimum directly.
	orgID, teamID, linkID := auth.NewID(), auth.NewID(), auth.NewID()
	if _, err := rig.Pool.Exec(ctx, `INSERT INTO organizations (id, name, slug) VALUES ($1, 'Acme', 'acme')`, orgID); err != nil {
		t.Fatal(err)
	}
	if _, err := rig.Pool.Exec(ctx, `INSERT INTO teams (id, organization_id, name) VALUES ($1, $2, 'Marketing')`, teamID, orgID); err != nil {
		t.Fatal(err)
	}
	if _, err := rig.Pool.Exec(ctx, `INSERT INTO links (id, organization_id, team_id, slug, target_url) VALUES ($1, $2, $3, 'abc12345', 'https://example.com')`, linkID, orgID, teamID); err != nil {
		t.Fatal(err)
	}

	rec := redirect.NewRecorder(q, slog.Default())
	ua := "Mozilla/5.0"
	for i := 0; i < 3; i++ {
		rec.Record(redirect.ClickEvent{LinkID: linkID, Slug: "abc12345", IPHash: strings.Repeat("a", 64), UserAgent: &ua, Host: "snarvei.test", Path: "/l/abc12345", RedirectStatus: 302})
	}
	if !rec.Drain(5 * time.Second) {
		t.Fatal("drain timed out")
	}
	var n int
	if err := rig.Pool.QueryRow(ctx, `SELECT count(*) FROM click_events WHERE link_id = $1`, linkID).Scan(&n); err != nil || n != 3 {
		t.Fatalf("clicks stored: %d %v", n, err)
	}

	// A failing insert (unknown link) is logged, not fatal, and does not block Drain.
	rec.Record(redirect.ClickEvent{LinkID: "missing", Slug: "x", IPHash: "h", Host: "h", Path: "/l/x", RedirectStatus: 302})
	if !rec.Drain(5 * time.Second) {
		t.Fatal("drain after a failed insert timed out")
	}
}
```

- [ ] **Step 3: Write `privacy.go` and `recorder.go`**

`privacy.go`:

```go
// Package redirect serves GET /l/{slug}'s click side: data-minimised click
// events and the async recorder that stores them after the redirect is sent.
package redirect

import (
	"net/url"
	"strings"
)

const (
	maxUserAgent = 256
	maxUTMValue  = 200
)

// SanitizeQueryString keeps only utm_* parameters (keys lower-cased, values
// capped), in their original order. Short links travel in campaigns whose
// query strings routinely carry personal data or tokens.
func SanitizeQueryString(raw string) *string {
	if raw == "" {
		return nil
	}
	var kept []string
	for _, pair := range strings.Split(raw, "&") {
		if pair == "" {
			continue
		}
		key, value, _ := strings.Cut(pair, "=")
		k, err := url.QueryUnescape(key)
		if err != nil {
			continue
		}
		k = strings.ToLower(k)
		if !isUTMKey(k) {
			continue
		}
		v, err := url.QueryUnescape(value)
		if err != nil {
			continue
		}
		if len(v) > maxUTMValue {
			v = v[:maxUTMValue]
		}
		kept = append(kept, url.QueryEscape(k)+"="+url.QueryEscape(v))
	}
	if len(kept) == 0 {
		return nil
	}
	out := strings.Join(kept, "&")
	return &out
}

func isUTMKey(k string) bool {
	if !strings.HasPrefix(k, "utm_") || len(k) == len("utm_") {
		return false
	}
	for _, c := range k[len("utm_"):] {
		if (c < 'a' || c > 'z') && c != '_' {
			return false
		}
	}
	return true
}

// SanitizeReferer reduces a referer to origin + path.
func SanitizeReferer(raw string) *string {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil
	}
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	out := u.Scheme + "://" + u.Host + path
	return &out
}

// SanitizeUserAgent caps the user agent.
func SanitizeUserAgent(raw string) *string {
	if raw == "" {
		return nil
	}
	if len(raw) > maxUserAgent {
		raw = raw[:maxUserAgent]
	}
	return &raw
}
```

`recorder.go`:

```go
package redirect

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/refsdal/snarvei/server/internal/auth"
	"github.com/refsdal/snarvei/server/internal/db/gen"
)

// ClickEvent is one sanitised click.
type ClickEvent struct {
	LinkID, Slug, IPHash string
	UserAgent            *string
	Referer              *string
	QueryString          *string
	Country              *string
	Host, Path           string
	RedirectStatus       int
}

// insertTimeout bounds one click insert; a stuck database must not pin
// goroutines forever.
const insertTimeout = 5 * time.Second

// Recorder stores clicks asynchronously and can be drained at shutdown.
type Recorder struct {
	q   *gen.Queries
	log *slog.Logger
	wg  sync.WaitGroup
}

// NewRecorder builds a Recorder over q.
func NewRecorder(q *gen.Queries, log *slog.Logger) *Recorder {
	if log == nil {
		log = slog.Default()
	}
	return &Recorder{q: q, log: log}
}

// Record inserts e in the background. Analytics must never break a redirect,
// so failures are logged as click.record_failed and otherwise ignored.
func (r *Recorder) Record(e ClickEvent) {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), insertTimeout)
		defer cancel()
		err := r.q.InsertClick(ctx, gen.InsertClickParams{
			ID: auth.NewID(), LinkID: e.LinkID, IpHash: e.IPHash, UserAgent: e.UserAgent, Referer: e.Referer,
			Country: e.Country, Host: e.Host, Path: e.Path, QueryString: e.QueryString, RedirectStatusUsed: int16(e.RedirectStatus),
		})
		if err != nil {
			r.log.Error("click not recorded", "event", "click.record_failed", "link", e.LinkID, "slug", e.Slug, "error", err.Error())
		}
	}()
}

// Drain waits for in-flight inserts up to timeout.
func (r *Recorder) Drain(timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}
```

Match the generated `InsertClickParams` field names/types (`IpHash` vs `IPHash`, `RedirectStatusUsed int16`).

Run: `cd apps/server && mise exec -- go vet ./... && mise exec -- go test -p 1 -count=1 ./internal/redirect/ -v`
Expected: 2 PASS.

- [ ] **Step 4: Commit**

```bash
git add apps/server
git commit -m "feat(server): click privacy sanitisers and the drained async click recorder"
```

---

### Task 3: sqlc queries for links, history and analytics

**Files:**
- Modify: `apps/server/internal/db/queries/links.sql` (append), regenerate `internal/db/gen`

- [ ] **Step 1: Append the queries**

```sql
-- name: CreateLink :exec
INSERT INTO "links" ("id", "organization_id", "team_id", "slug", "target_url", "redirect_status", "is_active", "title", "description", "created_by", "updated_by")
VALUES ($1, $2, $3, $4, $5, $6, true, $7, $8, $9, $9);

-- name: InsertLinkHistory :exec
INSERT INTO "link_target_history" ("id", "link_id", "old_target_url", "new_target_url", "changed_by")
VALUES ($1, $2, $3, $4, $5);

-- name: GetLink :one
-- One row with the team name; callers check team access before answering.
SELECT l."id", l."organization_id", l."team_id", t."name" AS team_name, l."slug", l."target_url", l."redirect_status",
    l."is_active", l."title", l."description", l."created_by", l."updated_by", l."created_at", l."updated_at"
FROM "links" l
JOIN "teams" t ON t."id" = l."team_id"
WHERE l."id" = $1;

-- name: GetActiveLinkBySlug :one
SELECT "id", "target_url", "redirect_status" FROM "links" WHERE "slug" = $1 AND "is_active";

-- name: ListLinks :many
-- Newest first within an organization, optionally restricted to a team set
-- (an org member's accessible teams) or one team.
SELECT l."id", l."organization_id", l."team_id", t."name" AS team_name, l."slug", l."target_url", l."redirect_status",
    l."is_active", l."title", l."description", l."created_by", l."updated_by", l."created_at", l."updated_at"
FROM "links" l
JOIN "teams" t ON t."id" = l."team_id"
WHERE l."organization_id" = sqlc.arg(organization_id)
  AND (sqlc.narg(team_id)::text IS NULL OR l."team_id" = sqlc.narg(team_id))
  AND (sqlc.narg(team_ids)::text[] IS NULL OR l."team_id" = ANY(sqlc.narg(team_ids)::text[]))
ORDER BY l."created_at" DESC, l."id" DESC
LIMIT sqlc.arg(page_size) OFFSET sqlc.arg(page_offset);

-- name: CountLinks :one
SELECT COUNT(*)::int FROM "links" l
WHERE l."organization_id" = sqlc.arg(organization_id)
  AND (sqlc.narg(team_id)::text IS NULL OR l."team_id" = sqlc.narg(team_id))
  AND (sqlc.narg(team_ids)::text[] IS NULL OR l."team_id" = ANY(sqlc.narg(team_ids)::text[]));

-- name: UpdateLink :exec
UPDATE "links" SET "target_url" = $2, "redirect_status" = $3, "is_active" = $4, "title" = $5, "description" = $6,
    "updated_by" = $7, "updated_at" = now()
WHERE "id" = $1;

-- name: DeleteLink :execrows
DELETE FROM "links" WHERE "id" = $1;

-- name: ListLinkHistory :many
SELECT "id", "link_id", "old_target_url", "new_target_url", "changed_by", "changed_at"
FROM "link_target_history"
WHERE "link_id" = $1
ORDER BY "changed_at" DESC, "id" DESC
LIMIT $2 OFFSET $3;

-- name: CountLinkHistory :one
SELECT COUNT(*)::int FROM "link_target_history" WHERE "link_id" = $1;

-- name: AnalyticsTotals :one
SELECT COUNT(*)::int AS total_clicks, COUNT(DISTINCT "ip_hash")::int AS unique_visitors
FROM "click_events" WHERE "link_id" = $1 AND "clicked_at" >= $2 AND "clicked_at" < $3;

-- name: AnalyticsByDay :many
SELECT to_char(date_trunc('day', "clicked_at" AT TIME ZONE 'UTC'), 'YYYY-MM-DD') AS day, COUNT(*)::int AS clicks
FROM "click_events" WHERE "link_id" = $1 AND "clicked_at" >= $2 AND "clicked_at" < $3
GROUP BY 1 ORDER BY 1;

-- name: AnalyticsTopReferers :many
SELECT "referer", COUNT(*)::int AS clicks
FROM "click_events" WHERE "link_id" = $1 AND "clicked_at" >= $2 AND "clicked_at" < $3
GROUP BY "referer" ORDER BY clicks DESC, "referer" NULLS LAST LIMIT 10;

-- name: AnalyticsTopCountries :many
SELECT "country", COUNT(*)::int AS clicks
FROM "click_events" WHERE "link_id" = $1 AND "clicked_at" >= $2 AND "clicked_at" < $3
GROUP BY "country" ORDER BY clicks DESC, "country" NULLS LAST LIMIT 10;
```

- [ ] **Step 2: Generate and verify**

Run: `cd apps/server && mise exec -- go generate ./... && mise exec -- go vet ./... && mise exec -- go test -p 1 -count=1 ./internal/db/ -run TestSqlcOutputIsUpToDate -v`
Expected: sqlc accepts the queries (if it rejects `sqlc.narg(...)::text[]`, express the team filter as two separate queries `ListLinksInTeams`/`CountLinksInTeams` taking `team_ids text[]` and drop the narg; record the change), drift guard PASS. List every generated method signature and row field in the report (Tasks 5 and 6 depend on them).

- [ ] **Step 3: Commit**

```bash
git add apps/server/internal/db
git commit -m "feat(server): sqlc queries for links, target history, redirect lookup and analytics"
```

---

### Task 4: Spec operations for links, codegen, tiers and the shared team-access resolver

**Files:**
- Modify: `openapi/snarvei.yaml`, `apps/server/internal/api/tiers.go`, `apps/server/internal/api/middleware/middleware.go`, `apps/server/internal/api/middleware/middleware_test.go`
- Create: `apps/server/internal/api/stubs.go` (temporary 501 stubs; deleted in Task 6)
- Regenerate: `internal/api/gen/*.gen.go`, `internal/api/snarvei.yaml`

**Interfaces:**
- Produces:
  ```go
  package middleware
  // ResolveTeamAccess loads the team, the caller's org role and team membership,
  // and applies authz.CanAccessTeam. Errors: ErrTeamNotFound, ErrTeamForbidden,
  // or a wrapped database error.
  func ResolveTeamAccess(ctx context.Context, d Deps, userID, teamID string) (TeamCtx, error)
  var ErrTeamNotFound, ErrTeamForbidden error
  ```
  `RequireTeam` is rewritten on top of it (same HTTP behaviour: 404 unknown, 403 forbidden). Operation tiers: `ListLinks` session, `CreateLink` session+rate-limited (`tierSessionRateLimited`), `GetLink`/`UpdateLink`/`DeleteLink`/`ListLinkHistory`/`GetLinkAnalytics` session (team access resolved in the handler).

- [ ] **Step 1: Extend `openapi/snarvei.yaml`**

Paths:

```yaml
  /api/links:
    get:
      operationId: listLinks
      summary: Links in an organization the caller can see, newest first, paged.
      tags: [links]
      parameters:
        - { name: organizationId, in: query, required: true, schema: { type: string } }
        - { name: teamId, in: query, required: false, schema: { type: string } }
        - { $ref: "#/components/parameters/Page" }
        - { $ref: "#/components/parameters/PageSize" }
      responses:
        "200": { description: A page of links., content: { application/json: { schema: { $ref: "#/components/schemas/LinkPage" } } } }
        "400": { $ref: "#/components/responses/ValidationFailed" }
        "401": { $ref: "#/components/responses/Unauthenticated" }
        "403": { $ref: "#/components/responses/Forbidden" }
        "404": { $ref: "#/components/responses/NotFound" }
    post:
      operationId: createLink
      summary: Create a link in a team. Generated slug unless a custom one is given. Rate limited.
      tags: [links]
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [teamId, targetUrl]
              properties:
                teamId: { type: string }
                targetUrl: { type: string, minLength: 1, maxLength: 2048 }
                redirectStatus: { type: integer, enum: [301, 302, 307], default: 302 }
                slug: { type: string, minLength: 3, maxLength: 64 }
                title: { type: string, maxLength: 200 }
                description: { type: string, maxLength: 2000 }
      responses:
        "201": { description: Created., content: { application/json: { schema: { $ref: "#/components/schemas/Link" } } } }
        "400": { $ref: "#/components/responses/ValidationFailed" }
        "401": { $ref: "#/components/responses/Unauthenticated" }
        "403": { $ref: "#/components/responses/Forbidden" }
        "404": { $ref: "#/components/responses/NotFound" }
        "409": { description: SLUG_TAKEN., content: { application/json: { schema: { $ref: "#/components/schemas/Error" } } } }
        "429": { $ref: "#/components/responses/RateLimited" }
  /api/links/{linkId}:
    get:
      operationId: getLink
      summary: One link.
      tags: [links]
      parameters: [{ $ref: "#/components/parameters/LinkId" }]
      responses:
        "200": { description: Link., content: { application/json: { schema: { $ref: "#/components/schemas/Link" } } } }
        "401": { $ref: "#/components/responses/Unauthenticated" }
        "403": { $ref: "#/components/responses/Forbidden" }
        "404": { $ref: "#/components/responses/NotFound" }
    patch:
      operationId: updateLink
      summary: Update target, status, activation, title or description. The slug never changes.
      tags: [links]
      parameters: [{ $ref: "#/components/parameters/LinkId" }]
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              properties:
                targetUrl: { type: string, minLength: 1, maxLength: 2048 }
                redirectStatus: { type: integer, enum: [301, 302, 307] }
                isActive: { type: boolean }
                title: { type: string, maxLength: 200, nullable: true }
                description: { type: string, maxLength: 2000, nullable: true }
      responses:
        "200": { description: Updated., content: { application/json: { schema: { $ref: "#/components/schemas/Link" } } } }
        "400": { $ref: "#/components/responses/ValidationFailed" }
        "401": { $ref: "#/components/responses/Unauthenticated" }
        "403": { $ref: "#/components/responses/Forbidden" }
        "404": { $ref: "#/components/responses/NotFound" }
    delete:
      operationId: deleteLink
      summary: Delete a link with its history and click events.
      tags: [links]
      parameters: [{ $ref: "#/components/parameters/LinkId" }]
      responses:
        "204": { description: Deleted. }
        "401": { $ref: "#/components/responses/Unauthenticated" }
        "403": { $ref: "#/components/responses/Forbidden" }
        "404": { $ref: "#/components/responses/NotFound" }
  /api/links/{linkId}/history:
    get:
      operationId: listLinkHistory
      summary: Target changes, newest first, paged.
      tags: [links]
      parameters:
        - { $ref: "#/components/parameters/LinkId" }
        - { $ref: "#/components/parameters/Page" }
        - { $ref: "#/components/parameters/PageSize" }
      responses:
        "200": { description: History page., content: { application/json: { schema: { $ref: "#/components/schemas/HistoryPage" } } } }
        "400": { $ref: "#/components/responses/ValidationFailed" }
        "401": { $ref: "#/components/responses/Unauthenticated" }
        "403": { $ref: "#/components/responses/Forbidden" }
        "404": { $ref: "#/components/responses/NotFound" }
  /api/links/{linkId}/analytics:
    get:
      operationId: getLinkAnalytics
      summary: Click analytics over the last N days.
      tags: [links]
      parameters:
        - { $ref: "#/components/parameters/LinkId" }
        - { name: days, in: query, required: false, schema: { type: integer, minimum: 1, maximum: 365, default: 30 } }
      responses:
        "200": { description: Analytics., content: { application/json: { schema: { $ref: "#/components/schemas/Analytics" } } } }
        "400": { $ref: "#/components/responses/ValidationFailed" }
        "401": { $ref: "#/components/responses/Unauthenticated" }
        "403": { $ref: "#/components/responses/Forbidden" }
        "404": { $ref: "#/components/responses/NotFound" }
```

Components (add to the existing `parameters`/`schemas`):

```yaml
    LinkId: { name: linkId, in: path, required: true, schema: { type: string } }
    Page: { name: page, in: query, required: false, schema: { type: integer, minimum: 1, default: 1 } }
    PageSize: { name: pageSize, in: query, required: false, schema: { type: integer, minimum: 1, maximum: 500, default: 100 } }
```

```yaml
    Link:
      type: object
      required: [id, organizationId, teamId, teamName, slug, targetUrl, redirectStatus, isActive, title, description, createdBy, updatedBy, createdAt, updatedAt]
      properties:
        id: { type: string }
        organizationId: { type: string }
        teamId: { type: string }
        teamName: { type: string }
        slug: { type: string }
        targetUrl: { type: string }
        redirectStatus: { type: integer, enum: [301, 302, 307] }
        isActive: { type: boolean }
        title: { type: string, nullable: true }
        description: { type: string, nullable: true }
        createdBy: { type: string, nullable: true }
        updatedBy: { type: string, nullable: true }
        createdAt: { type: string, format: date-time }
        updatedAt: { type: string, format: date-time }
    LinkPage:
      type: object
      required: [items, page, pageSize, total]
      properties:
        items: { type: array, items: { $ref: "#/components/schemas/Link" } }
        page: { type: integer }
        pageSize: { type: integer }
        total: { type: integer }
    HistoryItem:
      type: object
      required: [id, linkId, oldTargetUrl, newTargetUrl, changedBy, changedAt]
      properties:
        id: { type: string }
        linkId: { type: string }
        oldTargetUrl: { type: string, nullable: true }
        newTargetUrl: { type: string }
        changedBy: { type: string, nullable: true }
        changedAt: { type: string, format: date-time }
    HistoryPage:
      type: object
      required: [items, page, pageSize, total]
      properties:
        items: { type: array, items: { $ref: "#/components/schemas/HistoryItem" } }
        page: { type: integer }
        pageSize: { type: integer }
        total: { type: integer }
    Analytics:
      type: object
      required: [totalClicks, uniqueVisitorApproximation, clicksByDay, topReferrers, topCountries, range]
      properties:
        totalClicks: { type: integer }
        uniqueVisitorApproximation: { type: integer }
        clicksByDay:
          type: array
          items: { type: object, required: [day, clicks], properties: { day: { type: string }, clicks: { type: integer } } }
        topReferrers:
          type: array
          items: { type: object, required: [referer, clicks], properties: { referer: { type: string, nullable: true }, clicks: { type: integer } } }
        topCountries:
          type: array
          items: { type: object, required: [country, clicks], properties: { country: { type: string, nullable: true }, clicks: { type: integer } } }
        range:
          type: object
          required: [from, to]
          properties: { from: { type: string, format: date-time }, to: { type: string, format: date-time } }
```

Run: `cd apps/server && mise exec -- go generate ./...` — the strict interface gains `ListLinks`, `CreateLink`, `GetLink`, `UpdateLink`, `DeleteLink`, `ListLinkHistory`, `GetLinkAnalytics`.

- [ ] **Step 2: Add the tiers and temporary stubs**

In `tiers.go` add to `operationTiers`: `"ListLinks": tierSession, "CreateLink": tierSessionRateLimited, "GetLink": tierSession, "UpdateLink": tierSession, "DeleteLink": tierSession, "ListLinkHistory": tierSession, "GetLinkAnalytics": tierSession`. Create `stubs.go` with one `return nil, notImplemented` method per new operation (define `var notImplemented = fail(http.StatusNotImplemented, "NOT_IMPLEMENTED", "Not implemented yet")` in the file). `go vet ./...` must pass.

- [ ] **Step 3: Extract `ResolveTeamAccess` in middleware (TDD)**

Add to `middleware_test.go`:

```go
func TestResolveTeamAccess(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	ownerID, _ := f.user("Owner", "owner@example.com")
	memberID, _ := f.user("Member", "member@example.com")
	outsiderID, _ := f.user("Outsider", "outsider@example.com")
	strangerID, _ := f.user("Stranger", "stranger@example.com")
	org, _ := f.d.Auth.CreateOrganization(ctx, ownerID, "Acme", "acme")
	for _, u := range []struct{ id, mail string }{{memberID, "member@example.com"}, {outsiderID, "outsider@example.com"}} {
		inv, _ := f.d.Auth.CreateInvitation(ctx, ownerID, org.ID, u.mail, "member")
		if _, err := f.d.Auth.AcceptInvitation(ctx, u.id, inv.ID); err != nil {
			t.Fatal(err)
		}
	}
	team, _ := f.d.Q.CreateTeam(ctx, gen.CreateTeamParams{ID: auth.NewID(), OrganizationID: org.ID, Name: "Marketing"})
	_ = f.d.Q.AddTeamMember(ctx, gen.AddTeamMemberParams{TeamID: team.ID, UserID: memberID})

	if _, err := middleware.ResolveTeamAccess(ctx, f.d, ownerID, "nope"); !errors.Is(err, middleware.ErrTeamNotFound) {
		t.Fatalf("unknown team: %v", err)
	}
	for _, uid := range []string{outsiderID, strangerID} {
		if _, err := middleware.ResolveTeamAccess(ctx, f.d, uid, team.ID); !errors.Is(err, middleware.ErrTeamForbidden) {
			t.Fatalf("%s: %v", uid, err)
		}
	}
	tc, err := middleware.ResolveTeamAccess(ctx, f.d, memberID, team.ID)
	if err != nil || !tc.IsTeamMember || tc.Role != "member" || tc.OrgID != org.ID {
		t.Fatalf("member: %+v %v", tc, err)
	}
	tc, err = middleware.ResolveTeamAccess(ctx, f.d, ownerID, team.ID)
	if err != nil || tc.IsTeamMember || tc.Role != "owner" {
		t.Fatalf("owner: %+v %v", tc, err)
	}
}
```

(add `"errors"` to the test imports). Then in `middleware.go`:

```go
var (
	ErrTeamNotFound  = errors.New("middleware: team not found")
	ErrTeamForbidden = errors.New("middleware: team access denied")
)

// ResolveTeamAccess loads the team and decides whether userID may act on it.
func ResolveTeamAccess(ctx context.Context, d Deps, userID, teamID string) (TeamCtx, error) {
	team, err := d.Q.GetTeam(ctx, teamID)
	if errors.Is(err, pgx.ErrNoRows) {
		return TeamCtx{}, ErrTeamNotFound
	}
	if err != nil {
		return TeamCtx{}, fmt.Errorf("middleware: load team: %w", err)
	}
	role, err := memberRole(ctx, d.Q, team.OrganizationID, userID)
	if err != nil {
		return TeamCtx{}, fmt.Errorf("middleware: membership lookup: %w", err)
	}
	n, err := d.Q.IsTeamMember(ctx, gen.IsTeamMemberParams{TeamID: teamID, UserID: userID})
	if err != nil {
		return TeamCtx{}, fmt.Errorf("middleware: team membership lookup: %w", err)
	}
	tc := TeamCtx{TeamID: teamID, OrgID: team.OrganizationID, UserID: userID, Role: role, IsTeamMember: n > 0}
	if !authz.CanAccessTeam(tc.Role, tc.IsTeamMember) {
		return tc, ErrTeamForbidden
	}
	return tc, nil
}
```

and rewrite `RequireTeam` to call it: `ErrTeamNotFound` → 404 NOT_FOUND "Team not found", `ErrTeamForbidden` → 403 FORBIDDEN "Team access denied", other errors → 500 INTERNAL; success → context as before. Existing middleware tests must still pass.

Run: `cd apps/server && mise exec -- go vet ./... && mise exec -- go test -p 1 -count=1 ./internal/api/middleware/ ./internal/api/ -v 2>&1 | grep -E "^(--- |ok|FAIL)" | head -30`

- [ ] **Step 4: Commit**

```bash
git add openapi apps/server
git commit -m "feat(api): link, history and analytics operations in the spec; tiers; shared team-access resolver"
```

---

### Task 5: Link handlers: create, get, update with history, delete, list

**Files:**
- Create: `apps/server/internal/api/links.go`, `apps/server/internal/api/links_test.go`
- Modify: `apps/server/internal/api/stubs.go` (remove the five implemented stubs; keep history/analytics for Task 6), `apps/server/internal/api/errors.go` (map `links.ErrInvalidSlug`/`links.ErrInvalidTargetURL` → `400 VALIDATION_FAILED`, `middleware.ErrTeamNotFound` → 404, `middleware.ErrTeamForbidden` → 403)

**Interfaces:**
- Produces: `func (d Deps) linkForCaller(ctx context.Context, linkID string) (gen.GetLinkRow, middleware.TeamCtx, error)` shared by every per-link handler (Task 6 reuses it): loads the link (404), resolves team access (403/404), returns both.

- [ ] **Step 1: Write the failing tests `links_test.go`**

```go
package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/refsdal/snarvei/server/internal/testrig"
)

type linkFixture struct {
	a                     *testrig.AppRig
	orgID, teamID, other  string
	owner, member, outside string // cookies: owner; member of teamID; org member outside teamID
	stranger              string // cookie of a user in another org
}

func newLinkFixture(t *testing.T) *linkFixture {
	t.Helper()
	a := testrig.App(t)
	orgID, owner := a.NewOrg("Acme", "acme", "owner@example.com")
	ownerID := ownerIDOf(t, a, "owner@example.com")
	memberID, member := a.Join(orgID, ownerID, "member@example.com", "member")
	_, outside := a.Join(orgID, ownerID, "outside@example.com", "member")
	_, stranger := a.NewOrg("Other", "other", "stranger@example.com")
	team := a.Do(http.MethodPost, "/api/organizations/"+orgID+"/teams", map[string]string{"name": "Marketing"}, owner)
	other := a.Do(http.MethodPost, "/api/organizations/"+orgID+"/teams", map[string]string{"name": "Sales"}, owner)
	teamID := team.JSON["id"].(string)
	if resp := a.Do(http.MethodPost, "/api/teams/"+teamID+"/members", map[string]string{"userId": memberID}, owner); resp.Code != 204 {
		t.Fatalf("add member: %d %s", resp.Code, resp.Body)
	}
	return &linkFixture{a: a, orgID: orgID, teamID: teamID, other: other.JSON["id"].(string), owner: owner, member: member, outside: outside, stranger: stranger}
}

func (f *linkFixture) create(t *testing.T, cookie string, body map[string]any) testrig.Response {
	t.Helper()
	if _, ok := body["teamId"]; !ok {
		body["teamId"] = f.teamID
	}
	return f.a.Do(http.MethodPost, "/api/links", body, cookie)
}

func TestCreateLinkRules(t *testing.T) {
	f := newLinkFixture(t)
	resp := f.create(t, f.owner, map[string]any{"targetUrl": "https://example.com/launch"})
	if resp.Code != 201 {
		t.Fatalf("create: %d %s", resp.Code, resp.Body)
	}
	slug := resp.JSON["slug"].(string)
	if len(slug) != 8 || strings.ContainsAny(slug, "0OIl1") || resp.JSON["redirectStatus"] != float64(302) || resp.JSON["isActive"] != true || resp.JSON["teamName"] != "Marketing" || resp.JSON["organizationId"] != f.orgID {
		t.Fatalf("created link: %s", resp.Body)
	}
	if resp.JSON["title"] != nil || resp.JSON["createdBy"] == nil {
		t.Fatalf("defaults: %s", resp.Body)
	}
	// initial history row
	hist := f.a.Do(http.MethodGet, "/api/links/"+resp.JSON["id"].(string)+"/history", nil, f.owner)
	if hist.Code != 200 || hist.JSON["total"] != float64(1) {
		t.Fatalf("initial history: %d %s", hist.Code, hist.Body)
	}

	custom := f.create(t, f.owner, map[string]any{"targetUrl": "https://example.com/x", "slug": "  Summer-2026 ", "title": "  Campaign  ", "description": "   "})
	if custom.Code != 201 || custom.JSON["slug"] != "summer-2026" || custom.JSON["title"] != "Campaign" || custom.JSON["description"] != nil {
		t.Fatalf("custom slug: %d %s", custom.Code, custom.Body)
	}
	if dup := f.create(t, f.owner, map[string]any{"targetUrl": "https://example.com/y", "slug": "summer-2026"}); dup.Code != 409 || dup.JSON["code"] != "SLUG_TAKEN" {
		t.Fatalf("dup slug: %d %s", dup.Code, dup.Body)
	}
	// taken across organizations too
	if dup := f.a.Do(http.MethodPost, "/api/links", map[string]any{"teamId": f.strangerTeam(t), "targetUrl": "https://example.com/z", "slug": "summer-2026"}, f.stranger); dup.Code != 409 {
		t.Fatalf("cross-org slug: %d %s", dup.Code, dup.Body)
	}
	for _, bad := range []map[string]any{
		{"targetUrl": "javascript:alert(1)"}, {"targetUrl": "https://user:pw@example.com/"}, {"targetUrl": "example.com"},
		{"targetUrl": "https://example.com", "slug": "Hello World!"}, {"targetUrl": "https://example.com", "slug": "ab"},
		{"targetUrl": "https://example.com", "redirectStatus": 308},
	} {
		if resp := f.create(t, f.owner, bad); resp.Code != 400 || resp.JSON["code"] != "VALIDATION_FAILED" {
			t.Errorf("%v: %d %s", bad, resp.Code, resp.Body)
		}
	}
	if resp := f.create(t, f.owner, map[string]any{"targetUrl": "https://example.com", "teamId": "nope"}); resp.Code != 404 {
		t.Fatalf("unknown team: %d %s", resp.Code, resp.Body)
	}
	if resp := f.create(t, f.outside, map[string]any{"targetUrl": "https://example.com"}); resp.Code != 403 {
		t.Fatalf("org member outside the team: %d %s", resp.Code, resp.Body)
	}
	if resp := f.create(t, f.stranger, map[string]any{"targetUrl": "https://example.com"}); resp.Code != 403 {
		t.Fatalf("stranger: %d %s", resp.Code, resp.Body)
	}
	if resp := f.create(t, f.member, map[string]any{"targetUrl": "https://example.com"}); resp.Code != 201 {
		t.Fatalf("team member: %d %s", resp.Code, resp.Body)
	}
	if resp := f.create(t, "", map[string]any{"targetUrl": "https://example.com"}); resp.Code != 401 {
		t.Fatalf("anonymous: %d", resp.Code)
	}
}

// strangerTeam creates a team in the stranger's organization.
func (f *linkFixture) strangerTeam(t *testing.T) string {
	t.Helper()
	orgs := f.a.Do(http.MethodGet, "/api/organizations", nil, f.stranger)
	orgID := orgs.Array[0]["id"].(string)
	team := f.a.Do(http.MethodPost, "/api/organizations/"+orgID+"/teams", map[string]string{"name": "Theirs"}, f.stranger)
	return team.JSON["id"].(string)
}

func TestGetUpdateDeleteLink(t *testing.T) {
	f := newLinkFixture(t)
	created := f.create(t, f.owner, map[string]any{"targetUrl": "https://example.com/v1"})
	id := created.JSON["id"].(string)

	for cookie, want := range map[string]int{f.owner: 200, f.member: 200, f.outside: 403, f.stranger: 404, "": 401} {
		if resp := f.a.Do(http.MethodGet, "/api/links/"+id, nil, cookie); resp.Code != want {
			t.Errorf("get as %q: %d want %d", cookie[:min(12, len(cookie))], resp.Code, want)
		}
	}
	if resp := f.a.Do(http.MethodGet, "/api/links/nope", nil, f.owner); resp.Code != 404 {
		t.Fatalf("unknown: %d", resp.Code)
	}

	// title-only edit: no history row
	if resp := f.a.Do(http.MethodPatch, "/api/links/"+id, map[string]any{"title": "Renamed"}, f.member); resp.Code != 200 || resp.JSON["title"] != "Renamed" {
		t.Fatalf("patch title: %d %s", resp.Code, resp.Body)
	}
	if hist := f.a.Do(http.MethodGet, "/api/links/"+id+"/history", nil, f.owner); hist.JSON["total"] != float64(1) {
		t.Fatalf("title edit added history: %s", hist.Body)
	}
	// retarget: history row with old and new
	resp := f.a.Do(http.MethodPatch, "/api/links/"+id, map[string]any{"targetUrl": "https://example.com/v2", "redirectStatus": 307}, f.owner)
	if resp.Code != 200 || resp.JSON["targetUrl"] != "https://example.com/v2" || resp.JSON["redirectStatus"] != float64(307) {
		t.Fatalf("retarget: %d %s", resp.Code, resp.Body)
	}
	hist := f.a.Do(http.MethodGet, "/api/links/"+id+"/history", nil, f.owner)
	items := hist.JSON["items"].([]any)
	if hist.JSON["total"] != float64(2) || items[0].(map[string]any)["oldTargetUrl"] != "https://example.com/v1" || items[0].(map[string]any)["newTargetUrl"] != "https://example.com/v2" {
		t.Fatalf("history: %s", hist.Body)
	}
	// blank clears, null clears, absent keeps
	if resp := f.a.Do(http.MethodPatch, "/api/links/"+id, map[string]any{"title": "", "description": "  "}, f.owner); resp.JSON["title"] != nil || resp.JSON["description"] != nil {
		t.Fatalf("clear: %s", resp.Body)
	}
	if resp := f.a.Do(http.MethodPatch, "/api/links/"+id, map[string]any{"targetUrl": "javascript:alert(1)"}, f.owner); resp.Code != 400 {
		t.Fatalf("bad retarget: %d", resp.Code)
	}
	if got := f.a.Do(http.MethodGet, "/api/links/"+id, nil, f.owner); got.JSON["targetUrl"] != "https://example.com/v2" {
		t.Fatal("bad retarget must keep the old target")
	}
	// slug in the body is rejected by the spec (additionalProperties are allowed by default) — it must be ignored
	if resp := f.a.Do(http.MethodPatch, "/api/links/"+id, map[string]any{"slug": "changed"}, f.owner); resp.Code != 200 || resp.JSON["slug"] != created.JSON["slug"] {
		t.Fatalf("slug must not change: %d %s", resp.Code, resp.Body)
	}
	if resp := f.a.Do(http.MethodPatch, "/api/links/"+id, map[string]any{"isActive": false}, f.outside); resp.Code != 403 {
		t.Fatalf("outsider patch: %d", resp.Code)
	}
	if resp := f.a.Do(http.MethodPatch, "/api/links/"+id, map[string]any{"isActive": false}, f.stranger); resp.Code != 404 {
		t.Fatalf("stranger patch: %d", resp.Code)
	}
	if resp := f.a.Do(http.MethodDelete, "/api/links/"+id, nil, f.outside); resp.Code != 403 {
		t.Fatalf("outsider delete: %d", resp.Code)
	}
	if resp := f.a.Do(http.MethodDelete, "/api/links/"+id, nil, f.member); resp.Code != 204 {
		t.Fatalf("delete: %d %s", resp.Code, resp.Body)
	}
	if resp := f.a.Do(http.MethodGet, "/api/links/"+id, nil, f.owner); resp.Code != 404 {
		t.Fatalf("after delete: %d", resp.Code)
	}
	if resp := f.a.Do(http.MethodGet, "/api/links/"+id+"/history", nil, f.owner); resp.Code != 404 {
		t.Fatalf("history after delete: %d", resp.Code)
	}
}

func TestListLinksScopingAndPaging(t *testing.T) {
	f := newLinkFixture(t)
	for i := 0; i < 5; i++ {
		f.create(t, f.owner, map[string]any{"targetUrl": "https://example.com/team", "title": "team"})
	}
	for i := 0; i < 2; i++ {
		f.a.Do(http.MethodPost, "/api/links", map[string]any{"teamId": f.other, "targetUrl": "https://example.com/other"}, f.owner)
	}
	all := f.a.Do(http.MethodGet, "/api/links?organizationId="+f.orgID, nil, f.owner)
	if all.Code != 200 || all.JSON["total"] != float64(7) || len(all.JSON["items"].([]any)) != 7 {
		t.Fatalf("owner list: %d %s", all.Code, all.Body)
	}
	mine := f.a.Do(http.MethodGet, "/api/links?organizationId="+f.orgID, nil, f.member)
	if mine.JSON["total"] != float64(5) {
		t.Fatalf("member list: %s", mine.Body)
	}
	none := f.a.Do(http.MethodGet, "/api/links?organizationId="+f.orgID, nil, f.outside)
	if none.Code != 200 || none.JSON["total"] != float64(0) {
		t.Fatalf("outsider list: %d %s", none.Code, none.Body)
	}
	if resp := f.a.Do(http.MethodGet, "/api/links?organizationId="+f.orgID, nil, f.stranger); resp.Code != 403 {
		t.Fatalf("stranger list: %d", resp.Code)
	}
	if resp := f.a.Do(http.MethodGet, "/api/links?organizationId="+f.orgID+"&teamId="+f.other, nil, f.member); resp.Code != 403 {
		t.Fatalf("member filtering another team: %d", resp.Code)
	}
	byTeam := f.a.Do(http.MethodGet, "/api/links?organizationId="+f.orgID+"&teamId="+f.other, nil, f.owner)
	if byTeam.JSON["total"] != float64(2) {
		t.Fatalf("team filter: %s", byTeam.Body)
	}
	// paging newest first, no overlap
	seen := map[string]bool{}
	for page := 1; page <= 4; page++ {
		resp := f.a.Do(http.MethodGet, "/api/links?organizationId="+f.orgID+"&page="+string(rune('0'+page))+"&pageSize=2", nil, f.owner)
		if resp.Code != 200 || resp.JSON["page"] != float64(page) || resp.JSON["pageSize"] != float64(2) {
			t.Fatalf("page %d: %d %s", page, resp.Code, resp.Body)
		}
		for _, it := range resp.JSON["items"].([]any) {
			id := it.(map[string]any)["id"].(string)
			if seen[id] {
				t.Fatalf("duplicate %s on page %d", id, page)
			}
			seen[id] = true
		}
	}
	if len(seen) != 7 {
		t.Fatalf("paged %d of 7", len(seen))
	}
	if resp := f.a.Do(http.MethodGet, "/api/links?organizationId="+f.orgID+"&pageSize=9999", nil, f.owner); resp.Code != 400 {
		t.Fatalf("pageSize cap: %d", resp.Code)
	}
	if resp := f.a.Do(http.MethodGet, "/api/links", nil, f.owner); resp.Code != 400 {
		t.Fatalf("missing organizationId: %d", resp.Code)
	}
}
```

- [ ] **Step 2: Write `links.go`**

```go
package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/refsdal/snarvei/server/internal/api/gen"
	"github.com/refsdal/snarvei/server/internal/api/middleware"
	"github.com/refsdal/snarvei/server/internal/auth"
	"github.com/refsdal/snarvei/server/internal/authz"
	"github.com/refsdal/snarvei/server/internal/db"
	dbgen "github.com/refsdal/snarvei/server/internal/db/gen"
	"github.com/refsdal/snarvei/server/internal/links"
)

const (
	maxSlugAttempts = 10
	maxTitle        = 200
	maxDescription  = 2000
	defaultPageSize = 100
	maxPageSize     = 500
)

func pageParams(page, pageSize *int) (p int, size int, offset int32, limit int32, err error) {
	p, size = 1, defaultPageSize
	if page != nil {
		p = *page
	}
	if pageSize != nil {
		size = *pageSize
	}
	if p < 1 || size < 1 || size > maxPageSize {
		return 0, 0, 0, 0, fail(http.StatusBadRequest, "VALIDATION_FAILED", "page must be >= 1 and pageSize 1..500")
	}
	return p, size, int32((p - 1) * size), int32(size), nil
}

func toLink(r dbgen.GetLinkRow) gen.Link {
	return gen.Link{
		Id: r.ID, OrganizationId: r.OrganizationID, TeamId: r.TeamID, TeamName: r.TeamName, Slug: r.Slug, TargetUrl: r.TargetUrl,
		RedirectStatus: gen.LinkRedirectStatus(r.RedirectStatus), IsActive: r.IsActive, Title: r.Title, Description: r.Description,
		CreatedBy: r.CreatedBy, UpdatedBy: r.UpdatedBy, CreatedAt: ts(r.CreatedAt), UpdatedAt: ts(r.UpdatedAt),
	}
}

// linkForCaller loads a link and checks the caller may act on its team.
// Non-members of the organization get NOT_FOUND (existence is not revealed);
// org members outside the team get FORBIDDEN.
func (d Deps) linkForCaller(ctx context.Context, linkID string) (dbgen.GetLinkRow, middleware.TeamCtx, error) {
	s := middleware.SessionFromContext(ctx)
	row, err := d.Q.GetLink(ctx, linkID)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, middleware.TeamCtx{}, fail(http.StatusNotFound, "NOT_FOUND", "Link not found")
	}
	if err != nil {
		return row, middleware.TeamCtx{}, err
	}
	tc, err := middleware.ResolveTeamAccess(ctx, d.mwDeps(), s.UserID, row.TeamID)
	if errors.Is(err, middleware.ErrTeamForbidden) {
		if tc.Role == "" {
			return row, tc, fail(http.StatusNotFound, "NOT_FOUND", "Link not found")
		}
		return row, tc, fail(http.StatusForbidden, "FORBIDDEN", "Team access denied")
	}
	if err != nil {
		return row, tc, err
	}
	return row, tc, nil
}

// optionalText trims and turns blank into nil (create) or keeps nil (update).
func optionalText(v *string, max int) (*string, error) {
	if v == nil {
		return nil, nil
	}
	t := strings.TrimSpace(*v)
	if len(t) > max {
		return nil, fail(http.StatusBadRequest, "VALIDATION_FAILED", "Text is too long")
	}
	if t == "" {
		return nil, nil
	}
	return &t, nil
}

func (d Deps) CreateLink(ctx context.Context, req gen.CreateLinkRequestObject) (gen.CreateLinkResponseObject, error) {
	s := middleware.SessionFromContext(ctx)
	tc, err := middleware.ResolveTeamAccess(ctx, d.mwDeps(), s.UserID, req.Body.TeamId)
	if errors.Is(err, middleware.ErrTeamNotFound) {
		return nil, fail(http.StatusNotFound, "NOT_FOUND", "Team not found")
	}
	if errors.Is(err, middleware.ErrTeamForbidden) {
		return nil, fail(http.StatusForbidden, "FORBIDDEN", "Team access denied")
	}
	if err != nil {
		return nil, err
	}
	target, err := links.ValidateTargetURL(req.Body.TargetUrl)
	if err != nil {
		return nil, err
	}
	status := 302
	if req.Body.RedirectStatus != nil {
		status = int(*req.Body.RedirectStatus)
	}
	if !links.ValidRedirectStatus(status) {
		return nil, fail(http.StatusBadRequest, "VALIDATION_FAILED", "redirectStatus must be 301, 302 or 307")
	}
	title, err := optionalText(req.Body.Title, maxTitle)
	if err != nil {
		return nil, err
	}
	description, err := optionalText(req.Body.Description, maxDescription)
	if err != nil {
		return nil, err
	}
	custom := ""
	if req.Body.Slug != nil {
		if custom, err = links.NormalizeCustomSlug(*req.Body.Slug); err != nil {
			return nil, err
		}
	}

	id := auth.NewID()
	for attempt := 0; ; attempt++ {
		slug := custom
		if slug == "" {
			slug = links.GenerateSlug()
		}
		err := d.inTx(ctx, func(q *dbgen.Queries) error {
			if err := q.CreateLink(ctx, dbgen.CreateLinkParams{ID: id, OrganizationID: tc.OrgID, TeamID: tc.TeamID, Slug: slug, TargetUrl: target, RedirectStatus: int16(status), Title: title, Description: description, CreatedBy: &s.UserID}); err != nil {
				return err
			}
			return q.InsertLinkHistory(ctx, dbgen.InsertLinkHistoryParams{ID: auth.NewID(), LinkID: id, OldTargetUrl: nil, NewTargetUrl: target, ChangedBy: &s.UserID})
		})
		if err == nil {
			break
		}
		if db.IsUniqueViolation(err) {
			if custom != "" {
				return nil, fail(http.StatusConflict, "SLUG_TAKEN", "That slug is already taken")
			}
			if attempt < maxSlugAttempts-1 {
				continue
			}
		}
		return nil, err
	}
	row, err := d.Q.GetLink(ctx, id)
	if err != nil {
		return nil, err
	}
	return gen.CreateLink201JSONResponse(toLink(row)), nil
}

// inTx runs fn in one transaction with a transaction-bound Queries.
func (d Deps) inTx(ctx context.Context, fn func(q *dbgen.Queries) error) error {
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(dbgen.New(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (d Deps) GetLink(ctx context.Context, req gen.GetLinkRequestObject) (gen.GetLinkResponseObject, error) {
	row, _, err := d.linkForCaller(ctx, req.LinkId)
	if err != nil {
		return nil, err
	}
	return gen.GetLink200JSONResponse(toLink(row)), nil
}

func (d Deps) UpdateLink(ctx context.Context, req gen.UpdateLinkRequestObject) (gen.UpdateLinkResponseObject, error) {
	s := middleware.SessionFromContext(ctx)
	row, _, err := d.linkForCaller(ctx, req.LinkId)
	if err != nil {
		return nil, err
	}
	target := row.TargetUrl
	if req.Body.TargetUrl != nil {
		if target, err = links.ValidateTargetURL(*req.Body.TargetUrl); err != nil {
			return nil, err
		}
	}
	status := int(row.RedirectStatus)
	if req.Body.RedirectStatus != nil {
		status = int(*req.Body.RedirectStatus)
		if !links.ValidRedirectStatus(status) {
			return nil, fail(http.StatusBadRequest, "VALIDATION_FAILED", "redirectStatus must be 301, 302 or 307")
		}
	}
	active := row.IsActive
	if req.Body.IsActive != nil {
		active = *req.Body.IsActive
	}
	title, description := row.Title, row.Description
	if req.Body.Title != nil { // present (string or JSON null): blank/null clears
		if title, err = optionalText(req.Body.Title, maxTitle); err != nil {
			return nil, err
		}
	}
	if req.Body.Description != nil {
		if description, err = optionalText(req.Body.Description, maxDescription); err != nil {
			return nil, err
		}
	}
	err = d.inTx(ctx, func(q *dbgen.Queries) error {
		if err := q.UpdateLink(ctx, dbgen.UpdateLinkParams{ID: row.ID, TargetUrl: target, RedirectStatus: int16(status), IsActive: active, Title: title, Description: description, UpdatedBy: &s.UserID}); err != nil {
			return err
		}
		if target != row.TargetUrl {
			old := row.TargetUrl
			return q.InsertLinkHistory(ctx, dbgen.InsertLinkHistoryParams{ID: auth.NewID(), LinkID: row.ID, OldTargetUrl: &old, NewTargetUrl: target, ChangedBy: &s.UserID})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	updated, err := d.Q.GetLink(ctx, row.ID)
	if err != nil {
		return nil, err
	}
	return gen.UpdateLink200JSONResponse(toLink(updated)), nil
}

func (d Deps) DeleteLink(ctx context.Context, req gen.DeleteLinkRequestObject) (gen.DeleteLinkResponseObject, error) {
	row, _, err := d.linkForCaller(ctx, req.LinkId)
	if err != nil {
		return nil, err
	}
	if _, err := d.Q.DeleteLink(ctx, row.ID); err != nil {
		return nil, err
	}
	return gen.DeleteLink204Response{}, nil
}

func (d Deps) ListLinks(ctx context.Context, req gen.ListLinksRequestObject) (gen.ListLinksResponseObject, error) {
	s := middleware.SessionFromContext(ctx)
	page, size, offset, limit, err := pageParams(req.Params.Page, req.Params.PageSize)
	if err != nil {
		return nil, err
	}
	orgID := req.Params.OrganizationId
	roles, err := d.Q.GetMemberRoles(ctx, dbgen.GetMemberRolesParams{OrganizationID: orgID, UserID: s.UserID})
	if err != nil {
		return nil, err
	}
	role := authz.Highest(roles)
	if role == "" {
		return nil, fail(http.StatusForbidden, "FORBIDDEN", "Organization access denied")
	}
	var teamID *string
	var teamIDs []string
	if req.Params.TeamId != nil && *req.Params.TeamId != "" {
		tc, err := middleware.ResolveTeamAccess(ctx, d.mwDeps(), s.UserID, *req.Params.TeamId)
		if errors.Is(err, middleware.ErrTeamNotFound) || (err == nil && tc.OrgID != orgID) {
			return nil, fail(http.StatusNotFound, "NOT_FOUND", "Team not found")
		}
		if errors.Is(err, middleware.ErrTeamForbidden) {
			return nil, fail(http.StatusForbidden, "FORBIDDEN", "Team access denied")
		}
		if err != nil {
			return nil, err
		}
		teamID = req.Params.TeamId
	} else if !authz.IsOrgAdmin(role) {
		ids, err := d.Q.ListAccessibleTeamIDs(ctx, dbgen.ListAccessibleTeamIDsParams{OrganizationID: orgID, UserID: s.UserID})
		if err != nil {
			return nil, err
		}
		if len(ids) == 0 {
			return gen.ListLinks200JSONResponse{Items: []gen.Link{}, Page: page, PageSize: size, Total: 0}, nil
		}
		teamIDs = ids
	}
	rows, err := d.Q.ListLinks(ctx, dbgen.ListLinksParams{OrganizationID: orgID, TeamID: teamID, TeamIds: teamIDs, PageSize: limit, PageOffset: offset})
	if err != nil {
		return nil, err
	}
	total, err := d.Q.CountLinks(ctx, dbgen.CountLinksParams{OrganizationID: orgID, TeamID: teamID, TeamIds: teamIDs})
	if err != nil {
		return nil, err
	}
	items := make([]gen.Link, 0, len(rows))
	for _, r := range rows {
		items = append(items, toLink(dbgen.GetLinkRow(r)))
	}
	return gen.ListLinks200JSONResponse{Items: items, Page: page, PageSize: size, Total: int(total)}, nil
}
```

Notes: `dbgen.GetLinkRow(r)` converts `ListLinksRow` to `GetLinkRow` when the column lists are identical (they are by construction); if sqlc emitted different field orders, write a small `rowToLink` for the list row instead. Match generated names (`TeamIds` vs `TeamIDs`, nullable param types from `sqlc.narg`, `RedirectStatus int16`, `gen.LinkRedirectStatus`). Also add to `errors.go`'s `classify`: `links.ErrInvalidSlug` and `links.ErrInvalidTargetURL` → `400 VALIDATION_FAILED` with the error's message; `middleware.ErrTeamNotFound` → 404; `middleware.ErrTeamForbidden` → 403.

- [ ] **Step 3: Run**

Run: `cd apps/server && mise exec -- go vet ./... && mise exec -- go test -p 1 -count=1 ./internal/api/ -run 'TestCreateLinkRules|TestGetUpdateDeleteLink|TestListLinksScopingAndPaging' -v 2>&1 | tail -20`
Expected: 3 PASS (`ListLinkHistory` still stubbed at this point returns 501 — the history assertions in these tests need Task 6; to keep TDD honest, implement `ListLinkHistory` in THIS task as part of `links.go` (it is a link handler) and remove its stub: `pageParams`, `linkForCaller`, then `ListLinkHistory`/`CountLinkHistory` → `gen.ListLinkHistory200JSONResponse{Items, Page, PageSize, Total}` with `HistoryItem{Id, LinkId, OldTargetUrl, NewTargetUrl, ChangedBy, ChangedAt}`).

- [ ] **Step 4: Commit**

```bash
git add apps/server
git commit -m "feat(api): link CRUD with generated or custom slugs, target history and scoped paging"
```

---

### Task 6: Analytics, the public redirect with click recording, OpenAPI and Scalar routes, shutdown drain

**Files:**
- Create: `apps/server/internal/api/analytics.go`, `redirect.go`, `docs.go`, `apps/server/internal/web/scalar.html`, tests `analytics_test.go`, `redirect_test.go`, `docs_test.go`
- Modify: `apps/server/internal/api/api.go` (Deps gains `Clicks *redirect.Recorder`; `NewHandler` mounts `d.mountRedirect(mux)` and `d.mountDocs(mux)`; the JSON 404 for `/l/*`, `/openapi.json`, `/scalar` disappears because real routes exist), `apps/server/internal/web/web.go` (`//go:embed scalar.html` + `func ScalarHTML() []byte`), `apps/server/internal/testrig/http.go` (`Clicks: redirect.NewRecorder(q, nil)` in `App`, and the rig exposes it so tests can `Drain`), `apps/server/cmd/snarvei/main.go` (recorder in `buildDeps`; after `srv.Shutdown`, `deps.Clicks.Drain(5*time.Second)` with a log line when it times out), delete `stubs.go`.

- [ ] **Step 1: Write the failing tests**

`analytics_test.go`:

```go
package api_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/refsdal/snarvei/server/internal/testrig"
)

func TestAnalytics(t *testing.T) {
	f := newLinkFixture(t)
	created := f.create(t, f.owner, map[string]any{"targetUrl": "https://example.com"})
	id := created.JSON["id"].(string)
	ctx := context.Background()
	insert := func(age time.Duration, ip, referer, country string) {
		t.Helper()
		var ref, ctry *string
		if referer != "" {
			ref = &referer
		}
		if country != "" {
			ctry = &country
		}
		if _, err := f.a.Rig.Pool.Exec(ctx, `INSERT INTO click_events (id, link_id, clicked_at, ip_hash, referer, country, host, path, redirect_status_used) VALUES (gen_random_uuid()::text, $1, now() - $2::interval, $3, $4, $5, 'h', '/l/x', 302)`,
			id, age.String(), ip, ref, ctry); err != nil {
			t.Fatal(err)
		}
	}
	insert(time.Hour, "ip1", "https://news.example/a", "NO")
	insert(2*time.Hour, "ip1", "https://news.example/a", "NO")
	insert(48*time.Hour, "ip2", "", "SE")
	insert(40*24*time.Hour, "ip3", "https://old.example/", "DE") // outside the default 30 days

	resp := f.a.Do(http.MethodGet, "/api/links/"+id+"/analytics", nil, f.member)
	if resp.Code != 200 || resp.JSON["totalClicks"] != float64(3) || resp.JSON["uniqueVisitorApproximation"] != float64(2) {
		t.Fatalf("analytics: %d %s", resp.Code, resp.Body)
	}
	refs := resp.JSON["topReferrers"].([]any)
	if refs[0].(map[string]any)["referer"] != "https://news.example/a" || refs[0].(map[string]any)["clicks"] != float64(2) {
		t.Fatalf("referrers: %s", resp.Body)
	}
	countries := resp.JSON["topCountries"].([]any)
	if countries[0].(map[string]any)["country"] != "NO" {
		t.Fatalf("countries: %s", resp.Body)
	}
	days := resp.JSON["clicksByDay"].([]any)
	sum := 0.0
	for _, d := range days {
		sum += d.(map[string]any)["clicks"].(float64)
	}
	if sum != 3 {
		t.Fatalf("clicksByDay: %s", resp.Body)
	}
	rng := resp.JSON["range"].(map[string]any)
	from, _ := time.Parse(time.RFC3339, rng["from"].(string))
	if time.Since(from) < 29*24*time.Hour || time.Since(from) > 31*24*time.Hour {
		t.Fatalf("range.from: %v", from)
	}

	wide := f.a.Do(http.MethodGet, "/api/links/"+id+"/analytics?days=365", nil, f.owner)
	if wide.JSON["totalClicks"] != float64(4) {
		t.Fatalf("365 days: %s", wide.Body)
	}
	for _, bad := range []string{"0", "366", "x"} {
		if resp := f.a.Do(http.MethodGet, "/api/links/"+id+"/analytics?days="+bad, nil, f.owner); resp.Code != 400 {
			t.Errorf("days=%s: %d", bad, resp.Code)
		}
	}
	if resp := f.a.Do(http.MethodGet, "/api/links/"+id+"/analytics", nil, f.outside); resp.Code != 403 {
		t.Fatalf("outsider: %d", resp.Code)
	}
	if resp := f.a.Do(http.MethodGet, "/api/links/"+id+"/analytics", nil, f.stranger); resp.Code != 404 {
		t.Fatalf("stranger: %d", resp.Code)
	}
}
```

`redirect_test.go`:

```go
package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/refsdal/snarvei/server/internal/testrig"
)

func follow(a *testrig.AppRig, path string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = "203.0.113.5:1234"
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return a.DoRaw(req)
}

func TestRedirectAndClickRecording(t *testing.T) {
	f := newLinkFixture(t)
	created := f.create(t, f.owner, map[string]any{"targetUrl": "https://example.com/landing", "redirectStatus": 307})
	slug, id := created.JSON["slug"].(string), created.JSON["id"].(string)

	rec := follow(f.a, "/l/"+slug+"?utm_source=news&token=secret", map[string]string{"User-Agent": "UA/1.0", "Referer": "https://ref.example/page?x=1", "CF-IPCountry": "NO"})
	if rec.Code != 307 || rec.Header().Get("Location") != "https://example.com/landing" || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("redirect: %d %q %q", rec.Code, rec.Header().Get("Location"), rec.Header().Get("Cache-Control"))
	}
	if !f.a.Clicks.Drain(5 * time.Second) {
		t.Fatal("drain")
	}
	var ipHash, ua, ref, qs, host, path string
	var country *string
	var status int
	err := f.a.Rig.Pool.QueryRow(context.Background(), `SELECT ip_hash, COALESCE(user_agent,''), COALESCE(referer,''), COALESCE(query_string,''), country, host, path, redirect_status_used FROM click_events WHERE link_id = $1`, id).
		Scan(&ipHash, &ua, &ref, &qs, &country, &host, &path, &status)
	if err != nil {
		t.Fatal(err)
	}
	if len(ipHash) != 64 || ipHash == "203.0.113.5" || ua != "UA/1.0" || ref != "https://ref.example/page" || qs != "utm_source=news" || path != "/l/"+slug || status != 307 {
		t.Fatalf("click row: %q %q %q %q %q %d", ipHash, ua, ref, qs, path, status)
	}
	if country != nil { // TrustedProxyHops is 0 in the rig: CF-IPCountry is untrusted
		t.Fatalf("country must be null when no proxy is trusted: %v", *country)
	}

	miss := follow(f.a, "/l/doesnotexist", nil)
	if miss.Code != 404 || miss.Header().Get("Cache-Control") != "no-store" || miss.Body.String() != "Link not found" || miss.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Fatalf("miss: %d %q %q", miss.Code, miss.Header().Get("Cache-Control"), miss.Body.String())
	}
	f.a.Do(http.MethodPatch, "/api/links/"+id, map[string]any{"isActive": false}, f.owner)
	if rec := follow(f.a, "/l/"+slug, nil); rec.Code != 404 {
		t.Fatalf("inactive: %d", rec.Code)
	}
	f.a.Clicks.Drain(5 * time.Second)
	var n int
	_ = f.a.Rig.Pool.QueryRow(context.Background(), `SELECT count(*) FROM click_events WHERE link_id = $1`, id).Scan(&n)
	if n != 1 {
		t.Fatalf("inactive link must record no click: %d", n)
	}
	f.a.Do(http.MethodDelete, "/api/links/"+id, nil, f.owner)
	_ = f.a.Rig.Pool.QueryRow(context.Background(), `SELECT count(*) FROM click_events WHERE link_id = $1`, id).Scan(&n)
	if n != 0 {
		t.Fatal("deleting the link must delete its clicks")
	}
}

func TestRedirectIsRateLimited(t *testing.T) {
	f := newLinkFixture(t)
	for i := 0; i <= 100; i++ {
		rec := follow(f.a, "/l/nothing", nil)
		if i < 100 && rec.Code != 404 {
			t.Fatalf("hit %d: %d", i, rec.Code)
		}
		if i == 100 && (rec.Code != 429 || rec.Header().Get("Retry-After") == "") {
			t.Fatalf("hit 101: %d", rec.Code)
		}
	}
}
```

`docs_test.go`:

```go
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
```

- [ ] **Step 2: Write `analytics.go`**

```go
package api

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/refsdal/snarvei/server/internal/api/gen"
	dbgen "github.com/refsdal/snarvei/server/internal/db/gen"
)

const defaultAnalyticsDays = 30

func (d Deps) GetLinkAnalytics(ctx context.Context, req gen.GetLinkAnalyticsRequestObject) (gen.GetLinkAnalyticsResponseObject, error) {
	row, _, err := d.linkForCaller(ctx, req.LinkId)
	if err != nil {
		return nil, err
	}
	days := defaultAnalyticsDays
	if req.Params.Days != nil {
		days = *req.Params.Days
	}
	if days < 1 || days > 365 {
		return nil, fail(400, "VALIDATION_FAILED", "days must be 1..365")
	}
	to := time.Now().UTC()
	from := to.Add(-time.Duration(days) * 24 * time.Hour)
	tz := func(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

	totals, err := d.Q.AnalyticsTotals(ctx, dbgen.AnalyticsTotalsParams{LinkID: row.ID, ClickedAt: tz(from), ClickedAt_2: tz(to)})
	if err != nil {
		return nil, err
	}
	byDay, err := d.Q.AnalyticsByDay(ctx, dbgen.AnalyticsByDayParams{LinkID: row.ID, ClickedAt: tz(from), ClickedAt_2: tz(to)})
	if err != nil {
		return nil, err
	}
	refs, err := d.Q.AnalyticsTopReferers(ctx, dbgen.AnalyticsTopReferersParams{LinkID: row.ID, ClickedAt: tz(from), ClickedAt_2: tz(to)})
	if err != nil {
		return nil, err
	}
	countries, err := d.Q.AnalyticsTopCountries(ctx, dbgen.AnalyticsTopCountriesParams{LinkID: row.ID, ClickedAt: tz(from), ClickedAt_2: tz(to)})
	if err != nil {
		return nil, err
	}
	out := gen.Analytics{TotalClicks: int(totals.TotalClicks), UniqueVisitorApproximation: int(totals.UniqueVisitors)}
	out.ClicksByDay = make([]struct {
		Clicks int    `json:"clicks"`
		Day    string `json:"day"`
	}, 0, len(byDay))
	// Use the generated anonymous struct types exactly as oapi-codegen emitted
	// them (open types.gen.go); the three loops below are shape-for-shape.
	for _, r := range byDay {
		out.ClicksByDay = append(out.ClicksByDay, struct {
			Clicks int    `json:"clicks"`
			Day    string `json:"day"`
		}{Clicks: int(r.Clicks), Day: r.Day})
	}
	for _, r := range refs {
		out.TopReferrers = append(out.TopReferrers, struct {
			Clicks  int     `json:"clicks"`
			Referer *string `json:"referer"`
		}{Clicks: int(r.Clicks), Referer: r.Referer})
	}
	for _, r := range countries {
		out.TopCountries = append(out.TopCountries, struct {
			Clicks  int     `json:"clicks"`
			Country *string `json:"country"`
		}{Clicks: int(r.Clicks), Country: r.Country})
	}
	if out.TopReferrers == nil {
		out.TopReferrers = []struct {
			Clicks  int     `json:"clicks"`
			Referer *string `json:"referer"`
		}{}
	}
	if out.TopCountries == nil {
		out.TopCountries = []struct {
			Clicks  int     `json:"clicks"`
			Country *string `json:"country"`
		}{}
	}
	out.Range.From, out.Range.To = from, to
	return gen.GetLinkAnalytics200JSONResponse(out), nil
}
```

The anonymous struct literal shapes must match `types.gen.go` exactly (field order and tags as generated); if oapi-codegen named them (it may emit `gen.Analytics_ClicksByDay_Item`), use the named types.

- [ ] **Step 3: Write `redirect.go`, `docs.go`, `scalar.html`, and wire `web.ScalarHTML`**

`redirect.go`:

```go
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
	mux.Handle("GET /l/{slug}", limited(http.HandlerFunc(d.followLink)))
}

func (d Deps) followLink(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
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

	// Recorded after the redirect is written; the recorder owns the goroutine.
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
```

`http.Redirect` with a 3xx writes a small HTML body for GET; that is fine (spec only fixes status, `Location` and `no-store`).

`docs.go`:

```go
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
```

In `NewHandler`: after `loadSpec()`, `specJSON, err := spec.MarshalJSON()` (panic on error), then `d.mountRedirect(mux)` and `d.mountDocs(mux, specJSON)` next to the image mount. Add `Clicks *redirect.Recorder` to `Deps` and include it in the nil assertion.

`apps/server/internal/web/scalar.html`:

```html
<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <title>Snarvei API reference</title>
    <meta name="viewport" content="width=device-width, initial-scale=1" />
  </head>
  <body>
    <script id="api-reference" data-url="/openapi.json"></script>
    <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
  </body>
</html>
```

`web.go`: add `//go:embed scalar.html` `var scalarHTML []byte` and `func ScalarHTML() []byte { return scalarHTML }`. Remove `/openapi.json` and `/scalar` from the web package comment about JSON 404s (they are real routes now) — the `serverOwned` sets stay.

`testrig/http.go`: `App` builds `clicks := redirect.NewRecorder(q, nil)`, sets `Deps.Clicks: clicks` and exposes `Clicks *redirect.Recorder` on `AppRig`.

`cmd/snarvei/main.go`: in `buildDeps`, `Clicks: redirect.NewRecorder(q, slog.Default())`; in `serve`, after `srv.Shutdown(ctx)`: `if !deps.Clicks.Drain(5 * time.Second) { slog.Warn("click recorder drain timed out", "event", "click.drain_timeout") }`.

Delete `stubs.go`. Update `api_test.go`'s `TestUnknownAPIPathIsJSON404`: `/l/abc`, `/openapi.json`, `/scalar` are no longer 404 JSON (drop them from that list; `/api/nope` and `/images/profile/x` stay).

- [ ] **Step 4: Run everything**

Run: `cd apps/server && mise exec -- go vet ./... && mise exec -- go test -p 1 -count=1 ./... 2>&1 | grep -E "^(--- FAIL|FAIL|ok)" | head -30`
Expected: all packages ok. Then the manual check: boot the server as in phase 2 Task 11, create a link through the API and `curl -i localhost:3000/l/<slug>` → `307`/`302` with `Location` and `cache-control: no-store`; `curl -s localhost:3000/openapi.json | head -c 100`; `curl -sI localhost:3000/scalar | grep -i content-security-policy`.

- [ ] **Step 5: Commit**

```bash
git add apps/server
git commit -m "feat(server): public redirect with privacy-minimised click recording, per-link analytics, OpenAPI and Scalar routes"
```

---

### Task 7: Playwright link flows, CI smoke lines, docs note, commit

**Files:**
- Create: `e2e/links-api.spec.ts`
- Modify: `.github/workflows/ci.yml` (smoke), `AGENTS.md` (banner sentence)

- [ ] **Step 1: Write `e2e/links-api.spec.ts`**

```ts
import { expect, test, type APIRequestContext } from "@playwright/test";

const unique = () => Math.random().toString(36).slice(2, 10);
const PASSWORD = "Playwright123";
const ORIGIN = process.env.E2E_BASE_URL ?? "http://127.0.0.1:3300";
const headers = { origin: ORIGIN, "content-type": "application/json" };

async function signUp(request: APIRequestContext, name: string, email: string) {
  for (let attempt = 0; attempt < 8; attempt++) {
    const res = await request.post("/api/auth/signup/credential", { headers, data: { name, email, password: PASSWORD } });
    if (res.status() !== 429) {
      expect(res.status(), await res.text()).toBe(200);
      return;
    }
    const retryAfter = Number(res.headers()["retry-after"]);
    await new Promise((r) => setTimeout(r, (Number.isFinite(retryAfter) && retryAfter > 0 ? retryAfter : 2.5) * 1000));
  }
  throw new Error("sign-up kept being throttled");
}

async function workspace(request: APIRequestContext) {
  await signUp(request, "Owner", `owner-${unique()}@example.com`);
  const org = await (await request.post("/api/organizations", { headers, data: { name: "Acme", slug: `acme-${unique()}` } })).json();
  await request.post(`/api/organizations/${org.id}/switch`, { headers });
  const team = await (await request.post(`/api/organizations/${org.id}/teams`, { headers, data: { name: "Marketing" } })).json();
  return { orgId: org.id as string, teamId: team.id as string };
}

test("create a link, follow it, retarget it, see history and analytics, deactivate, delete", async ({ request, playwright }) => {
  const { orgId, teamId } = await workspace(request);
  const created = await request.post("/api/links", { headers, data: { teamId, targetUrl: "https://example.com/v1", title: "Launch" } });
  expect(created.status(), await created.text()).toBe(201);
  const link = await created.json();
  expect(link.slug).toMatch(/^[A-Za-z2-9]{8}$/);

  const visitor = await playwright.request.newContext({ baseURL: ORIGIN, maxRedirects: 0 });
  const hop = await visitor.get(`/l/${link.slug}?utm_source=news&secret=1`, { headers: { referer: "https://ref.example/page?x=1" } });
  expect(hop.status()).toBe(302);
  expect(hop.headers()["location"]).toBe("https://example.com/v1");
  expect(hop.headers()["cache-control"]).toBe("no-store");

  await expect.poll(async () => (await (await request.get(`/api/links/${link.id}/analytics`)).json()).totalClicks, { timeout: 5000 }).toBe(1);
  const analytics = await (await request.get(`/api/links/${link.id}/analytics`)).json();
  expect(analytics.topReferrers[0]).toEqual({ referer: "https://ref.example/page", clicks: 1 });

  const retarget = await request.patch(`/api/links/${link.id}`, { headers, data: { targetUrl: "https://example.com/v2", redirectStatus: 307 } });
  expect(retarget.status()).toBe(200);
  const hop2 = await visitor.get(`/l/${link.slug}`);
  expect(hop2.status()).toBe(307);
  expect(hop2.headers()["location"]).toBe("https://example.com/v2");
  const history = await (await request.get(`/api/links/${link.id}/history`)).json();
  expect(history.total).toBe(2);
  expect(history.items[0]).toMatchObject({ oldTargetUrl: "https://example.com/v1", newTargetUrl: "https://example.com/v2" });

  const list = await (await request.get(`/api/links?organizationId=${orgId}`)).json();
  expect(list.total).toBe(1);
  expect(list.items[0].id).toBe(link.id);

  expect((await request.patch(`/api/links/${link.id}`, { headers, data: { isActive: false } })).status()).toBe(200);
  const off = await visitor.get(`/l/${link.slug}`);
  expect(off.status()).toBe(404);
  expect(off.headers()["cache-control"]).toBe("no-store");

  expect((await request.delete(`/api/links/${link.id}`)).status()).toBe(204);
  expect((await request.get(`/api/links/${link.id}`)).status()).toBe(404);
  await visitor.dispose();
});

test("custom slugs are normalised, unique across organizations and validated", async ({ request, playwright }) => {
  const { teamId } = await workspace(request);
  const slug = `launch-${unique()}`;
  const created = await request.post("/api/links", { headers, data: { teamId, targetUrl: "https://example.com", slug: `  ${slug.toUpperCase()} ` } });
  expect(created.status(), await created.text()).toBe(201);
  expect((await created.json()).slug).toBe(slug);
  const dup = await request.post("/api/links", { headers, data: { teamId, targetUrl: "https://example.com", slug } });
  expect(dup.status()).toBe(409);
  expect((await dup.json()).code).toBe("SLUG_TAKEN");

  const other = await playwright.request.newContext({ baseURL: ORIGIN });
  const theirs = await workspace(other);
  const cross = await other.post("/api/links", { headers, data: { teamId: theirs.teamId, targetUrl: "https://example.com", slug } });
  expect(cross.status()).toBe(409);
  await other.dispose();

  for (const bad of [{ targetUrl: "javascript:alert(1)" }, { targetUrl: "https://example.com", slug: "Hello World!" }, { targetUrl: "https://example.com", redirectStatus: 308 }]) {
    const res = await request.post("/api/links", { headers, data: { teamId, ...bad } });
    expect(res.status(), JSON.stringify(bad)).toBe(400);
  }
});

test("a member outside the team cannot see or edit its links", async ({ request, playwright }) => {
  const { orgId, teamId } = await workspace(request);
  const link = await (await request.post("/api/links", { headers, data: { teamId, targetUrl: "https://example.com" } })).json();
  await request.delete("/api/_test/mail");
  const email = `member-${unique()}@example.com`;
  const inv = await (await request.post(`/api/organizations/${orgId}/invitations`, { headers, data: { email, role: "member" } })).json();
  const member = await playwright.request.newContext({ baseURL: ORIGIN });
  expect((await member.post(`/api/invitations/${inv.id}/register`, { headers, data: { name: "Member", password: PASSWORD } })).status()).toBe(201);
  expect((await member.get(`/api/links/${link.id}`)).status()).toBe(403);
  expect((await member.patch(`/api/links/${link.id}`, { headers, data: { isActive: false } })).status()).toBe(403);
  expect((await (await member.get(`/api/links?organizationId=${orgId}`)).json()).total).toBe(0);
  await member.dispose();
});

test("openapi.json and the Scalar page are public", async ({ request }) => {
  const spec = await request.get("/openapi.json");
  expect(spec.status()).toBe(200);
  expect((await spec.json()).paths["/api/links"]).toBeTruthy();
  const page = await request.get("/scalar");
  expect(page.status()).toBe(200);
  expect(page.headers()["content-security-policy"]).toContain("https://cdn.jsdelivr.net");
});
```

- [ ] **Step 2: CI smoke additions**

In `.github/workflows/ci.yml`'s smoke step, after the sign-in probe and before the healthcheck line:

```bash
          # The redirect answers plain-text 404 with no-store for unknown slugs,
          # and the docs are public.
          test "$(curl -s -o /dev/null -w '%{http_code}' http://localhost:3000/l/does-not-exist)" = "404"
          curl -sI http://localhost:3000/l/does-not-exist | grep -qi 'cache-control: no-store'
          curl -fsS http://localhost:3000/openapi.json | grep -q '"/api/links"'
          curl -fsSI http://localhost:3000/scalar | grep -qi 'content-security-policy'
```

- [ ] **Step 3: Verify, docs line, commit**

```bash
bun run check && bun run test
(cd apps/server && mise exec -- go vet ./... && mise exec -- go test -p 1 -count=1 ./...)
E2E_REBUILD=1 mise run e2e 2>&1 | tail -12      # expect 15 passed (7 smoke + 4 auth-api + 4 links-api)
```

Add to `AGENTS.md`'s banner paragraph: `Phase 3 (links, the /l/{slug} redirect with click analytics, /openapi.json and /scalar) is implemented.`

```bash
git add e2e/links-api.spec.ts .github/workflows/ci.yml AGENTS.md
git commit -m "test(e2e): Playwright link flows against the image; CI smoke for the redirect and docs"
```

Do not push; the controller runs the whole-branch review and opens the PR (base `feat/go-migration-phase-2` until #80 merges).

---

## Self-review

**Spec coverage (phase 3 in section 11 and the cited sections):** links CRUD with generated/custom slugs, `409 SLUG_TAKEN`, target rules, slug immutability (T1, T5); history (T5); analytics with `days` (T6, section 5); redirect semantics, no-store, 404 text, rate limit, click privacy, async recorder with drain on shutdown (T2, T6, sections 3 and 5); Scalar + `/openapi.json` public with the Scalar CSP exception (T6, section 2); `POST /api/links` on the shared write limit (T4 tiers); tenancy rules on every link route (T4 resolver, T5, T6 tests); Playwright flows and CI smoke (T7).

**Deviations decided here:** (1) the spec's "same shape as today" for analytics is kept minus geo columns, with `topReferrers`/`topCountries` capped at 10 per section 5 (today's app used 5); (2) `GET /l/{slug}` is hand-routed and not in the OpenAPI document (a redirect is not a JSON operation); (3) paging is page/pageSize per spec section 2 rather than the old keyset cursor; (4) `http.Redirect`'s small HTML body is accepted.

**Placeholder scan:** none; generated-name notes point at concrete files.

**Type consistency:** `middleware.ResolveTeamAccess(ctx, Deps, userID, teamID) (TeamCtx, error)` with `ErrTeamNotFound`/`ErrTeamForbidden` is defined in T4 and used in T5/T6; `d.linkForCaller` defined in T5, used in T6; `redirect.Recorder{Record, Drain}` and `ClickEvent` fields defined in T2 match T6's `followLink`; `testrig.AppRig.Clicks` added in T6 and used by `redirect_test.go`; sqlc names in T3 match the calls in T5/T6 (`ListLinksParams{OrganizationID, TeamID, TeamIds, PageSize, PageOffset}`, `Analytics*Params{LinkID, ClickedAt, ClickedAt_2}`) subject to the generated-name check each task performs.
