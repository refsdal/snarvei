# Go Migration Phase 2: Auth and Tenancy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the Go server real users, sessions, organizations, teams and invitations: Limen-backed auth behind `internal/auth`, the `/api/me` family (profile, image, email change, sessions, deletion), organizations and members, teams and team membership, invitations with optional team, SMTP email, the storage port, the rate limiter and the client-IP helper, all wired into `cmd/snarvei`, with Go tests against real Postgres and an API-level Playwright suite against the image.

**Architecture:** Limen (`github.com/thecodearcher/limen` v0.2.1 with `credential-password`, `two-factor`, `organization`) is confined to `internal/auth` behind `auth.Service`; every other package sees only `auth.Session` and the service's methods. `internal/api/middleware` resolves the session and the organization or team named in the path and applies the tenancy rules from `internal/authz`. Every route except `/api/auth/*` stays spec-first in `openapi/snarvei.yaml`; multipart uploads and image streaming are hand-routed on the same mux. Queries are sqlc SQL in `internal/db/queries`.

**Tech Stack:** Go 1.27, Limen v0.2.1 (+ `adapters/sql` v0.2.0, `plugins/credential-password` v0.2.0, `plugins/two-factor` v0.2.0, `plugins/organization` v0.1.0), sqlc 1.31.1, pgx v5.10.0, aws-sdk-go-v2 (v1.45.1, credentials v1.20.2, service/s3 v1.110.0, smithy-go v1.28.1), `net/smtp`, `log/slog`, oapi-codegen v2.8.0, Playwright.

**Spec:** `docs/superpowers/specs/2026-09-04-go-backend-migration-design.md` (sections 2 (API endpoints: Me, Organizations, Teams), 3, 4, 7, 8, 9, 11 phase 2)

## Global Constraints

- Limen versions pinned exactly: `github.com/thecodearcher/limen v0.2.1`, `github.com/thecodearcher/limen/adapters/sql v0.2.0`, `github.com/thecodearcher/limen/plugins/credential-password v0.2.0`, `github.com/thecodearcher/limen/plugins/two-factor v0.2.0`, `github.com/thecodearcher/limen/plugins/organization v0.1.0`. AWS SDK pins as in the tech stack line. `internal/auth` is the ONLY package that imports any `thecodearcher/limen` path (a test enforces it).
- Limen HTTP routes are an ALLOWLIST. Allowed: `me`, `signout`, `signin`, `signup` (only when `OPEN_SIGNUP=1`), `passwords-change`, `passwords-request-reset`, `passwords-reset`, `two-factor-initiate-setup`, `two-factor-finalize-setup`, `two-factor-disable`, `two-factor-verify`, `get-backup-codes`, `totp-uri`. Everything else in `knownRouteIDs` is passed to `limen.WithHTTPDisabledPaths`.
- Session cookie name `snarvei_session`, HttpOnly, SameSite=Lax, Secure iff `APP_URL` is https. Sessions last 7 days and slide (Limen defaults). Trusted origin is exactly `APP_URL`.
- Org roles are Limen's `owner`, `admin`, `member`. Rules (`internal/authz`): owner/admin see and mutate every team and link; member only teams they belong to; only owner/admin create teams, manage team membership, invite and cancel invitations; no role management endpoint in this phase.
- Client IP: `TRUSTED_PROXY_HOPS` semantics from spec section 3 (0 = peer address; N = N-th from the right of `X-Forwarded-For`). Country from `CF-IPCountry` only when hops > 0 and the value is a two-letter code other than `XX`/`T1`. IP hash = HMAC-SHA256 keyed by `IP_HASH_PEPPER` or, when unset, `HMAC-SHA256(AUTH_SECRET, "snarvei:ip-hash")`. No raw IP is ever stored (Limen gets the keyed extractor for session metadata and rate-limit keys).
- Rate limits (Snarvei's `rate_limit` table, fixed window, per hashed IP): `POST /api/invitations/{id}/register`, `POST /api/me/email` and (phase 3) `POST /api/links` share `30` per `60 s`; `429` with `Retry-After`. Limen's own DB-backed limiter covers `/api/auth/*` at 60 per minute base.
- Error envelope `{"code","message","details"?}` for every API error. Codes used in this phase: `UNAUTHENTICATED` 401, `FORBIDDEN` 403, `NOT_FOUND` 404, `VALIDATION_FAILED` 400, `INVALID_PASSWORD` 401, `EMAIL_TAKEN` 409, `SLUG_TAKEN` 409, `ALREADY_MEMBER` 409, `INVITATION_EXISTS` 409, `INVITATION_EMAIL_MISMATCH` 403, `INVITATION_INVALID` 410, `LAST_OWNER` 409, `TEAM_EXISTS` 409, `CONFLICT` 409, `RATE_LIMITED` 429, `INTERNAL` 500.
- Profile images: multipart field `file`, ≤ 2 MiB, content type sniffed to `image/png`, `image/jpeg` or `image/webp`; stored at key `profile/<userId>/<uuid>.<ext>`; `users.image` holds the public path `/images/profile/<userId>/<uuid>.<ext>`; served with `Cache-Control: public, max-age=31536000, immutable`.
- Email change tokens: 32 random bytes, hex, stored as SHA-256 hex in `email_change_requests`, valid 1 hour, single use; link `<APP_URL>/app/settings?emailToken=<token>`. Password reset link `<APP_URL>/reset-password?token=<token>`; invitation link `<APP_URL>/app/invitations/<id>`.
- Account deletion requires the password, refuses with `409 LAST_OWNER` when the user is the sole owner of any organization, revokes all sessions, then deletes the user row (cascades sessions, accounts, memberships, team memberships; links keep rows with null authorship).
- Logging moves to `log/slog` JSON on stdout; event names `email.not_configured`, `email.send_failed`, `request.error`.
- Generated code (sqlc `internal/db/gen`, oapi-codegen `internal/api/gen`, `internal/api/snarvei.yaml`) is committed; `go generate ./...` from `apps/server` regenerates; drift guards exist for oapi-codegen and the spec copy; this phase adds one for sqlc.
- Tests: `go test -p 1 -count=1 ./...` against `TEST_DATABASE_URL`; TDD per task; Conventional Commits with the two trailers `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>` and `Claude-Session: https://claude.ai/code/session_01UdGgRFBUoiwkd9PLH7zUJE`.
- Pjokk (`~/projects/refsdal/pjokk/apps/server`) is the reference for Limen usage; where this plan says "copy from Pjokk", copy then apply the listed edits.
- Run commands from the repository root unless a step says otherwise; Go through `mise exec --` (or `eval "$(mise activate bash)"`); the test Postgres via `docker compose -f docker-compose.test.yml up -d --wait`.

---

## File Structure

Created in this phase:

```
apps/server/internal/clientip/{clientip.go,clientip_test.go}        X-Forwarded-For, CF-IPCountry, keyed IP hash
apps/server/internal/ratelimit/{ratelimit.go,ratelimit_test.go}     Postgres fixed-window counter
apps/server/internal/storage/{storage.go,fs.go,s3.go,memory.go,storage_test.go}   copied from Pjokk
apps/server/internal/email/{email.go,smtp.go,templates.go,email_test.go}          Sender, SMTP, no-op, recording, templates
apps/server/internal/authz/{authz.go,authz_test.go}                 pure role/team rules
apps/server/internal/db/queries/{ratelimit.sql,auth.sql,me.sql,orgs.sql,teams.sql,invitations.sql}
apps/server/internal/db/gen/*.go                                    sqlc output (committed)
apps/server/internal/db/gen_test.go                                 sqlc drift guard
apps/server/internal/auth/{auth.go,core_plugin.go,session.go,routes.go,errors.go,auth_test.go,boundary_test.go}
apps/server/internal/api/middleware/{middleware.go,middleware_test.go}
apps/server/internal/api/{tiers.go,errors.go,me.go,images.go,organizations.go,invitations.go,teams.go,testhooks.go}
apps/server/internal/api/{me_test.go,organizations_test.go,invitations_test.go,teams_test.go}
apps/server/internal/testrig/http.go                                AppRig (handler + sign-up/sign-in helpers)
apps/server/internal/db/migrations/00002_*.sql                      only if Limen's runtime schema check demands it
e2e/auth-api.spec.ts                                                Playwright request-level flows against the image
```

Modified: `openapi/snarvei.yaml` (+ generated copies), `apps/server/internal/api/api.go` (Deps, chains, hand routes), `apps/server/cmd/snarvei/main.go` (composition, slog), `apps/server/go.mod`/`go.sum`, `.github/workflows/ci.yml` (sign-in smoke probe), `AGENTS.md` banner line.

Responsibilities: `clientip` never stores anything; `ratelimit` only counts; `storage` only bytes; `email` only sends; `authz` only decides; `auth` is the only Limen importer and owns users, sessions, organizations and invitations; `middleware` puts identity and tenancy on the context and rejects; `api` handlers translate HTTP to `auth.Service` and sqlc calls; `cmd/snarvei` constructs everything.

---

### Task 1: Client-IP helper, rate limiter, storage port

**Files:**
- Create: `apps/server/internal/clientip/clientip.go`, `apps/server/internal/clientip/clientip_test.go`, `apps/server/internal/db/queries/ratelimit.sql`, `apps/server/internal/ratelimit/ratelimit.go`, `apps/server/internal/ratelimit/ratelimit_test.go`, `apps/server/internal/db/gen_test.go`, `apps/server/internal/storage/*` (copied)
- Modify: `apps/server/generate.go` (add the sqlc directive), `apps/server/go.mod`

**Interfaces:**
- Produces:
  ```go
  package clientip
  func FromRequest(r *http.Request, trustedHops int) string   // never empty: "unknown" fallback
  func Country(r *http.Request, trustedHops int) string       // "" when untrusted/absent/XX/T1
  type Hasher struct{ key []byte }
  func NewHasher(pepper, authSecret string) *Hasher
  func (h *Hasher) Hash(ip string) string                     // hex HMAC-SHA256
  func (h *Hasher) Extractor(trustedHops int) func(*http.Request) string   // Hash(FromRequest(r, hops))

  package ratelimit
  type Store interface { Hit(ctx context.Context, key string, window time.Duration) (count int, retryAfter time.Duration, err error) }
  func NewPostgres(q *gen.Queries) Store
  func Key(name, bucket string, now time.Time, window time.Duration) (key string, windowStart time.Time)

  package storage  // as Pjokk: Storage{Put,GetStream,Delete,List}, NewFS(root), NewS3(S3Config), NewMemory()
  ```

- [ ] **Step 1: Write the failing clientip tests**

`apps/server/internal/clientip/clientip_test.go`:

```go
package clientip

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func req(remote, xff, country string) *httptest.ResponseRecorder {
	return nil
}

func TestFromRequest(t *testing.T) {
	cases := []struct {
		name          string
		remote, xff   string
		hops          int
		want          string
	}{
		{"zero hops ignores the header", "10.0.0.1:5555", "1.2.3.4", 0, "10.0.0.1"},
		{"one hop picks the rightmost entry", "10.0.0.1:5555", "9.9.9.9, 1.2.3.4", 1, "1.2.3.4"},
		{"two hops counts back from the right", "10.0.0.1:5555", "9.9.9.9, 1.2.3.4, 172.16.0.1", 2, "1.2.3.4"},
		{"more hops than entries floors at the leftmost", "10.0.0.1:5555", "1.2.3.4, 172.16.0.1", 5, "1.2.3.4"},
		{"hops but no header falls back to the peer", "10.0.0.1:5555", "", 1, "10.0.0.1"},
		{"ipv6 peer keeps its host", "[::1]:5555", "", 0, "::1"},
		{"empty everything is unknown", "", "", 0, "unknown"},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = c.remote
		if c.xff != "" {
			r.Header.Set("X-Forwarded-For", c.xff)
		}
		if got := FromRequest(r, c.hops); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestCountry(t *testing.T) {
	cases := []struct {
		header string
		hops   int
		want   string
	}{
		{"NO", 1, "NO"},
		{"no", 1, "NO"},
		{"NO", 0, ""},
		{"XX", 1, ""},
		{"T1", 1, ""},
		{"", 1, ""},
		{"NOR", 1, ""},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		if c.header != "" {
			r.Header.Set("CF-IPCountry", c.header)
		}
		if got := Country(r, c.hops); got != c.want {
			t.Errorf("header %q hops %d: got %q want %q", c.header, c.hops, got, c.want)
		}
	}
}

func TestHasher(t *testing.T) {
	a := NewHasher("", strings.Repeat("s", 32))
	b := NewHasher("", strings.Repeat("s", 32))
	c := NewHasher("pepper", strings.Repeat("s", 32))
	if a.Hash("1.2.3.4") != b.Hash("1.2.3.4") {
		t.Fatal("same secret must give the same hash")
	}
	if a.Hash("1.2.3.4") == c.Hash("1.2.3.4") {
		t.Fatal("a pepper must change the hash")
	}
	if a.Hash("1.2.3.4") == a.Hash("1.2.3.5") {
		t.Fatal("different addresses must differ")
	}
	if h := a.Hash("1.2.3.4"); len(h) != 64 || strings.ContainsAny(h, "1234.") && strings.Contains(h, "1.2.3.4") {
		t.Fatalf("hash must be 64 hex chars and not contain the address: %q", h)
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.1:5555"
	r.Header.Set("X-Forwarded-For", "1.2.3.4")
	if a.Extractor(1)(r) != a.Hash("1.2.3.4") {
		t.Fatal("extractor must hash the trusted client address")
	}
	if a.Extractor(0)(r) != a.Hash("10.0.0.1") {
		t.Fatal("extractor with zero hops must hash the peer")
	}
}
```

(Delete the unused `req` stub before running; it is not part of the file.)

- [ ] **Step 2: Run to verify failure**

Run: `cd apps/server && mise exec -- go test ./internal/clientip/`
Expected: compile error, `undefined: FromRequest`.

- [ ] **Step 3: Write `apps/server/internal/clientip/clientip.go`**

```go
// Package clientip derives the caller's address from a request behind N
// trusted proxies, the country Cloudflare reports, and the keyed digest that
// is the only form of the address Snarvei ever stores.
package clientip

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
)

// FromRequest returns the client address. With trustedHops == 0 the peer
// address is the client; with N > 0 the N-th entry from the right of
// X-Forwarded-For is (Cloudflare proxied DNS = 1). Never empty.
func FromRequest(r *http.Request, trustedHops int) string {
	peer := hostOnly(r.RemoteAddr)
	if trustedHops <= 0 {
		return orUnknown(peer)
	}
	var chain []string
	for _, part := range strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",") {
		if p := strings.TrimSpace(part); p != "" {
			chain = append(chain, p)
		}
	}
	if len(chain) == 0 {
		return orUnknown(peer)
	}
	index := len(chain) - trustedHops
	if index < 0 {
		index = 0
	}
	return orUnknown(chain[index])
}

// Country returns Cloudflare's two-letter country code when a proxy is
// trusted, upper-cased; "" for untrusted requests, absent headers and the
// XX/T1 placeholders.
func Country(r *http.Request, trustedHops int) string {
	if trustedHops <= 0 {
		return ""
	}
	code := strings.ToUpper(strings.TrimSpace(r.Header.Get("CF-IPCountry")))
	if len(code) != 2 || code == "XX" || code == "T1" {
		return ""
	}
	return code
}

// Hasher produces keyed digests of addresses.
type Hasher struct{ key []byte }

// NewHasher uses pepper when set, otherwise a key derived from authSecret
// with its own domain separator so it can never equal the signing secret.
func NewHasher(pepper, authSecret string) *Hasher {
	if pepper != "" {
		return &Hasher{key: []byte(pepper)}
	}
	mac := hmac.New(sha256.New, []byte(authSecret))
	mac.Write([]byte("snarvei:ip-hash"))
	return &Hasher{key: mac.Sum(nil)}
}

// Hash returns the hex HMAC-SHA256 of ip.
func (h *Hasher) Hash(ip string) string {
	mac := hmac.New(sha256.New, h.key)
	mac.Write([]byte(ip))
	return hex.EncodeToString(mac.Sum(nil))
}

// Extractor adapts the hasher to the func(*http.Request) string shape Limen
// wants for session metadata and rate-limit keys.
func (h *Hasher) Extractor(trustedHops int) func(*http.Request) string {
	return func(r *http.Request) string { return h.Hash(FromRequest(r, trustedHops)) }
}

func hostOnly(address string) string {
	if host, _, err := net.SplitHostPort(address); err == nil {
		return host
	}
	return address
}

func orUnknown(address string) string {
	if address == "" {
		return "unknown"
	}
	return address
}
```

Run: `cd apps/server && mise exec -- go test ./internal/clientip/ -v`
Expected: 3 PASS.

- [ ] **Step 4: Add the sqlc query file and the generate directive**

`apps/server/internal/db/queries/ratelimit.sql`:

```sql
-- Snarvei's own fixed-window counters (table rate_limit; Limen's rate_limits
-- is separate). The key carries the window index, so a bucket never outlives
-- its window; SweepRateLimit removes old windows opportunistically.

-- name: HitRateLimit :one
INSERT INTO "rate_limit" ("key", "window_start", "count")
VALUES ($1, $2, 1)
ON CONFLICT ("key") DO UPDATE SET "count" = "rate_limit"."count" + 1
RETURNING "count";

-- name: SweepRateLimit :execrows
DELETE FROM "rate_limit" WHERE "window_start" < $1;
```

Append to `apps/server/generate.go` (after the oapi-codegen lines):

```go
//go:generate sqlc generate
```

Run: `cd apps/server && rm -f internal/db/queries/.keep && mise exec -- go generate ./... && ls internal/db/gen`
Expected: `db.go models.go querier.go ratelimit.sql.go` (names may vary: `sqlc` emits `db.go`, `models.go`, one file per query file). `models.go` must contain `type RateLimit struct` and `type RateLimits struct` (both tables, thanks to `emit_exact_table_names`).

- [ ] **Step 5: Write the sqlc drift guard `apps/server/internal/db/gen_test.go`**

```go
package db_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestSqlcOutputIsUpToDate regenerates into a temp dir and diffs against the
// committed internal/db/gen. Skips locally without sqlc on PATH; fails in CI.
func TestSqlcOutputIsUpToDate(t *testing.T) {
	if _, err := exec.LookPath("sqlc"); err != nil {
		if os.Getenv("CI") != "" {
			t.Fatal("sqlc missing in CI")
		}
		t.Skip("sqlc not on PATH (run `mise install`)")
	}
	tmp := t.TempDir()
	cfg, err := os.ReadFile("../../sqlc.yaml")
	if err != nil {
		t.Fatal(err)
	}
	rewritten := []byte(string(cfg))
	rewritten = []byte(replaceOnce(string(rewritten), `out: "internal/db/gen"`, `out: "`+tmp+`"`))
	tmpCfg := filepath.Join(t.TempDir(), "sqlc.yaml")
	if err := os.WriteFile(tmpCfg, rewritten, 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sqlc", "generate", "-f", tmpCfg)
	cmd.Dir = "../.."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sqlc generate: %v\n%s", err, out)
	}
	entries, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		want, _ := os.ReadFile(filepath.Join(tmp, e.Name()))
		got, err := os.ReadFile(filepath.Join("gen", e.Name()))
		if err != nil || string(want) != string(got) {
			t.Fatalf("internal/db/gen/%s is stale: run `go generate ./...` from apps/server and commit", e.Name())
		}
	}
}

func replaceOnce(s, old, new string) string {
	i := len(s)
	for j := 0; j+len(old) <= len(s); j++ {
		if s[j:j+len(old)] == old {
			i = j
			break
		}
	}
	if i == len(s) {
		return s
	}
	return s[:i] + new + s[i+len(old):]
}
```

Run: `cd apps/server && mise exec -- go test ./internal/db/ -run TestSqlcOutputIsUpToDate -v`
Expected: PASS.

- [ ] **Step 6: Write the failing rate-limit test**

`apps/server/internal/ratelimit/ratelimit_test.go`:

```go
package ratelimit_test

import (
	"context"
	"testing"
	"time"

	"github.com/refsdal/snarvei/server/internal/db/gen"
	"github.com/refsdal/snarvei/server/internal/ratelimit"
	"github.com/refsdal/snarvei/server/internal/testrig"
)

func TestKeyIsStableWithinAWindow(t *testing.T) {
	base := time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC)
	k1, ws1 := ratelimit.Key("redirect", "abc", base, time.Minute)
	k2, _ := ratelimit.Key("redirect", "abc", base.Add(30*time.Second), time.Minute)
	k3, ws3 := ratelimit.Key("redirect", "abc", base.Add(61*time.Second), time.Minute)
	if k1 != k2 || k1 == k3 {
		t.Fatalf("keys: %s %s %s", k1, k2, k3)
	}
	if !ws1.Equal(base) || !ws3.Equal(base.Add(time.Minute)) {
		t.Fatalf("window starts: %v %v", ws1, ws3)
	}
}

func TestPostgresHitCountsAndSweeps(t *testing.T) {
	rig := testrig.Setup(t)
	store := ratelimit.NewPostgres(gen.New(rig.Pool))
	ctx := context.Background()

	var last int
	for i := 1; i <= 3; i++ {
		count, retry, err := store.Hit(ctx, "rl:test:bucket:1", time.Minute)
		if err != nil {
			t.Fatalf("hit %d: %v", i, err)
		}
		if count != i {
			t.Fatalf("hit %d counted %d", i, count)
		}
		if retry <= 0 || retry > time.Minute {
			t.Fatalf("retryAfter = %v", retry)
		}
		last = count
	}
	if last != 3 {
		t.Fatal("expected 3 hits")
	}

	// An old bucket is swept when a new window opens.
	if _, err := rig.Pool.Exec(ctx, `INSERT INTO rate_limit (key, window_start, count) VALUES ('rl:old', now() - interval '1 hour', 9)`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Hit(ctx, "rl:test:other:1", time.Minute); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := rig.Pool.QueryRow(ctx, `SELECT count(*) FROM rate_limit WHERE key = 'rl:old'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("old bucket was not swept")
	}
}
```

Run: `cd apps/server && mise exec -- go test -p 1 ./internal/ratelimit/`
Expected: compile error.

- [ ] **Step 7: Write `apps/server/internal/ratelimit/ratelimit.go`**

```go
// Package ratelimit is Snarvei's fixed-window counter over the rate_limit
// table, shared by every replica. Keys are built by Key so the window index
// is part of the key and buckets never outlive their window.
package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/refsdal/snarvei/server/internal/db/gen"
)

// Store counts hits per key.
type Store interface {
	// Hit increments the bucket for key in the window containing now and
	// returns the count after the increment plus how long until the window
	// ends (what a 429 puts in Retry-After).
	Hit(ctx context.Context, key string, window time.Duration) (count int, retryAfter time.Duration, err error)
}

// Key builds "rl:<name>:<bucket>:<windowIndex>" and returns the window start.
func Key(name, bucket string, now time.Time, window time.Duration) (string, time.Time) {
	index := now.Unix() / int64(window/time.Second)
	start := time.Unix(index*int64(window/time.Second), 0).UTC()
	return fmt.Sprintf("rl:%s:%s:%d", name, bucket, index), start
}

// Postgres is the production Store.
type Postgres struct {
	q   *gen.Queries
	now func() time.Time
}

// NewPostgres builds a Store over q.
func NewPostgres(q *gen.Queries) Store { return &Postgres{q: q, now: time.Now} }

func (p *Postgres) Hit(ctx context.Context, key string, window time.Duration) (int, time.Duration, error) {
	now := p.now()
	_, start := Key("", "", now, window)
	count, err := p.q.HitRateLimit(ctx, gen.HitRateLimitParams{
		Key:         key,
		WindowStart: pgtype.Timestamptz{Time: start, Valid: true},
	})
	if err != nil {
		return 0, 0, fmt.Errorf("ratelimit: hit %q: %w", key, err)
	}
	// Opportunistic housekeeping on the first hit of a bucket: drop buckets
	// older than two windows. Failure is not the caller's problem.
	if count == 1 {
		cutoff := start.Add(-2 * window)
		_, _ = p.q.SweepRateLimit(ctx, pgtype.Timestamptz{Time: cutoff, Valid: true})
	}
	retry := start.Add(window).Sub(now)
	if retry <= 0 {
		retry = time.Second
	}
	return int(count), retry, nil
}
```

Run: `cd apps/server && mise exec -- go test -p 1 -count=1 ./internal/ratelimit/ -v`
Expected: 2 PASS. If sqlc typed `WindowStart` differently (e.g. `time.Time`), match the generated params struct.

- [ ] **Step 8: Copy the storage port from Pjokk**

```bash
cp ~/projects/refsdal/pjokk/apps/server/internal/storage/{storage.go,fs.go,s3.go,memory.go,storage_test.go} apps/server/internal/storage/
sed -i 's#github.com/refsdal/pjokk/server#github.com/refsdal/snarvei/server#g; s/\.pjokk-tmp-/.snarvei-tmp-/g' apps/server/internal/storage/*.go
cd apps/server && mise exec -- go get github.com/aws/aws-sdk-go-v2@v1.45.1 github.com/aws/aws-sdk-go-v2/credentials@v1.20.2 github.com/aws/aws-sdk-go-v2/service/s3@v1.110.0 github.com/aws/smithy-go@v1.28.1
```

Then edit `storage.go`'s package comment to drop the Pjokk task references (keep the interface unchanged), and delete any test in `storage_test.go` that needs a live S3/MinIO (Pjokk's conformance test runs the memory and fs drivers; keep those). Read `storage_test.go` once: if it references a `TEST_S3_*` env gate, keep the gate (the test skips without it).

Run: `cd apps/server && mise exec -- go vet ./... && mise exec -- go test ./internal/storage/ -v 2>&1 | tail -12`
Expected: conformance tests PASS for memory and fs; S3 skipped.

- [ ] **Step 9: Commit**

```bash
cd apps/server && mise exec -- go mod tidy && cd ../..
git add apps/server
git commit -m "feat(server): client-ip helper, Postgres rate limiter, storage port, sqlc wiring"
```

---

### Task 2: Email package (SMTP, no-op, recording, templates)

**Files:**
- Create: `apps/server/internal/email/email.go`, `apps/server/internal/email/smtp.go`, `apps/server/internal/email/templates.go`, `apps/server/internal/email/email_test.go`

**Interfaces:**
- Produces:
  ```go
  package email
  type Message struct { To, Subject, Text, HTML string }
  type Sender interface { Send(ctx context.Context, m Message) error }
  type SMTPConfig struct { Host string; Port int; Username, Password, From string }
  func NewSMTP(cfg SMTPConfig) Sender
  func NewNoop(log *slog.Logger) Sender            // logs event=email.not_configured with to+subject only
  type Recording struct{ ... }; func NewRecording() *Recording; (*Recording).Send; (*Recording).Messages() []Message; (*Recording).Last(to string) (Message, bool); (*Recording).Reset()
  type Template struct { Subject, Text, HTML string }
  func Invitation(appName, orgName, inviterName, link string) Template
  func PasswordReset(appName, link string) Template
  func EmailChange(appName, newEmail, link string) Template
  func (t Template) To(addr string) Message
  ```

- [ ] **Step 1: Write the failing tests `email_test.go`**

```go
package email_test

import (
	"bufio"
	"context"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"

	"github.com/refsdal/snarvei/server/internal/email"
)

// fakeSMTP speaks just enough SMTP to accept one plaintext message.
type fakeSMTP struct {
	addr string
	mu   sync.Mutex
	data string
	from string
	rcpt string
}

func startFakeSMTP(t *testing.T) *fakeSMTP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeSMTP{addr: ln.Addr().String()}
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)
		w := func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }
		w("220 fake ESMTP")
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimRight(line, "\r\n")
			switch {
			case strings.HasPrefix(line, "EHLO"):
				w("250-fake")
				w("250 AUTH PLAIN")
			case strings.HasPrefix(line, "AUTH PLAIN"):
				w("235 ok")
			case strings.HasPrefix(line, "MAIL FROM:"):
				f.mu.Lock()
				f.from = line
				f.mu.Unlock()
				w("250 ok")
			case strings.HasPrefix(line, "RCPT TO:"):
				f.mu.Lock()
				f.rcpt = line
				f.mu.Unlock()
				w("250 ok")
			case line == "DATA":
				w("354 go")
				var b strings.Builder
				for {
					l, err := r.ReadString('\n')
					if err != nil {
						return
					}
					if l == ".\r\n" {
						break
					}
					b.WriteString(l)
				}
				f.mu.Lock()
				f.data = b.String()
				f.mu.Unlock()
				w("250 queued")
			case line == "QUIT":
				w("221 bye")
				return
			default:
				w("250 ok")
			}
		}
	}()
	t.Cleanup(func() { ln.Close() })
	return f
}

func TestSMTPSendsAMultipartMessage(t *testing.T) {
	srv := startFakeSMTP(t)
	host, port, _ := net.SplitHostPort(srv.addr)
	var p int
	for _, c := range port {
		p = p*10 + int(c-'0')
	}
	sender := email.NewSMTP(email.SMTPConfig{Host: host, Port: p, Username: "u", Password: "p", From: "Snarvei <no-reply@example.com>"})
	msg := email.PasswordReset("Snarvei", "http://localhost:3000/reset-password?token=abc").To("someone@example.com")
	if err := sender.Send(context.Background(), msg); err != nil {
		t.Fatalf("Send: %v", err)
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if !strings.Contains(srv.from, "no-reply@example.com") || !strings.Contains(srv.rcpt, "someone@example.com") {
		t.Fatalf("envelope: %q %q", srv.from, srv.rcpt)
	}
	for _, want := range []string{"Subject: Reset your Snarvei password", "Content-Type: multipart/alternative", "text/plain", "text/html", "reset-password?token=abc"} {
		if !strings.Contains(srv.data, want) {
			t.Errorf("message lacks %q:\n%s", want, srv.data)
		}
	}
}

func TestNoopLogsWithoutTheBody(t *testing.T) {
	var buf strings.Builder
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	sender := email.NewNoop(logger)
	msg := email.EmailChange("Snarvei", "new@example.com", "http://localhost:3000/app/settings?emailToken=secret").To("new@example.com")
	if err := sender.Send(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "email.not_configured") || !strings.Contains(out, "new@example.com") {
		t.Fatalf("log line: %s", out)
	}
	if strings.Contains(out, "secret") {
		t.Fatalf("log line leaks the token: %s", out)
	}
}

func TestRecordingKeepsMessages(t *testing.T) {
	rec := email.NewRecording()
	_ = rec.Send(context.Background(), email.Invitation("Snarvei", "Acme", "Ada", "http://x/app/invitations/1").To("a@example.com"))
	_ = rec.Send(context.Background(), email.Invitation("Snarvei", "Acme", "", "http://x/app/invitations/2").To("b@example.com"))
	if len(rec.Messages()) != 2 {
		t.Fatal("expected two messages")
	}
	last, ok := rec.Last("b@example.com")
	if !ok || !strings.Contains(last.Text, "/app/invitations/2") || strings.Contains(last.Text, " by ") {
		t.Fatalf("last for b: %+v", last)
	}
	first, _ := rec.Last("a@example.com")
	if !strings.Contains(first.Text, "invited by Ada") {
		t.Fatalf("inviter missing: %q", first.Text)
	}
	rec.Reset()
	if len(rec.Messages()) != 0 {
		t.Fatal("reset must clear")
	}
}

func TestTemplatesEscapeHTML(t *testing.T) {
	tpl := email.Invitation("Snarvei", "<b>Acme</b>", "", "http://x/app/invitations/1")
	if strings.Contains(tpl.HTML, "<b>Acme</b>") || !strings.Contains(tpl.HTML, "&lt;b&gt;Acme&lt;/b&gt;") {
		t.Fatalf("org name not escaped: %s", tpl.HTML)
	}
	if !strings.Contains(tpl.Subject, "<b>Acme</b>") {
		t.Fatal("subject is plain text and must not be escaped")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd apps/server && mise exec -- go test ./internal/email/`
Expected: compile error, `undefined: email.NewSMTP`.

- [ ] **Step 3: Write `email.go`**

```go
// Package email is the transactional mail port: one Sender interface, an
// SMTP driver, a no-op driver that logs a redacted line when mail is not
// configured, a recording driver for tests and e2e hooks, and the templates.
package email

import (
	"context"
	"log/slog"
	"sync"
)

// Message is one outgoing mail.
type Message struct {
	To      string
	Subject string
	Text    string
	HTML    string
}

// Sender delivers messages.
type Sender interface {
	Send(ctx context.Context, m Message) error
}

type noop struct{ log *slog.Logger }

// NewNoop returns a Sender that drops every message and logs
// event=email.not_configured with only the recipient and subject: bodies
// carry bearer links and must never be logged.
func NewNoop(log *slog.Logger) Sender {
	if log == nil {
		log = slog.Default()
	}
	return &noop{log: log}
}

func (n *noop) Send(_ context.Context, m Message) error {
	n.log.Warn("email dropped: no SMTP configured", "event", "email.not_configured", "to", m.To, "subject", m.Subject)
	return nil
}

// Recording keeps every message in memory (tests, E2E_TEST_HOOKS).
type Recording struct {
	mu       sync.Mutex
	messages []Message
}

// NewRecording returns an empty Recording.
func NewRecording() *Recording { return &Recording{} }

func (r *Recording) Send(_ context.Context, m Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages = append(r.messages, m)
	return nil
}

// Messages returns a copy, oldest first.
func (r *Recording) Messages() []Message {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Message, len(r.messages))
	copy(out, r.messages)
	return out
}

// Last returns the most recent message sent to addr.
func (r *Recording) Last(addr string) (Message, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := len(r.messages) - 1; i >= 0; i-- {
		if r.messages[i].To == addr {
			return r.messages[i], true
		}
	}
	return Message{}, false
}

// Reset forgets everything.
func (r *Recording) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages = nil
}
```

- [ ] **Step 4: Write `smtp.go`**

```go
package email

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"mime"
	"mime/multipart"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

// SMTPConfig is the all-or-nothing SMTP group from the environment.
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string // "Name <addr>" or "addr"
}

type smtpSender struct{ cfg SMTPConfig }

// NewSMTP returns a Sender that speaks SMTP: implicit TLS on 465, STARTTLS
// when the server offers it otherwise (required unless the host is loopback),
// PLAIN auth when a username is set.
func NewSMTP(cfg SMTPConfig) Sender { return &smtpSender{cfg: cfg} }

func (s *smtpSender) Send(ctx context.Context, m Message) error {
	from, err := mail.ParseAddress(s.cfg.From)
	if err != nil {
		return fmt.Errorf("email: EMAIL_FROM %q: %w", s.cfg.From, err)
	}
	to, err := mail.ParseAddress(m.To)
	if err != nil {
		return fmt.Errorf("email: recipient %q: %w", m.To, err)
	}
	body, err := build(from, to, m)
	if err != nil {
		return err
	}

	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	var conn net.Conn
	if s.cfg.Port == 465 {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: s.cfg.Host})
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("email: connect %s: %w", addr, err)
	}
	client, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("email: smtp handshake: %w", err)
	}
	defer client.Close()

	if s.cfg.Port != 465 {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: s.cfg.Host}); err != nil {
				return fmt.Errorf("email: starttls: %w", err)
			}
		} else if !isLoopback(s.cfg.Host) {
			return fmt.Errorf("email: %s offers no STARTTLS; refusing to send credentials in clear", s.cfg.Host)
		}
	}
	if s.cfg.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)); err != nil {
			return fmt.Errorf("email: auth: %w", err)
		}
	}
	if err := client.Mail(from.Address); err != nil {
		return fmt.Errorf("email: MAIL FROM: %w", err)
	}
	if err := client.Rcpt(to.Address); err != nil {
		return fmt.Errorf("email: RCPT TO: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("email: DATA: %w", err)
	}
	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("email: write body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("email: finish body: %w", err)
	}
	return client.Quit()
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// build renders RFC 5322 headers plus a multipart/alternative body.
func build(from, to *mail.Address, m Message) ([]byte, error) {
	var buf bytes.Buffer
	mp := multipart.NewWriter(&buf)
	fmt.Fprintf(&buf, "From: %s\r\n", from.String())
	fmt.Fprintf(&buf, "To: %s\r\n", to.String())
	fmt.Fprintf(&buf, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", m.Subject))
	fmt.Fprintf(&buf, "Date: %s\r\n", time.Now().UTC().Format(time.RFC1123Z))
	fmt.Fprintf(&buf, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&buf, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", mp.Boundary())
	part := func(ctype, content string) error {
		h := textproto.MIMEHeader{}
		h.Set("Content-Type", ctype+"; charset=utf-8")
		h.Set("Content-Transfer-Encoding", "8bit")
		w, err := mp.CreatePart(h)
		if err != nil {
			return err
		}
		_, err = w.Write([]byte(strings.ReplaceAll(content, "\n", "\r\n")))
		return err
	}
	if err := part("text/plain", m.Text); err != nil {
		return nil, fmt.Errorf("email: text part: %w", err)
	}
	if m.HTML != "" {
		if err := part("text/html", m.HTML); err != nil {
			return nil, fmt.Errorf("email: html part: %w", err)
		}
	}
	if err := mp.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
```

- [ ] **Step 5: Write `templates.go`**

```go
package email

import (
	"fmt"
	"html"
)

// Template is a rendered mail without a recipient.
type Template struct {
	Subject string
	Text    string
	HTML    string
}

// To binds the template to a recipient.
func (t Template) To(addr string) Message {
	return Message{To: addr, Subject: t.Subject, Text: t.Text, HTML: t.HTML}
}

func layout(appName, title, bodyHTML string) string {
	return fmt.Sprintf(`<!doctype html>
<html><body style="font-family:system-ui,-apple-system,Segoe UI,Roboto,sans-serif;line-height:1.5;color:#111">
<h2 style="margin:0 0 16px">%s</h2>
%s
<p style="color:#666;font-size:12px;margin-top:32px">Sent by %s. If you did not expect this email you can ignore it.</p>
</body></html>`, html.EscapeString(title), bodyHTML, html.EscapeString(appName))
}

func linkButton(href, label string) string {
	h := html.EscapeString(href)
	return fmt.Sprintf(`<p><a href="%s" style="display:inline-block;padding:10px 16px;background:#4f46e5;color:#fff;text-decoration:none;border-radius:6px">%s</a></p><p style="font-size:12px;color:#666">Or open this link: %s</p>`, h, html.EscapeString(label), h)
}

// Invitation is the organization invitation mail; inviterName may be empty.
func Invitation(appName, orgName, inviterName, link string) Template {
	by := ""
	if inviterName != "" {
		by = " by " + inviterName
	}
	return Template{
		Subject: fmt.Sprintf("You have been invited to %s on %s", orgName, appName),
		Text:    fmt.Sprintf("You have been invited%s to join %s on %s.\n\nAccept the invitation: %s\n\nIf you did not expect this invitation you can ignore this email.", by, orgName, appName, link),
		HTML: layout(appName, "Join "+orgName,
			fmt.Sprintf("<p>You have been invited%s to join <strong>%s</strong> on %s.</p>%s", html.EscapeString(by), html.EscapeString(orgName), html.EscapeString(appName), linkButton(link, "Accept invitation"))),
	}
}

// PasswordReset is the forgot-password mail.
func PasswordReset(appName, link string) Template {
	return Template{
		Subject: fmt.Sprintf("Reset your %s password", appName),
		Text:    fmt.Sprintf("Reset your %s password: %s\n\nIf you did not request this, ignore this email.", appName, link),
		HTML:    layout(appName, "Reset your password", "<p>Use the button below to choose a new password.</p>"+linkButton(link, "Reset password")),
	}
}

// EmailChange confirms a new address.
func EmailChange(appName, newEmail, link string) Template {
	return Template{
		Subject: fmt.Sprintf("Confirm your new %s email address", appName),
		Text:    fmt.Sprintf("Confirm changing your %s email address to %s: %s", appName, newEmail, link),
		HTML: layout(appName, "Confirm your new email address",
			fmt.Sprintf("<p>Confirm changing your email address to <strong>%s</strong>.</p>%s", html.EscapeString(newEmail), linkButton(link, "Confirm change"))),
	}
}
```

- [ ] **Step 6: Run the tests**

Run: `cd apps/server && mise exec -- go vet ./internal/email/ && mise exec -- go test ./internal/email/ -v`
Expected: 4 PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/server/internal/email
git commit -m "feat(server): email port with SMTP, no-op and recording senders and templates"
```

---

### Task 3: Pure authorization rules (`internal/authz`)

**Files:**
- Create: `apps/server/internal/authz/authz.go`, `apps/server/internal/authz/authz_test.go`

**Interfaces:**
- Produces:
  ```go
  package authz
  const RoleOwner, RoleAdmin, RoleMember = "owner", "admin", "member"
  func IsValidInviteRole(role string) bool            // admin | member
  func IsOrgAdmin(role string) bool                   // owner or admin
  func Highest(roles []string) string                 // owner > admin > member > ""
  func CanAccessTeam(orgRole string, isTeamMember bool) bool
  func CanManageTeams(orgRole string) bool            // owner/admin
  func CanInvite(orgRole string) bool                 // owner/admin
  ```

- [ ] **Step 1: Write the failing tests**

```go
package authz

import "testing"

func TestRoles(t *testing.T) {
	if !IsOrgAdmin(RoleOwner) || !IsOrgAdmin(RoleAdmin) || IsOrgAdmin(RoleMember) || IsOrgAdmin("") {
		t.Fatal("IsOrgAdmin")
	}
	if Highest([]string{"member", "admin"}) != RoleAdmin || Highest([]string{"member", "owner", "admin"}) != RoleOwner || Highest(nil) != "" || Highest([]string{"bogus"}) != "" {
		t.Fatal("Highest")
	}
	if !IsValidInviteRole(RoleAdmin) || !IsValidInviteRole(RoleMember) || IsValidInviteRole(RoleOwner) || IsValidInviteRole("x") {
		t.Fatal("IsValidInviteRole")
	}
}

func TestTeamAccess(t *testing.T) {
	cases := []struct {
		role   string
		member bool
		want   bool
	}{
		{RoleOwner, false, true}, {RoleAdmin, false, true}, {RoleMember, true, true}, {RoleMember, false, false}, {"", true, false},
	}
	for _, c := range cases {
		if got := CanAccessTeam(c.role, c.member); got != c.want {
			t.Errorf("CanAccessTeam(%q,%v)=%v", c.role, c.member, got)
		}
	}
	if !CanManageTeams(RoleAdmin) || CanManageTeams(RoleMember) || !CanInvite(RoleOwner) || CanInvite(RoleMember) {
		t.Fatal("manage/invite")
	}
}
```

- [ ] **Step 2: Run to verify failure, then write `authz.go`**

```go
// Package authz holds Snarvei's tenancy rules as pure functions: owners and
// admins see and manage everything in their organization; members only the
// teams they belong to. No I/O, so every rule is unit-tested here and the
// middleware only gathers the facts.
package authz

const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
)

var rank = map[string]int{RoleOwner: 3, RoleAdmin: 2, RoleMember: 1}

// IsValidInviteRole reports whether an invitation may carry role.
func IsValidInviteRole(role string) bool { return role == RoleAdmin || role == RoleMember }

// IsOrgAdmin reports whether role sees every team in the organization.
func IsOrgAdmin(role string) bool { return role == RoleOwner || role == RoleAdmin }

// Highest picks the most privileged known role; "" when none is known.
func Highest(roles []string) string {
	best, bestRank := "", 0
	for _, r := range roles {
		if rank[r] > bestRank {
			best, bestRank = r, rank[r]
		}
	}
	return best
}

// CanAccessTeam: org admins always; members only when they belong to the team.
func CanAccessTeam(orgRole string, isTeamMember bool) bool {
	return IsOrgAdmin(orgRole) || (orgRole == RoleMember && isTeamMember)
}

// CanManageTeams: create teams and change team membership.
func CanManageTeams(orgRole string) bool { return IsOrgAdmin(orgRole) }

// CanInvite: create and cancel invitations.
func CanInvite(orgRole string) bool { return IsOrgAdmin(orgRole) }
```

Run: `cd apps/server && mise exec -- go test ./internal/authz/ -v`
Expected: 2 PASS.

- [ ] **Step 3: Commit**

```bash
git add apps/server/internal/authz
git commit -m "feat(server): pure organization and team authorization rules"
```

---

### Task 4: sqlc queries for auth, profile, organizations, teams and invitations

**Files:**
- Create: `apps/server/internal/db/queries/auth.sql`, `me.sql`, `orgs.sql`, `teams.sql`, `invitations.sql`
- Modify: `apps/server/internal/db/gen/*` (regenerated)

**Interfaces:**
- Produces the `gen.Queries` methods named below (exact names are what Tasks 5 to 10 call). Column aliases become field names in the generated row structs (`sqlc` camel-cases them: `user_id` → `UserID`, `active_organization_id` → `ActiveOrganizationID`).

- [ ] **Step 1: Write `auth.sql`**

```sql
-- Queries internal/auth runs against Limen's tables. Confined to that package.

-- name: GetAuthSession :one
SELECT
    u."id" AS user_id,
    COALESCE(u."name", '') AS name,
    u."email" AS email,
    u."image" AS image,
    u."two_factor_enabled" AS two_factor_enabled,
    s."id" AS session_id,
    s."expires_at" AS expires_at,
    COALESCE(s."active_organization_id", '') AS active_organization_id
FROM "sessions" s
JOIN "users" u ON u."id" = s."user_id"
WHERE s."token" = $1 AND s."user_id" = $2;

-- name: GetSessionRecord :one
SELECT "user_id", "token", "expires_at" FROM "sessions" WHERE "token" = $1;

-- name: SetSessionActiveOrganization :exec
UPDATE "sessions" SET "active_organization_id" = $2 WHERE "token" = $1;

-- name: ClearActiveOrganizationForUser :exec
UPDATE "sessions" SET "active_organization_id" = NULL WHERE "user_id" = $1 AND "active_organization_id" = $2;

-- name: CountOrganizationMembership :one
SELECT COUNT(*)::int FROM "organization_members" WHERE "organization_id" = $1 AND "user_id" = $2;

-- name: GetMemberRoles :many
-- Every role row the member holds; authz.Highest picks the one that counts.
SELECT COALESCE(r."role", '') AS role
FROM "organization_members" m
JOIN "organization_member_roles" r ON r."member_id" = m."id"
WHERE m."organization_id" = $1 AND m."user_id" = $2;

-- name: GetInvitationToken :one
SELECT "token", "organization_id", "email", "status" FROM "organization_invitations" WHERE "id" = $1;

-- name: CountUsersByEmail :one
SELECT COUNT(*)::int FROM "users" WHERE lower("email") = lower($1);

-- name: DeleteUser :exec
DELETE FROM "users" WHERE "id" = $1;
```

- [ ] **Step 2: Write `me.sql`**

```sql
-- name: GetUserProfile :one
SELECT "id", COALESCE("name", '') AS name, "email", "image", "two_factor_enabled" FROM "users" WHERE "id" = $1;

-- name: UpdateUserName :exec
UPDATE "users" SET "name" = $2, "updated_at" = now() WHERE "id" = $1;

-- name: UpdateUserImage :exec
UPDATE "users" SET "image" = $2, "updated_at" = now() WHERE "id" = $1;

-- name: UpdateUserEmail :exec
UPDATE "users" SET "email" = $2, "updated_at" = now() WHERE "id" = $1;

-- name: ListUserSessions :many
SELECT "id", "token", "created_at", "last_access", "expires_at", COALESCE("metadata", '') AS metadata
FROM "sessions"
WHERE "user_id" = $1 AND "expires_at" > now()
ORDER BY "created_at" DESC;

-- name: GetUserSessionByID :one
SELECT "id", "token" FROM "sessions" WHERE "id" = $1 AND "user_id" = $2;

-- name: CreateEmailChangeRequest :exec
INSERT INTO "email_change_requests" ("id", "user_id", "new_email", "token_hash", "expires_at")
VALUES ($1, $2, $3, $4, $5);

-- name: DeleteEmailChangeRequestsForUser :exec
DELETE FROM "email_change_requests" WHERE "user_id" = $1;

-- name: GetEmailChangeRequest :one
SELECT "id", "user_id", "new_email", "expires_at" FROM "email_change_requests" WHERE "token_hash" = $1;

-- name: ListOrganizationsWhereSoleOwner :many
-- Organizations where this user holds the owner role and nobody else does.
SELECT o."id", o."name"
FROM "organizations" o
JOIN "organization_members" m ON m."organization_id" = o."id" AND m."user_id" = $1
JOIN "organization_member_roles" r ON r."member_id" = m."id" AND r."role" = 'owner'
WHERE (
    SELECT COUNT(*) FROM "organization_members" m2
    JOIN "organization_member_roles" r2 ON r2."member_id" = m2."id" AND r2."role" = 'owner'
    WHERE m2."organization_id" = o."id" AND m2."user_id" <> $1
) = 0;
```

- [ ] **Step 3: Write `orgs.sql`**

```sql
-- name: ListOrganizationsForUser :many
-- One row per organization with the member's highest role, owner > admin > member.
SELECT o."id", o."name", o."slug", o."created_at",
    (SELECT r."role" FROM "organization_member_roles" r
     WHERE r."member_id" = m."id"
     ORDER BY CASE r."role" WHEN 'owner' THEN 3 WHEN 'admin' THEN 2 WHEN 'member' THEN 1 ELSE 0 END DESC
     LIMIT 1) AS role
FROM "organizations" o
JOIN "organization_members" m ON m."organization_id" = o."id" AND m."user_id" = $1
ORDER BY o."name";

-- name: GetOrganization :one
SELECT "id", "name", "slug" FROM "organizations" WHERE "id" = $1;

-- name: GetOrganizationBySlug :one
SELECT "id", "name", "slug" FROM "organizations" WHERE "slug" = $1;

-- name: ListOrganizationMembers :many
SELECT m."id" AS member_id, u."id" AS user_id, COALESCE(u."name", '') AS name, u."email", m."created_at",
    (SELECT r."role" FROM "organization_member_roles" r
     WHERE r."member_id" = m."id"
     ORDER BY CASE r."role" WHEN 'owner' THEN 3 WHEN 'admin' THEN 2 WHEN 'member' THEN 1 ELSE 0 END DESC
     LIMIT 1) AS role
FROM "organization_members" m
JOIN "users" u ON u."id" = m."user_id"
WHERE m."organization_id" = $1
ORDER BY m."created_at";
```

- [ ] **Step 4: Write `teams.sql`**

```sql
-- name: CreateTeam :one
INSERT INTO "teams" ("id", "organization_id", "name") VALUES ($1, $2, $3)
RETURNING "id", "organization_id", "name", "created_at", "updated_at";

-- name: GetTeam :one
SELECT "id", "organization_id", "name", "created_at", "updated_at" FROM "teams" WHERE "id" = $1;

-- name: ListTeams :many
-- Every team in the organization with its member count (org admins).
SELECT t."id", t."organization_id", t."name", t."created_at", t."updated_at",
    (SELECT COUNT(*)::int FROM "team_members" tm WHERE tm."team_id" = t."id") AS member_count
FROM "teams" t
WHERE t."organization_id" = $1
ORDER BY t."name";

-- name: ListTeamsForMember :many
-- Only the teams the user belongs to (org members).
SELECT t."id", t."organization_id", t."name", t."created_at", t."updated_at",
    (SELECT COUNT(*)::int FROM "team_members" tm WHERE tm."team_id" = t."id") AS member_count
FROM "teams" t
JOIN "team_members" me ON me."team_id" = t."id" AND me."user_id" = $2
WHERE t."organization_id" = $1
ORDER BY t."name";

-- name: IsTeamMember :one
SELECT COUNT(*)::int FROM "team_members" WHERE "team_id" = $1 AND "user_id" = $2;

-- name: AddTeamMember :exec
INSERT INTO "team_members" ("team_id", "user_id") VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: RemoveTeamMember :execrows
DELETE FROM "team_members" WHERE "team_id" = $1 AND "user_id" = $2;

-- name: ListTeamMembers :many
SELECT tm."user_id", COALESCE(u."name", '') AS name, u."email", tm."created_at"
FROM "team_members" tm
JOIN "users" u ON u."id" = tm."user_id"
WHERE tm."team_id" = $1
ORDER BY tm."created_at";

-- name: ListAccessibleTeamIDs :many
-- Team ids a member may see in an organization (org admins use ListTeams).
SELECT t."id" FROM "teams" t
JOIN "team_members" tm ON tm."team_id" = t."id" AND tm."user_id" = $2
WHERE t."organization_id" = $1;
```

- [ ] **Step 5: Write `invitations.sql`**

```sql
-- name: GetInvitation :one
-- The public view plus everything accept/register need. inviter/team joins
-- are LEFT so a deleted inviter or team never hides the invitation.
SELECT i."id", i."organization_id", i."email", COALESCE(i."roles", '') AS roles, i."status", i."token",
    i."expires_at", i."created_at",
    o."name" AS organization_name, o."slug" AS organization_slug,
    COALESCE(u."name", '') AS inviter_name,
    it."team_id" AS team_id, t."name" AS team_name
FROM "organization_invitations" i
JOIN "organizations" o ON o."id" = i."organization_id"
LEFT JOIN "users" u ON u."id" = i."inviter_id"
LEFT JOIN "invitation_teams" it ON it."invitation_id" = i."id"
LEFT JOIN "teams" t ON t."id" = it."team_id"
WHERE i."id" = $1;

-- name: ListPendingInvitations :many
SELECT i."id", i."email", COALESCE(i."roles", '') AS roles, i."status", i."expires_at", i."created_at",
    it."team_id" AS team_id, t."name" AS team_name
FROM "organization_invitations" i
LEFT JOIN "invitation_teams" it ON it."invitation_id" = i."id"
LEFT JOIN "teams" t ON t."id" = it."team_id"
WHERE i."organization_id" = $1 AND i."status" = 'pending'
ORDER BY i."created_at" DESC;

-- name: SetInvitationTeam :exec
INSERT INTO "invitation_teams" ("invitation_id", "team_id") VALUES ($1, $2)
ON CONFLICT ("invitation_id") DO UPDATE SET "team_id" = EXCLUDED."team_id";
```

- [ ] **Step 6: Generate and compile**

```bash
cd apps/server && mise exec -- go generate ./... && mise exec -- go vet ./... && mise exec -- go test -p 1 -count=1 ./internal/db/ -run TestSqlcOutputIsUpToDate -v
```

Expected: `sqlc generate` succeeds (fix any SQL it rejects; `$1` reused in `ListOrganizationsWhereSoleOwner` is fine), vet clean, drift guard PASS. Inspect `internal/db/gen/*.sql.go` once and note the generated method names and row struct fields; Tasks 5 to 10 use them as written above. Where sqlc named a field differently (for example `ExpiresAt pgtype.Timestamptz`), that generated name is the one to use.

- [ ] **Step 7: Commit**

```bash
git add apps/server/internal/db
git commit -m "feat(server): sqlc queries for auth, profile, organizations, teams and invitations"
```

---

### Task 5: `internal/auth`: Limen behind `auth.Service`

**Files:**
- Create: `apps/server/internal/auth/errors.go`, `routes.go`, `core_plugin.go`, `session.go`, `auth.go`, `auth_test.go`, `boundary_test.go`
- Modify: `apps/server/go.mod`, `go.sum` (Limen pins)

**Interfaces:**
- Consumes: `gen.Queries` (Task 4: `GetAuthSession`, `GetSessionRecord`, `SetSessionActiveOrganization`, `CountOrganizationMembership`, `GetInvitationToken`, `DeleteUser`), `email.Sender`, `email.Invitation/PasswordReset`.
- Produces:
  ```go
  package auth
  const BasePath = "/api/auth"
  const SessionCookieName = "snarvei_session"
  type Session struct { UserID, Name, Email string; Image *string; TwoFactorEnabled bool; SessionID, Token string; ExpiresAt time.Time; ActiveOrganizationID string }
  type Organization struct { ID, Name, Slug string }
  type Invitation struct { ID, OrganizationID, Email, Role, Status string; ExpiresAt *time.Time }
  type Config struct { AppURL, AppName, Secret string; OpenSignup bool; Pool *pgxpool.Pool; ClientIP func(*http.Request) string; Email email.Sender; Log *slog.Logger }
  type Service interface {
      Handler() http.Handler
      SessionFromRequest(w http.ResponseWriter, r *http.Request) (*Session, error)   // nil,nil when signed out; writes a refreshed cookie
      CreateUser(ctx context.Context, name, email, password string) (userID string, err error)
      VerifyPassword(ctx context.Context, userID, password string) error            // ErrInvalidPassword
      StartSession(ctx context.Context, w http.ResponseWriter, r *http.Request, userID string) error
      CreateOrganization(ctx context.Context, userID, name, slug string) (*Organization, error)   // ErrSlugTaken
      SetActiveOrganization(ctx context.Context, sessionToken, orgID string) error   // ErrNotMember
      CreateInvitation(ctx context.Context, inviterUserID, orgID, email, role string) (*Invitation, error)
      AcceptInvitation(ctx context.Context, userID, invitationID string) (*Invitation, error)
      RejectInvitation(ctx context.Context, userID, invitationID string) error
      CancelInvitation(ctx context.Context, actorUserID, orgID, invitationID string) error
      RevokeSession(ctx context.Context, token string) error
      RevokeAllSessions(ctx context.Context, userID string) error
      DeleteUser(ctx context.Context, userID string) error
  }
  func New(cfg Config) (Service, error)
  // errors.go sentinels: ErrInvalidPassword, ErrEmailTaken, ErrSlugTaken, ErrNotMember, ErrAlreadyMember, ErrInvitationExists, ErrInvitationEmailMismatch, ErrInvitationInvalid, ErrForbidden, ErrNotFound, ErrSessionNotFound; type PasswordPolicyError struct{ Requirement string }
  ```
- Ruling recorded in this plan: organization listing and role lookups are plain sqlc queries used by handlers and middleware (Task 4), not Service methods; the Service carries only what needs Limen.

- [ ] **Step 1: Pin the Limen modules**

```bash
cd apps/server && mise exec -- go get github.com/thecodearcher/limen@v0.2.1 github.com/thecodearcher/limen/adapters/sql@v0.2.0 \
  github.com/thecodearcher/limen/plugins/credential-password@v0.2.0 github.com/thecodearcher/limen/plugins/two-factor@v0.2.0 \
  github.com/thecodearcher/limen/plugins/organization@v0.1.0
```

- [ ] **Step 2: Write `errors.go` and `routes.go`**

`errors.go`:

```go
package auth

import (
	"errors"
	"fmt"
)

// Sentinels callers branch on. Handlers map them to HTTP codes; nothing
// outside this package ever sees a Limen error value.
var (
	ErrInvalidPassword         = errors.New("auth: invalid password")
	ErrEmailTaken              = errors.New("auth: email already in use")
	ErrSlugTaken               = errors.New("auth: organization slug already in use")
	ErrNotMember               = errors.New("auth: not a member of this organization")
	ErrAlreadyMember           = errors.New("auth: already a member of this organization")
	ErrInvitationExists        = errors.New("auth: a pending invitation already exists for this email")
	ErrInvitationEmailMismatch = errors.New("auth: invitation was sent to a different email address")
	ErrInvitationInvalid       = errors.New("auth: invitation is no longer valid")
	ErrForbidden               = errors.New("auth: forbidden")
	ErrNotFound                = errors.New("auth: not found")
	ErrSessionNotFound         = errors.New("auth: session not found")
	ErrUnknownRole             = errors.New("auth: unknown role")
)

// PasswordPolicyError says a password failed Limen's policy (min 8 chars,
// an uppercase letter, a digit). Handlers answer 400 with the requirement.
type PasswordPolicyError struct{ Requirement string }

func (e *PasswordPolicyError) Error() string { return "auth: password " + e.Requirement }

func wrap(op string, err error) error { return fmt.Errorf("auth: %s: %w", op, err) }
```

`routes.go`:

```go
package auth

// knownRouteIDs is every route the registered Limen plugins can mount at
// the pinned versions. The HTTP surface is an ALLOWLIST: everything here that
// is not in allowedRouteIDs is disabled. Revisit on every Limen upgrade; a
// route added upstream and not named here would be silently enabled, which
// is why TestLimenRouteAllowlist probes concrete paths.
var knownRouteIDs = []string{
	// core
	"me", "list-sessions", "signout", "revoke-sessions", "verify-email", "email-verifications",
	// credential-password
	"signin", "signup", "passwords-request-reset", "passwords-reset", "passwords-change", "passwords-set", "usernames-check",
	// two-factor
	"two-factor-initiate-setup", "two-factor-finalize-setup", "two-factor-disable", "two-factor-verify",
	"get-backup-codes", "totp-uri", "otp-send",
	// organization
	"organizations:create", "organizations:list", "organizations:check-slug", "organizations:update", "organizations:delete",
	"organizations:members-list", "organizations:member-get", "organizations:get-active", "organizations:switch",
	"organizations:leave-organization", "organizations:invite-member", "organizations:respond-to-invitation",
	"organizations:get-invitation-by-token", "organizations:cancel-pending-invitation", "organizations:list-invitations",
	"organizations:revoke-member-role", "organizations:assign-member-role", "organizations:remove-member",
	"organizations:create-role", "organizations:list-roles", "organizations:update-role", "organizations:delete-role",
}

// allowedRouteIDs is what the SPA reaches on /api/auth/*. Sessions list and
// revoke are Snarvei's own (Limen's serialise the token); every organization
// and invitation route is Snarvei's own (invitations carry a team, roles are
// enforced in one place); email OTP is not offered.
func allowedRouteIDs(openSignup bool) []string {
	allowed := []string{
		"me", "signout", "signin",
		"passwords-change", "passwords-request-reset", "passwords-reset",
		"two-factor-initiate-setup", "two-factor-finalize-setup", "two-factor-disable", "two-factor-verify",
		"get-backup-codes", "totp-uri",
	}
	if openSignup {
		allowed = append(allowed, "signup")
	}
	return allowed
}

func disabledRouteIDs(openSignup bool) []string {
	allowed := map[string]struct{}{}
	for _, id := range allowedRouteIDs(openSignup) {
		allowed[id] = struct{}{}
	}
	var disabled []string
	for _, id := range knownRouteIDs {
		if _, ok := allowed[id]; !ok {
			disabled = append(disabled, id)
		}
	}
	return disabled
}
```

- [ ] **Step 3: Write `core_plugin.go`**

```go
package auth

import (
	"errors"

	"github.com/thecodearcher/limen"
)

// corePlugin exists because *limen.LimenCore is not reachable from the
// *limen.Limen handle, but every plugin receives it in Initialize. It keeps
// the pointer so this package can refresh cookies, create sessions for the
// invitation-register flow and read users through DBAction.
type corePlugin struct{ core *limen.LimenCore }

func (p *corePlugin) Name() limen.PluginName { return "snarvei-core" }

func (p *corePlugin) Initialize(core *limen.LimenCore) error {
	if core == nil {
		return errors.New("auth: limen initialized the plugin with a nil core")
	}
	p.core = core
	return nil
}

func (p *corePlugin) PluginHTTPConfig() limen.PluginHTTPConfig { return limen.PluginHTTPConfig{} }

func (p *corePlugin) RegisterRoutes(*limen.LimenHTTPCore, *limen.RouteBuilder) {}

var _ limen.Plugin = (*corePlugin)(nil)
```

- [ ] **Step 4: Write `session.go`**

```go
package auth

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/thecodearcher/limen"

	"github.com/refsdal/snarvei/server/internal/db/gen"
)

// SessionFromRequest validates the cookie or bearer token through Limen,
// then loads everything the app branches on from our own tables in one
// query. (nil, nil) means nobody is signed in. When Limen extended the
// session while validating it, the refreshed cookie is written to w.
func (s *service) SessionFromRequest(w http.ResponseWriter, r *http.Request) (*Session, error) {
	validated, err := s.limen.GetSession(r)
	if err != nil {
		if isSignedOut(err) {
			return nil, nil
		}
		return nil, wrap("validate session", err)
	}
	if validated == nil || validated.User == nil || validated.Session == nil {
		return nil, nil
	}
	row, err := s.q.GetAuthSession(r.Context(), gen.GetAuthSessionParams{
		Token:  validated.Session.Token,
		UserID: idString(validated.User.ID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, wrap("load session", err)
	}
	if validated.Refreshed != nil && w != nil {
		if err := s.core.Cookies().SetSessionCookie(w, validated.Refreshed); err != nil {
			return nil, wrap("refresh cookie", err)
		}
	}
	return &Session{
		UserID:               row.UserID,
		Name:                 row.Name,
		Email:                row.Email,
		Image:                row.Image,
		TwoFactorEnabled:     row.TwoFactorEnabled,
		SessionID:            row.SessionID,
		Token:                validated.Session.Token,
		ExpiresAt:            row.ExpiresAt.Time,
		ActiveOrganizationID: row.ActiveOrganizationID,
	}, nil
}

func isSignedOut(err error) bool {
	return errors.Is(err, limen.ErrSessionNotFound) || errors.Is(err, limen.ErrSessionExpired) ||
		errors.Is(err, limen.ErrSessionInvalid) || errors.Is(err, limen.ErrRecordNotFound)
}
```

(`row.Image` is `*string` and `row.ExpiresAt` is `pgtype.Timestamptz` in sqlc's output; adjust to the generated field types.)

- [ ] **Step 5: Write `auth.go`**

```go
// Package auth is the ONLY package that imports Limen. Everything else
// consumes Service and Session. The instance is built once at startup.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/thecodearcher/limen"
	sqladapter "github.com/thecodearcher/limen/adapters/sql"
	credentialpassword "github.com/thecodearcher/limen/plugins/credential-password"
	"github.com/thecodearcher/limen/plugins/organization"
	twofactor "github.com/thecodearcher/limen/plugins/two-factor"

	"github.com/refsdal/snarvei/server/internal/authz"
	"github.com/refsdal/snarvei/server/internal/db/gen"
	"github.com/refsdal/snarvei/server/internal/email"
)

// BasePath is where Limen's router is mounted; it must match the mount point exactly.
const BasePath = "/api/auth"

// SessionCookieName is pinned so an upgrade cannot rename it under us.
const SessionCookieName = "snarvei_session"

// Session is what the rest of the app knows about the caller.
type Session struct {
	UserID               string
	Name                 string
	Email                string
	Image                *string
	TwoFactorEnabled     bool
	SessionID            string
	Token                string
	ExpiresAt            time.Time
	ActiveOrganizationID string // "" when none
}

// Organization is the subset of Limen's organization the app uses.
type Organization struct{ ID, Name, Slug string }

// Invitation is the subset of Limen's invitation the app uses.
type Invitation struct {
	ID, OrganizationID, Email, Role, Status string
	ExpiresAt                              *time.Time
}

// Config is what the composition root supplies.
type Config struct {
	AppURL     string
	AppName    string
	Secret     string
	OpenSignup bool
	Pool       *pgxpool.Pool
	// ClientIP returns the keyed digest of the client address; Limen uses it
	// for session metadata and rate-limit keys so no raw address is stored.
	ClientIP func(*http.Request) string
	Email    email.Sender
	Log      *slog.Logger
}

// Service is the entire auth surface the rest of the app may use.
type Service interface {
	Handler() http.Handler
	SessionFromRequest(w http.ResponseWriter, r *http.Request) (*Session, error)
	CreateUser(ctx context.Context, name, email, password string) (string, error)
	VerifyPassword(ctx context.Context, userID, password string) error
	StartSession(ctx context.Context, w http.ResponseWriter, r *http.Request, userID string) error
	CreateOrganization(ctx context.Context, userID, name, slug string) (*Organization, error)
	SetActiveOrganization(ctx context.Context, sessionToken, orgID string) error
	CreateInvitation(ctx context.Context, inviterUserID, orgID, emailAddr, role string) (*Invitation, error)
	AcceptInvitation(ctx context.Context, userID, invitationID string) (*Invitation, error)
	RejectInvitation(ctx context.Context, userID, invitationID string) error
	CancelInvitation(ctx context.Context, actorUserID, orgID, invitationID string) error
	RevokeSession(ctx context.Context, token string) error
	RevokeAllSessions(ctx context.Context, userID string) error
	DeleteUser(ctx context.Context, userID string) error
}

type service struct {
	limen *limen.Limen
	core  *limen.LimenCore
	org   organization.API
	cred  credentialpassword.API
	pool  *pgxpool.Pool
	q     *gen.Queries
	cfg   Config
	log   *slog.Logger
}

var _ Service = (*service)(nil)

// New builds the Limen instance and wires it to Snarvei's schema.
func New(cfg Config) (Service, error) {
	switch {
	case cfg.Pool == nil:
		return nil, errors.New("auth: a database pool is required")
	case cfg.AppURL == "":
		return nil, errors.New("auth: AppURL is required")
	case len(cfg.Secret) < 32:
		return nil, errors.New("auth: Secret must be at least 32 bytes")
	case cfg.ClientIP == nil:
		return nil, errors.New("auth: ClientIP extractor is required")
	case cfg.Email == nil:
		return nil, errors.New("auth: Email sender is required")
	}
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	if cfg.AppName == "" {
		cfg.AppName = "Snarvei"
	}

	sqlDB := stdlib.OpenDBFromPool(cfg.Pool)
	secret := sha256.Sum256([]byte(cfg.Secret))
	core := &corePlugin{}
	appURL := strings.TrimRight(cfg.AppURL, "/")

	credPlugin := credentialpassword.New(
		credentialpassword.WithSendPasswordResetEmail(func(to, token string) {
			link := appURL + "/reset-password?token=" + token
			if err := cfg.Email.Send(context.Background(), email.PasswordReset(cfg.AppName, link).To(to)); err != nil {
				cfg.Log.Warn("password reset mail failed", "event", "email.send_failed", "to", to, "error", err.Error())
			}
		}),
	)
	twoFactorPlugin := twofactor.New(
		twofactor.WithTOTP(twofactor.WithTOTPIssuer(cfg.AppName)),
		twofactor.WithRevokeOtherSessionsOnStateChange(true),
	)
	var svc *service
	orgPlugin := organization.New(
		organization.WithSlugGenerator(func(_ string, provided string) string { return provided }),
		organization.WithSendInvitationMail(func(ctx context.Context, data *organization.SendInvitationMailData) {
			inviter := ""
			if data.Inviter != nil {
				if name, ok := data.Inviter.Raw()["name"].(string); ok {
					inviter = name
				}
			}
			link := appURL + "/app/invitations/" + idString(data.Invitation.ID)
			msg := email.Invitation(cfg.AppName, data.Organization.Name, inviter, link).To(data.Invitation.Email)
			if err := cfg.Email.Send(ctx, msg); err != nil {
				cfg.Log.Warn("invitation mail failed", "event", "email.send_failed", "to", data.Invitation.Email, "error", err.Error())
			}
		}),
	)

	instance, err := limen.New(&limen.Config{
		BaseURL:  appURL,
		Database: sqladapter.NewPostgreSQL(sqlDB),
		Secret:   secret[:],
		Schema: limen.NewDefaultSchemaConfig(
			limen.WithSchemaIDGenerator(uuidGenerator{}),
			// The sign-up route carries {name, email, password}; Limen only
			// knows email/password, so name is picked off the body here.
			limen.WithSchemaUser(limen.WithUserAdditionalFields(func(ctx *limen.AdditionalFieldsContext) (map[string]any, error) {
				if !ctx.IsCreate() {
					return nil, nil
				}
				fields := map[string]any{}
				if name, ok := ctx.GetBodyValue("name").(string); ok && strings.TrimSpace(name) != "" {
					fields["name"] = strings.TrimSpace(name)
				}
				return fields, nil
			})),
		),
		Session: limen.NewDefaultSessionConfig(
			limen.WithSessionIPAddressExtractor(cfg.ClientIP),
		),
		HTTP: limen.NewDefaultHTTPConfig(
			limen.WithHTTPBasePath(BasePath),
			limen.WithHTTPSessionCookieName(SessionCookieName),
			limen.WithHTTPCookieSecure(strings.HasPrefix(appURL, "https://")),
			limen.WithHTTPTrustedOrigins([]string{appURL}),
			limen.WithHTTPDisabledPaths(disabledRouteIDs(cfg.OpenSignup)),
			limen.WithHTTPRateLimiter(
				limen.WithRateLimiterStore(limen.StoreTypeDatabase),
				limen.WithRateLimiterKeyGenerator(cfg.ClientIP),
				limen.WithRateLimiterMaxRequests(60),
				limen.WithRateLimiterWindow(time.Minute),
			),
		),
		Plugins: []limen.Plugin{credPlugin, twoFactorPlugin, orgPlugin, core},
	})
	if err != nil {
		return nil, fmt.Errorf("auth: build limen: %w", err)
	}
	if core.core == nil {
		return nil, errors.New("auth: limen did not initialize the core plugin")
	}
	svc = &service{
		limen: instance,
		core:  core.core,
		org:   organization.Use(instance),
		cred:  credentialpassword.Use(instance),
		pool:  cfg.Pool,
		q:     gen.New(cfg.Pool),
		cfg:   cfg,
		log:   cfg.Log,
	}
	return svc, nil
}

func (s *service) Handler() http.Handler { return s.limen.Handler() }

func (s *service) CreateUser(ctx context.Context, name, emailAddr, password string) (string, error) {
	emailAddr = limen.NormalizeEmail(emailAddr)
	result, err := s.cred.SignUpWithCredentialAndPassword(ctx, &limen.User{Email: emailAddr, Password: &password}, map[string]any{"name": strings.TrimSpace(name)})
	if err != nil {
		return "", mapError("create user", err)
	}
	return idString(result.User.ID), nil
}

func (s *service) VerifyPassword(ctx context.Context, userID, password string) error {
	user, err := s.core.DBAction.FindUserByID(ctx, userID)
	if err != nil {
		return mapError("load user", err)
	}
	ok, err := s.cred.ComparePassword(password, user.Password)
	if err != nil {
		return wrap("compare password", err)
	}
	if !ok {
		return ErrInvalidPassword
	}
	return nil
}

func (s *service) StartSession(ctx context.Context, w http.ResponseWriter, r *http.Request, userID string) error {
	user, err := s.core.DBAction.FindUserByID(ctx, userID)
	if err != nil {
		return mapError("load user", err)
	}
	result, err := s.core.CreateSession(ctx, r, w, &limen.AuthenticationResult{User: user})
	if err != nil {
		return wrap("create session", err)
	}
	if err := s.core.Cookies().SetSessionCookie(w, result); err != nil {
		return wrap("set session cookie", err)
	}
	return nil
}

func (s *service) CreateOrganization(ctx context.Context, userID, name, slug string) (*Organization, error) {
	user, err := s.core.DBAction.FindUserByID(ctx, userID)
	if err != nil {
		return nil, mapError("load user", err)
	}
	org, err := s.org.CreateOrganization(ctx, user, &organization.CreateOrganizationRequest{Name: name, Slug: slug})
	if err != nil {
		return nil, mapError("create organization", err)
	}
	return &Organization{ID: idString(org.ID), Name: org.Name, Slug: org.Slug}, nil
}

// SetActiveOrganization checks membership first: the middleware trusts the
// session's active organization, so writing it unchecked would let any user
// read any organization by naming its id.
func (s *service) SetActiveOrganization(ctx context.Context, sessionToken, orgID string) error {
	record, err := s.q.GetSessionRecord(ctx, sessionToken)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrSessionNotFound
		}
		return wrap("load session", err)
	}
	n, err := s.q.CountOrganizationMembership(ctx, gen.CountOrganizationMembershipParams{OrganizationID: orgID, UserID: record.UserID})
	if err != nil {
		return wrap("check membership", err)
	}
	if n == 0 {
		return ErrNotMember
	}
	if err := s.q.SetSessionActiveOrganization(ctx, gen.SetSessionActiveOrganizationParams{Token: sessionToken, ActiveOrganizationID: &orgID}); err != nil {
		return wrap("set active organization", err)
	}
	return nil
}

func (s *service) CreateInvitation(ctx context.Context, inviterUserID, orgID, emailAddr, role string) (*Invitation, error) {
	if !authz.IsValidInviteRole(role) {
		return nil, ErrUnknownRole
	}
	inviter, err := s.core.DBAction.FindUserByID(ctx, inviterUserID)
	if err != nil {
		return nil, mapError("load inviter", err)
	}
	org, err := s.org.GetOrganization(ctx, orgID)
	if err != nil {
		return nil, mapError("load organization", err)
	}
	inv, err := s.org.CreateInvitation(ctx, inviter, org, &organization.CreateInvitationRequest{Email: emailAddr, Role: role})
	if err != nil {
		return nil, mapError("create invitation", err)
	}
	return toInvitation(inv), nil
}

func (s *service) respond(ctx context.Context, userID, invitationID string, response organization.InvitationResponse) (*Invitation, error) {
	user, err := s.core.DBAction.FindUserByID(ctx, userID)
	if err != nil {
		return nil, mapError("load user", err)
	}
	row, err := s.q.GetInvitationToken(ctx, invitationID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, wrap("load invitation", err)
	}
	if row.Status != string(organization.InvitationStatusPending) {
		return nil, ErrInvitationInvalid
	}
	inv, err := s.org.RespondToInvitation(ctx, user, row.Token, response)
	if err != nil {
		return nil, mapError("respond to invitation", err)
	}
	return toInvitation(inv), nil
}

func (s *service) AcceptInvitation(ctx context.Context, userID, invitationID string) (*Invitation, error) {
	return s.respond(ctx, userID, invitationID, organization.InvitationResponseAccept)
}

func (s *service) RejectInvitation(ctx context.Context, userID, invitationID string) error {
	_, err := s.respond(ctx, userID, invitationID, organization.InvitationResponseReject)
	return err
}

func (s *service) CancelInvitation(ctx context.Context, actorUserID, orgID, invitationID string) error {
	actor, err := s.core.DBAction.FindUserByID(ctx, actorUserID)
	if err != nil {
		return mapError("load user", err)
	}
	org, err := s.org.GetOrganization(ctx, orgID)
	if err != nil {
		return mapError("load organization", err)
	}
	if _, err := s.org.CancelPendingInvitation(ctx, actor, org, invitationID); err != nil {
		return mapError("cancel invitation", err)
	}
	return nil
}

func (s *service) RevokeSession(ctx context.Context, token string) error {
	if err := s.limen.RevokeSession(ctx, token); err != nil {
		return mapError("revoke session", err)
	}
	return nil
}

func (s *service) RevokeAllSessions(ctx context.Context, userID string) error {
	if err := s.limen.RevokeAllSessions(ctx, userID); err != nil {
		return wrap("revoke sessions", err)
	}
	return nil
}

func (s *service) DeleteUser(ctx context.Context, userID string) error {
	if err := s.RevokeAllSessions(ctx, userID); err != nil {
		return err
	}
	if err := s.q.DeleteUser(ctx, userID); err != nil {
		return wrap("delete user", err)
	}
	return nil
}

// --- helpers ---------------------------------------------------------

func toInvitation(inv *organization.Invitation) *Invitation {
	role := ""
	if len(inv.Roles) > 0 {
		role = idString(inv.Roles[0])
	}
	return &Invitation{
		ID: idString(inv.ID), OrganizationID: idString(inv.OrganizationID), Email: inv.Email,
		Role: role, Status: string(inv.Status), ExpiresAt: inv.ExpiresAt,
	}
}

// mapError translates Limen's errors into this package's sentinels so no
// Limen type crosses the package boundary.
func mapError(op string, err error) error {
	switch {
	case errors.Is(err, credentialpassword.ErrEmailAlreadyExists):
		return ErrEmailTaken
	case errors.Is(err, credentialpassword.ErrInvalidCredential), errors.Is(err, credentialpassword.ErrInvalidPassword), errors.Is(err, credentialpassword.ErrInvalidCurrentPassword):
		return ErrInvalidPassword
	case errors.Is(err, credentialpassword.ErrPasswordTooShort):
		return &PasswordPolicyError{Requirement: "must be at least 8 characters"}
	case errors.Is(err, credentialpassword.ErrPasswordRequiresUppercase):
		return &PasswordPolicyError{Requirement: "must contain an uppercase letter"}
	case errors.Is(err, credentialpassword.ErrPasswordRequiresNumbers):
		return &PasswordPolicyError{Requirement: "must contain a number"}
	case errors.Is(err, organization.ErrOrganizationSlugAlreadyExists), errors.Is(err, organization.ErrInvalidSlug):
		return ErrSlugTaken
	case errors.Is(err, organization.ErrUserAlreadyInOrganization), errors.Is(err, organization.ErrMemberAlreadyExists):
		return ErrAlreadyMember
	case errors.Is(err, organization.ErrInvitationAlreadyExists):
		return ErrInvitationExists
	case errors.Is(err, organization.ErrInvitationEmailMismatch):
		return ErrInvitationEmailMismatch
	case errors.Is(err, organization.ErrInvalidInvitation):
		return ErrInvitationInvalid
	case errors.Is(err, organization.ErrInsufficientPermission), errors.Is(err, organization.ErrUserCannotInviteOwner):
		return ErrForbidden
	case errors.Is(err, limen.ErrRecordNotFound):
		return ErrNotFound
	}
	return wrap(op, err)
}

type uuidGenerator struct{}

func (uuidGenerator) Generate(context.Context) (any, error) { return NewID(), nil }
func (uuidGenerator) GetColumnType() limen.ColumnType     { return limen.ColumnTypeString }

// NewID returns a random UUIDv4 string; the same generator Limen uses for
// its tables, exported for Snarvei's own inserts.
func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("auth: crypto/rand failed: %v", err))
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func idString(id any) string {
	if s, ok := id.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", id)
}

// rolesFromJSON decodes Limen's roles column (a JSON array in text) — used by
// handlers that read organization_invitations.roles through sqlc.
func RolesFromJSON(raw string) []string {
	var out []string
	if raw == "" {
		return out
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return []string{raw}
	}
	return out
}
```

Notes for the implementer: (1) if Limen's organization `CreateInvitation` rejects a plain-string role for `Role any`, pass the role name as the string is what `validateAndResolveInvitationRole` resolves by name; (2) `ctx.GetBodyValue("name")` exists on `*limen.AdditionalFieldsContext`; (3) `limen.WithRateLimiterStore` takes a `limen.StoreType`; (4) if `twofactor.WithRevokeOtherSessionsOnStateChange` does not exist under that name, drop it and say so.

- [ ] **Step 6: Write the tests**

`boundary_test.go` (package `auth_test`) checks that no other package imports Limen:

```go
package auth_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOnlyThisPackageImportsLimen(t *testing.T) {
	root := "../.."
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") {
			return err
		}
		if strings.Contains(p, string(filepath.Separator)+"internal"+string(filepath.Separator)+"auth"+string(filepath.Separator)) {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if strings.Contains(string(b), `"github.com/thecodearcher/limen`) {
			t.Errorf("%s imports Limen; only internal/auth may", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
```

`auth_test.go` (package `auth_test`):

```go
package auth_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/refsdal/snarvei/server/internal/auth"
	"github.com/refsdal/snarvei/server/internal/clientip"
	"github.com/refsdal/snarvei/server/internal/email"
	"github.com/refsdal/snarvei/server/internal/testrig"
)

const testSecret = "snarvei-test-auth-secret-value-0123456789"
const password = "Testpass123"

type fixture struct {
	t    *testing.T
	svc  auth.Service
	rig  *testrig.Rig
	mux  *http.ServeMux
	mail *email.Recording
}

func newFixture(t *testing.T, openSignup bool) *fixture {
	t.Helper()
	rig := testrig.Setup(t)
	mail := email.NewRecording()
	svc, err := auth.New(auth.Config{
		AppURL: "http://localhost:3000", AppName: "Snarvei", Secret: testSecret, OpenSignup: openSignup,
		Pool: rig.Pool, ClientIP: clientip.NewHasher("", testSecret).Extractor(0), Email: mail,
	})
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	mux := http.NewServeMux()
	mux.Handle(auth.BasePath+"/", svc.Handler())
	return &fixture{t: t, svc: svc, rig: rig, mux: mux, mail: mail}
}

func (f *fixture) do(method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	f.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://localhost:3000")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	f.mux.ServeHTTP(rec, req)
	return rec
}

func (f *fixture) sessionCookie(rec *httptest.ResponseRecorder) *http.Cookie {
	f.t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.SessionCookieName && c.Value != "" {
			return c
		}
	}
	f.t.Fatalf("no session cookie: %d %s", rec.Code, rec.Body.String())
	return nil
}

func (f *fixture) signIn(name, emailAddr string) (string, *http.Cookie) {
	f.t.Helper()
	id, err := f.svc.CreateUser(context.Background(), name, emailAddr, password)
	if err != nil {
		f.t.Fatalf("CreateUser: %v", err)
	}
	rec := f.do(http.MethodPost, auth.BasePath+"/signin/credential", `{"credential":"`+emailAddr+`","password":"`+password+`"}`, nil)
	if rec.Code != http.StatusOK {
		f.t.Fatalf("sign in: %d %s", rec.Code, rec.Body.String())
	}
	return id, f.sessionCookie(rec)
}

func (f *fixture) session(cookie *http.Cookie) *auth.Session {
	f.t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.AddCookie(cookie)
	s, err := f.svc.SessionFromRequest(httptest.NewRecorder(), req)
	if err != nil {
		f.t.Fatalf("SessionFromRequest: %v", err)
	}
	return s
}

func TestSignInThenSessionFromRequest(t *testing.T) {
	f := newFixture(t, false)
	id, cookie := f.signIn("Kari", "kari@example.com")
	s := f.session(cookie)
	if s == nil || s.UserID != id || s.Name != "Kari" || s.Email != "kari@example.com" || s.Token == "" || s.SessionID == "" {
		t.Fatalf("session: %+v", s)
	}
	if s.ActiveOrganizationID != "" || s.TwoFactorEnabled {
		t.Fatalf("fresh session: %+v", s)
	}
	if got := f.session(&http.Cookie{Name: auth.SessionCookieName, Value: "bogus"}); got != nil {
		t.Fatal("bogus token must be signed out")
	}
}

func TestSignupRouteFollowsOpenSignup(t *testing.T) {
	closed := newFixture(t, false)
	rec := closed.do(http.MethodPost, auth.BasePath+"/signup/credential", `{"name":"X","email":"x@example.com","password":"`+password+`"}`, nil)
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("closed signup reachable: %d %s", rec.Code, rec.Body.String())
	}
	open := newFixture(t, true)
	rec = open.do(http.MethodPost, auth.BasePath+"/signup/credential", `{"name":"Ola","email":"ola@example.com","password":"`+password+`"}`, nil)
	if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		t.Fatalf("open signup: %d %s", rec.Code, rec.Body.String())
	}
	cookie := open.sessionCookie(rec)
	if s := open.session(cookie); s == nil || s.Name != "Ola" {
		t.Fatalf("signup must store the name: %+v", s)
	}
}

func TestLimenRouteAllowlist(t *testing.T) {
	f := newFixture(t, false)
	_, cookie := f.signIn("Kari", "kari@example.com")
	disabled := []struct{ method, path string }{
		{http.MethodGet, "/sessions"}, {http.MethodPost, "/revoke-sessions"},
		{http.MethodPost, "/signup/credential"}, {http.MethodPut, "/passwords"}, {http.MethodPost, "/usernames/check"},
		{http.MethodPost, "/two-factor/otp/send"},
		{http.MethodGet, "/organizations"}, {http.MethodPost, "/organizations"}, {http.MethodPost, "/organizations/switch"},
		{http.MethodGet, "/organizations/members"}, {http.MethodGet, "/organizations/active"}, {http.MethodPost, "/organizations/leave"},
		{http.MethodPost, "/organizations/check-slug"}, {http.MethodPost, "/organizations/invitations"},
		{http.MethodPost, "/organizations/invitations/respond"}, {http.MethodPost, "/organizations/invitations/cancel"},
		{http.MethodGet, "/organizations/invitations"}, {http.MethodDelete, "/organizations/members/m1"},
		{http.MethodPost, "/organizations/members/m1/roles/assign"}, {http.MethodPatch, "/organizations/o1"}, {http.MethodDelete, "/organizations/o1"},
	}
	for _, r := range disabled {
		rec := f.do(r.method, auth.BasePath+r.path, "{}", cookie)
		if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s reachable: %d %s", r.method, r.path, rec.Code, rec.Body.String())
		}
	}
	kept := []struct{ method, path string }{
		{http.MethodGet, "/me"}, {http.MethodPost, "/passwords/change"}, {http.MethodPost, "/passwords/request-reset"},
		{http.MethodPost, "/passwords/reset"}, {http.MethodPost, "/two-factor/initiate-setup"}, {http.MethodGet, "/two-factor/totp/uri"},
		{http.MethodGet, "/two-factor/backup-codes"}, {http.MethodPost, "/two-factor/verify"}, {http.MethodPost, "/signout"},
	}
	for _, r := range kept {
		rec := f.do(r.method, auth.BasePath+r.path, "{}", cookie)
		if rec.Code == http.StatusNotFound || rec.Code == http.StatusMethodNotAllowed {
			t.Errorf("%s %s should be reachable: %d", r.method, r.path, rec.Code)
		}
	}
}

func TestPasswordResetSendsMailAndRevokesSessions(t *testing.T) {
	f := newFixture(t, false)
	_, cookie := f.signIn("Kari", "kari@example.com")
	rec := f.do(http.MethodPost, auth.BasePath+"/passwords/request-reset", `{"email":"kari@example.com"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("request-reset: %d %s", rec.Code, rec.Body.String())
	}
	msg, ok := f.mail.Last("kari@example.com")
	if !ok || !strings.Contains(msg.Text, "/reset-password?token=") {
		t.Fatalf("reset mail: %+v", msg)
	}
	token := msg.Text[strings.Index(msg.Text, "token=")+len("token="):]
	token = strings.Fields(token)[0]
	rec = f.do(http.MethodPost, auth.BasePath+"/passwords/reset", `{"token":"`+token+`","new_password":"Newpass456"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("reset: %d %s", rec.Code, rec.Body.String())
	}
	if f.session(cookie) != nil {
		t.Fatal("old session must be revoked after a reset")
	}
	rec = f.do(http.MethodPost, auth.BasePath+"/signin/credential", `{"credential":"kari@example.com","password":"Newpass456"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("sign in with new password: %d %s", rec.Code, rec.Body.String())
	}
}

func TestOrganizationsAndInvitations(t *testing.T) {
	f := newFixture(t, false)
	ctx := context.Background()
	ownerID, ownerCookie := f.signIn("Owner", "owner@example.com")
	org, err := f.svc.CreateOrganization(ctx, ownerID, "Acme", "acme")
	if err != nil || org.Slug != "acme" {
		t.Fatalf("CreateOrganization: %v %+v", err, org)
	}
	if _, err := f.svc.CreateOrganization(ctx, ownerID, "Acme 2", "acme"); !errors.Is(err, auth.ErrSlugTaken) {
		t.Fatalf("duplicate slug: %v", err)
	}
	if err := f.svc.SetActiveOrganization(ctx, ownerCookie.Value, org.ID); err != nil {
		t.Fatalf("SetActiveOrganization: %v", err)
	}
	if s := f.session(ownerCookie); s.ActiveOrganizationID != org.ID {
		t.Fatalf("active org not reflected: %+v", s)
	}

	inv, err := f.svc.CreateInvitation(ctx, ownerID, org.ID, "member@example.com", "member")
	if err != nil || inv.Role != "member" || inv.Status != "pending" {
		t.Fatalf("CreateInvitation: %v %+v", err, inv)
	}
	if msg, ok := f.mail.Last("member@example.com"); !ok || !strings.Contains(msg.Text, "/app/invitations/"+inv.ID) || !strings.Contains(msg.Text, "invited by Owner") {
		t.Fatalf("invitation mail: %+v", msg)
	}
	if _, err := f.svc.CreateInvitation(ctx, ownerID, org.ID, "member@example.com", "member"); !errors.Is(err, auth.ErrInvitationExists) {
		t.Fatalf("duplicate invitation: %v", err)
	}
	if _, err := f.svc.CreateInvitation(ctx, ownerID, org.ID, "x@example.com", "owner"); !errors.Is(err, auth.ErrUnknownRole) {
		t.Fatalf("owner invite must be refused: %v", err)
	}

	strangerID, _ := f.signIn("Stranger", "stranger@example.com")
	if _, err := f.svc.AcceptInvitation(ctx, strangerID, inv.ID); !errors.Is(err, auth.ErrInvitationEmailMismatch) {
		t.Fatalf("mismatch: %v", err)
	}
	if _, err := f.svc.CreateInvitation(ctx, strangerID, org.ID, "y@example.com", "member"); !errors.Is(err, auth.ErrForbidden) && !errors.Is(err, auth.ErrNotFound) {
		t.Fatalf("non-member inviting: %v", err)
	}

	memberID, memberCookie := f.signIn("Member", "member@example.com")
	if err := f.svc.SetActiveOrganization(ctx, memberCookie.Value, org.ID); !errors.Is(err, auth.ErrNotMember) {
		t.Fatalf("switch before accepting: %v", err)
	}
	accepted, err := f.svc.AcceptInvitation(ctx, memberID, inv.ID)
	if err != nil || accepted.Status != "accepted" {
		t.Fatalf("AcceptInvitation: %v %+v", err, accepted)
	}
	if err := f.svc.SetActiveOrganization(ctx, memberCookie.Value, org.ID); err != nil {
		t.Fatalf("switch after accepting: %v", err)
	}
	if _, err := f.svc.AcceptInvitation(ctx, memberID, inv.ID); !errors.Is(err, auth.ErrInvitationInvalid) {
		t.Fatalf("second accept: %v", err)
	}

	inv2, _ := f.svc.CreateInvitation(ctx, ownerID, org.ID, "z@example.com", "admin")
	if err := f.svc.CancelInvitation(ctx, memberID, org.ID, inv2.ID); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("member cancelling: %v", err)
	}
	if err := f.svc.CancelInvitation(ctx, ownerID, org.ID, inv2.ID); err != nil {
		t.Fatalf("owner cancelling: %v", err)
	}
}

func TestPasswordsSessionsAndDeletion(t *testing.T) {
	f := newFixture(t, false)
	ctx := context.Background()
	id, cookie := f.signIn("Kari", "kari@example.com")
	if err := f.svc.VerifyPassword(ctx, id, password); err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if err := f.svc.VerifyPassword(ctx, id, "wrong"); !errors.Is(err, auth.ErrInvalidPassword) {
		t.Fatalf("wrong password: %v", err)
	}
	if _, err := f.svc.CreateUser(ctx, "Dup", "kari@example.com", password); !errors.Is(err, auth.ErrEmailTaken) {
		t.Fatalf("duplicate email: %v", err)
	}
	var pe *auth.PasswordPolicyError
	if _, err := f.svc.CreateUser(ctx, "Weak", "weak@example.com", "short"); !errors.As(err, &pe) {
		t.Fatalf("weak password: %v", err)
	}

	// StartSession issues a cookie without a password round trip.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/invitations/x/register", nil)
	if err := f.svc.StartSession(ctx, rec, req, id); err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	started := f.sessionCookie(rec)
	if s := f.session(started); s == nil || s.UserID != id {
		t.Fatalf("started session: %+v", s)
	}

	s := f.session(cookie)
	if err := f.svc.RevokeSession(ctx, s.Token); err != nil {
		t.Fatalf("RevokeSession: %v", err)
	}
	if f.session(cookie) != nil {
		t.Fatal("revoked session still valid")
	}
	if f.session(started) == nil {
		t.Fatal("other session must survive a single revoke")
	}
	if err := f.svc.DeleteUser(ctx, id); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if f.session(started) != nil {
		t.Fatal("deleted user still has a session")
	}
	var n int
	if err := f.rig.Pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE id = $1`, id).Scan(&n); err != nil || n != 0 {
		t.Fatalf("user row remains: %v %d", err, n)
	}
}

func TestSessionMetadataStoresNoRawAddress(t *testing.T) {
	f := newFixture(t, false)
	_, err := f.svc.CreateUser(context.Background(), "Kari", "kari@example.com", password)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, auth.BasePath+"/signin/credential", strings.NewReader(`{"credential":"kari@example.com","password":"`+password+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://localhost:3000")
	req.RemoteAddr = "203.0.113.7:4444"
	rec := httptest.NewRecorder()
	f.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("sign in: %d %s", rec.Code, rec.Body.String())
	}
	var metadata string
	if err := f.rig.Pool.QueryRow(context.Background(), `SELECT COALESCE(metadata,'') FROM sessions ORDER BY created_at DESC LIMIT 1`).Scan(&metadata); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(metadata, "203.0.113.7") {
		t.Fatalf("raw address stored: %s", metadata)
	}
	var m map[string]any
	_ = json.Unmarshal([]byte(metadata), &m)
	if ip, _ := m["ip_address"].(string); len(ip) != 64 {
		t.Fatalf("expected a 64-hex digest, got %q", ip)
	}
}
```

- [ ] **Step 7: Run, fix schema mismatches, and run again**

Run: `cd apps/server && mise exec -- go vet ./... && mise exec -- go test -p 1 -count=1 ./internal/auth/ -v 2>&1 | tail -60`

Expected: 8 tests PASS. If Limen reports a missing column or table (for example a `users` column it writes on sign-up, or an `organization_roles` lookup), do NOT edit `00001_init.sql`: add `apps/server/internal/db/migrations/00002_limen_align.sql` with the smallest `ALTER`/`CREATE` that satisfies it (and a matching `Down`), rerun `go generate ./...` if sqlc needs the new shape, and describe the mismatch in your report. If the sign-up route rejects the request with an origin/CSRF error, confirm the test sets `Origin: http://localhost:3000` (it does) and report what Limen demanded.

- [ ] **Step 8: Commit**

```bash
cd apps/server && mise exec -- go mod tidy && cd ../..
git add apps/server
git commit -m "feat(server): Limen-backed auth service with route allowlist, organizations and invitations"
```

---

### Task 6: Request middleware (session, organization, team, proxy, rate limit)

**Files:**
- Create: `apps/server/internal/api/middleware/middleware.go`, `apps/server/internal/api/middleware/middleware_test.go`

**Interfaces:**
- Consumes: `auth.Service.SessionFromRequest`, `gen.Queries` (`GetMemberRoles`, `GetTeam`, `IsTeamMember`), `authz`, `clientip`, `ratelimit.Store`, `respond`.
- Produces:
  ```go
  package middleware
  type Deps struct { Auth auth.Service; Q *gen.Queries; RateLimit ratelimit.Store; Hasher *clientip.Hasher; TrustedProxyHops int }
  type OrgCtx struct { OrgID, UserID, Role string }
  type TeamCtx struct { TeamID, OrgID, UserID, Role string; IsTeamMember bool }
  func TrustedProxy(hops int) func(http.Handler) http.Handler       // rewrites RemoteAddr; outermost
  func Session(d Deps) func(http.Handler) http.Handler              // resolves; never rejects
  func RequireSession() func(http.Handler) http.Handler             // 401 UNAUTHENTICATED
  func RequireOrg(d Deps) func(http.Handler) http.Handler           // path {orgId}; 403 FORBIDDEN unless member
  func RequireOrgAdmin() func(http.Handler) http.Handler            // 403 unless owner/admin (after RequireOrg)
  func RequireTeam(d Deps) func(http.Handler) http.Handler          // path {teamId}; 404 unknown; 403 unless authz.CanAccessTeam
  func RequireTeamAdmin() func(http.Handler) http.Handler           // 403 unless authz.CanManageTeams (after RequireTeam)
  func CaptureHTTP() func(http.Handler) http.Handler
  func RateLimit(d Deps, name string, limit int, window time.Duration) func(http.Handler) http.Handler   // 429 RATE_LIMITED + Retry-After
  func SessionFromContext(ctx) *auth.Session; func OrgFromContext(ctx) (OrgCtx, bool); func TeamFromContext(ctx) (TeamCtx, bool); func HTTPFromContext(ctx) (http.ResponseWriter, *http.Request, bool)
  ```

- [ ] **Step 1: Write the failing tests**

`middleware_test.go` uses a real auth service through testrig (Task 8 adds `AppRig`; this test builds its own small fixture so the package stays independent):

```go
package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/refsdal/snarvei/server/internal/api/middleware"
	"github.com/refsdal/snarvei/server/internal/auth"
	"github.com/refsdal/snarvei/server/internal/clientip"
	"github.com/refsdal/snarvei/server/internal/db/gen"
	"github.com/refsdal/snarvei/server/internal/email"
	"github.com/refsdal/snarvei/server/internal/ratelimit"
	"github.com/refsdal/snarvei/server/internal/testrig"
)

const secret = "snarvei-middleware-test-secret-0123456789"
const password = "Testpass123"

type fx struct {
	t    *testing.T
	d    middleware.Deps
	rig  *testrig.Rig
	auth http.Handler
}

func setup(t *testing.T) *fx {
	t.Helper()
	rig := testrig.Setup(t)
	hasher := clientip.NewHasher("", secret)
	svc, err := auth.New(auth.Config{AppURL: "http://localhost:3000", Secret: secret, OpenSignup: true, Pool: rig.Pool, ClientIP: hasher.Extractor(0), Email: email.NewRecording()})
	if err != nil {
		t.Fatal(err)
	}
	q := gen.New(rig.Pool)
	return &fx{t: t, rig: rig, auth: svc.Handler(), d: middleware.Deps{Auth: svc, Q: q, RateLimit: ratelimit.NewPostgres(q), Hasher: hasher, TrustedProxyHops: 0}}
}

func (f *fx) user(name, addr string) (string, string) {
	f.t.Helper()
	id, err := f.d.Auth.CreateUser(context.Background(), name, addr, password)
	if err != nil {
		f.t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, auth.BasePath+"/signin/credential", strings.NewReader(`{"credential":"`+addr+`","password":"`+password+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://localhost:3000")
	rec := httptest.NewRecorder()
	f.auth.ServeHTTP(rec, req)
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.SessionCookieName {
			return id, c.Name + "=" + c.Value
		}
	}
	f.t.Fatalf("no cookie: %d %s", rec.Code, rec.Body.String())
	return "", ""
}

func get(h http.Handler, pattern, path, cookie string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	mux.Handle(pattern, h)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func ok() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
}

func TestSessionAndRequireSession(t *testing.T) {
	f := setup(t)
	_, cookie := f.user("Kari", "kari@example.com")
	var seen *auth.Session
	h := middleware.Session(f.d)(middleware.RequireSession()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = middleware.SessionFromContext(r.Context())
	})))
	if rec := get(h, "GET /x", "/x", ""); rec.Code != 401 || !strings.Contains(rec.Body.String(), "UNAUTHENTICATED") {
		t.Fatalf("anonymous: %d %s", rec.Code, rec.Body.String())
	}
	if rec := get(h, "GET /x", "/x", cookie); rec.Code != 200 || seen == nil || seen.Name != "Kari" {
		t.Fatalf("signed in: %d %+v", rec.Code, seen)
	}
}

func TestRequireOrgAndAdmin(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	ownerID, ownerCookie := f.user("Owner", "owner@example.com")
	memberID, memberCookie := f.user("Member", "member@example.com")
	_, strangerCookie := f.user("Stranger", "stranger@example.com")
	org, err := f.d.Auth.CreateOrganization(ctx, ownerID, "Acme", "acme")
	if err != nil {
		t.Fatal(err)
	}
	inv, err := f.d.Auth.CreateInvitation(ctx, ownerID, org.ID, "member@example.com", "member")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.d.Auth.AcceptInvitation(ctx, memberID, inv.ID); err != nil {
		t.Fatal(err)
	}

	var seen middleware.OrgCtx
	member := middleware.Session(f.d)(middleware.RequireOrg(f.d)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen, _ = middleware.OrgFromContext(r.Context())
	})))
	admin := middleware.Session(f.d)(middleware.RequireOrg(f.d)(middleware.RequireOrgAdmin()(ok())))
	pattern := "GET /api/organizations/{orgId}/teams"
	path := "/api/organizations/" + org.ID + "/teams"

	if rec := get(member, pattern, path, ""); rec.Code != 401 {
		t.Fatalf("anonymous: %d", rec.Code)
	}
	if rec := get(member, pattern, path, strangerCookie); rec.Code != 403 || !strings.Contains(rec.Body.String(), "FORBIDDEN") {
		t.Fatalf("stranger: %d %s", rec.Code, rec.Body.String())
	}
	if rec := get(member, pattern, "/api/organizations/nope/teams", ownerCookie); rec.Code != 403 {
		t.Fatalf("unknown org must look like forbidden: %d", rec.Code)
	}
	if rec := get(member, pattern, path, memberCookie); rec.Code != 200 || seen.Role != "member" || seen.OrgID != org.ID || seen.UserID != memberID {
		t.Fatalf("member: %d %+v", rec.Code, seen)
	}
	if rec := get(member, pattern, path, ownerCookie); rec.Code != 200 || seen.Role != "owner" {
		t.Fatalf("owner: %d %+v", rec.Code, seen)
	}
	if rec := get(admin, pattern, path, memberCookie); rec.Code != 403 {
		t.Fatalf("member on admin route: %d", rec.Code)
	}
	if rec := get(admin, pattern, path, ownerCookie); rec.Code != 200 {
		t.Fatalf("owner on admin route: %d", rec.Code)
	}
}

func TestRequireTeam(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	ownerID, ownerCookie := f.user("Owner", "owner@example.com")
	memberID, memberCookie := f.user("Member", "member@example.com")
	outsiderID, outsiderCookie := f.user("Outsider", "outsider@example.com")
	org, _ := f.d.Auth.CreateOrganization(ctx, ownerID, "Acme", "acme")
	for _, u := range []struct{ id, mail string }{{memberID, "member@example.com"}, {outsiderID, "outsider@example.com"}} {
		inv, err := f.d.Auth.CreateInvitation(ctx, ownerID, org.ID, u.mail, "member")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.d.Auth.AcceptInvitation(ctx, u.id, inv.ID); err != nil {
			t.Fatal(err)
		}
	}
	team, err := f.d.Q.CreateTeam(ctx, gen.CreateTeamParams{ID: auth.NewID(), OrganizationID: org.ID, Name: "Marketing"})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.d.Q.AddTeamMember(ctx, gen.AddTeamMemberParams{TeamID: team.ID, UserID: memberID}); err != nil {
		t.Fatal(err)
	}

	var seen middleware.TeamCtx
	h := middleware.Session(f.d)(middleware.RequireTeam(f.d)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen, _ = middleware.TeamFromContext(r.Context())
	})))
	adminH := middleware.Session(f.d)(middleware.RequireTeam(f.d)(middleware.RequireTeamAdmin()(ok())))
	pattern := "GET /api/teams/{teamId}/members"
	path := "/api/teams/" + team.ID + "/members"

	if rec := get(h, pattern, "/api/teams/nope/members", ownerCookie); rec.Code != 404 {
		t.Fatalf("unknown team: %d", rec.Code)
	}
	if rec := get(h, pattern, path, outsiderCookie); rec.Code != 403 {
		t.Fatalf("org member outside the team: %d", rec.Code)
	}
	if rec := get(h, pattern, path, memberCookie); rec.Code != 200 || !seen.IsTeamMember || seen.Role != "member" || seen.OrgID != org.ID {
		t.Fatalf("team member: %d %+v", rec.Code, seen)
	}
	if rec := get(h, pattern, path, ownerCookie); rec.Code != 200 || seen.IsTeamMember || seen.Role != "owner" {
		t.Fatalf("owner outside the team: %d %+v", rec.Code, seen)
	}
	if rec := get(adminH, pattern, path, memberCookie); rec.Code != 403 {
		t.Fatalf("member on team admin route: %d", rec.Code)
	}
	if rec := get(adminH, pattern, path, ownerCookie); rec.Code != 200 {
		t.Fatalf("owner on team admin route: %d", rec.Code)
	}
}

func TestTrustedProxyAndRateLimit(t *testing.T) {
	f := setup(t)
	var remote string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { remote = r.RemoteAddr })
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.9")
	middleware.TrustedProxy(0)(inner).ServeHTTP(httptest.NewRecorder(), req)
	if !strings.HasPrefix(remote, "10.0.0.1") {
		t.Fatalf("hops=0 rewrote RemoteAddr to %q", remote)
	}
	middleware.TrustedProxy(1)(inner).ServeHTTP(httptest.NewRecorder(), req)
	if !strings.HasPrefix(remote, "203.0.113.9") {
		t.Fatalf("hops=1 did not rewrite RemoteAddr: %q", remote)
	}

	limited := middleware.RateLimit(f.d, "test", 2, time.Minute)(ok())
	for i := 1; i <= 3; i++ {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/x", nil)
		r.RemoteAddr = "198.51.100.5:1"
		limited.ServeHTTP(rec, r)
		if i <= 2 && rec.Code != 200 {
			t.Fatalf("hit %d: %d", i, rec.Code)
		}
		if i == 3 && (rec.Code != 429 || rec.Header().Get("Retry-After") == "" || !strings.Contains(rec.Body.String(), "RATE_LIMITED")) {
			t.Fatalf("hit 3: %d %q %s", rec.Code, rec.Header().Get("Retry-After"), rec.Body.String())
		}
	}
	rec := httptest.NewRecorder()
	other := httptest.NewRequest(http.MethodGet, "/x", nil)
	other.RemoteAddr = "198.51.100.6:1"
	limited.ServeHTTP(rec, other)
	if rec.Code != 200 {
		t.Fatalf("another address must have its own bucket: %d", rec.Code)
	}
}
```

- [ ] **Step 2: Run to verify failure, then write `middleware.go`**

```go
// Package middleware puts identity and tenancy on the request context and
// rejects what the rules in internal/authz forbid. Nothing here imports
// internal/api; the error envelope comes from internal/api/respond.
package middleware

import (
	"context"
	"errors"
	"math"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/refsdal/snarvei/server/internal/api/respond"
	"github.com/refsdal/snarvei/server/internal/auth"
	"github.com/refsdal/snarvei/server/internal/authz"
	"github.com/refsdal/snarvei/server/internal/clientip"
	"github.com/refsdal/snarvei/server/internal/db/gen"
	"github.com/refsdal/snarvei/server/internal/ratelimit"
)

// Deps is what the chain needs from the composition root.
type Deps struct {
	Auth             auth.Service
	Q                *gen.Queries
	RateLimit        ratelimit.Store
	Hasher           *clientip.Hasher
	TrustedProxyHops int
}

// OrgCtx is the organization named in the path plus the caller's role in it.
type OrgCtx struct{ OrgID, UserID, Role string }

// TeamCtx is the team named in the path plus the caller's standing.
type TeamCtx struct {
	TeamID, OrgID, UserID, Role string
	IsTeamMember                bool
}

type ctxKey int

const (
	sessionKey ctxKey = iota
	orgKey
	teamKey
	httpKey
)

type httpPair struct {
	w http.ResponseWriter
	r *http.Request
}

// SessionFromContext returns the resolved session or nil.
func SessionFromContext(ctx context.Context) *auth.Session {
	s, _ := ctx.Value(sessionKey).(*auth.Session)
	return s
}

// OrgFromContext returns what RequireOrg resolved.
func OrgFromContext(ctx context.Context) (OrgCtx, bool) {
	o, ok := ctx.Value(orgKey).(OrgCtx)
	return o, ok
}

// TeamFromContext returns what RequireTeam resolved.
func TeamFromContext(ctx context.Context) (TeamCtx, bool) {
	t, ok := ctx.Value(teamKey).(TeamCtx)
	return t, ok
}

// HTTPFromContext returns the raw writer and request captured by CaptureHTTP.
func HTTPFromContext(ctx context.Context) (http.ResponseWriter, *http.Request, bool) {
	p, ok := ctx.Value(httpKey).(httpPair)
	return p.w, p.r, ok
}

// TrustedProxy rewrites RemoteAddr to the trusted client address so Limen's
// limiter, the session digest and every handler agree on who is calling.
func TrustedProxy(hops int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if hops <= 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			address := clientip.FromRequest(r, hops)
			if address == "unknown" {
				next.ServeHTTP(w, r)
				return
			}
			if _, port, err := net.SplitHostPort(r.RemoteAddr); err == nil {
				address = net.JoinHostPort(address, port)
			}
			rewritten := *r
			rewritten.RemoteAddr = address
			next.ServeHTTP(w, &rewritten)
		})
	}
}

// Session resolves the caller (never rejects) and refreshes the cookie.
func Session(d Deps) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s, err := d.Auth.SessionFromRequest(w, r)
			if err != nil {
				respond.Error(w, http.StatusInternalServerError, "INTERNAL", "session lookup failed")
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionKey, s)))
		})
	}
}

// RequireSession rejects anonymous callers.
func RequireSession() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if SessionFromContext(r.Context()) == nil {
				respond.Error(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Not signed in")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireOrg resolves {orgId} and the caller's role; non-members (and
// unknown organizations) get 403 so existence is never revealed.
func RequireOrg(d Deps) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s := SessionFromContext(r.Context())
			if s == nil {
				respond.Error(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Not signed in")
				return
			}
			orgID := r.PathValue("orgId")
			role, err := memberRole(r.Context(), d.Q, orgID, s.UserID)
			if err != nil {
				respond.Error(w, http.StatusInternalServerError, "INTERNAL", "membership lookup failed")
				return
			}
			if role == "" {
				respond.Error(w, http.StatusForbidden, "FORBIDDEN", "Organization access denied")
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), orgKey, OrgCtx{OrgID: orgID, UserID: s.UserID, Role: role})))
		})
	}
}

// RequireOrgAdmin runs after RequireOrg.
func RequireOrgAdmin() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			o, ok := OrgFromContext(r.Context())
			if !ok || !authz.IsOrgAdmin(o.Role) {
				respond.Error(w, http.StatusForbidden, "FORBIDDEN", "Organization admin required")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireTeam resolves {teamId}, the team's organization, the caller's org
// role and team membership, and applies authz.CanAccessTeam.
func RequireTeam(d Deps) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s := SessionFromContext(r.Context())
			if s == nil {
				respond.Error(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Not signed in")
				return
			}
			teamID := r.PathValue("teamId")
			team, err := d.Q.GetTeam(r.Context(), teamID)
			if errors.Is(err, pgx.ErrNoRows) {
				respond.Error(w, http.StatusNotFound, "NOT_FOUND", "Team not found")
				return
			}
			if err != nil {
				respond.Error(w, http.StatusInternalServerError, "INTERNAL", "team lookup failed")
				return
			}
			role, err := memberRole(r.Context(), d.Q, team.OrganizationID, s.UserID)
			if err != nil {
				respond.Error(w, http.StatusInternalServerError, "INTERNAL", "membership lookup failed")
				return
			}
			n, err := d.Q.IsTeamMember(r.Context(), gen.IsTeamMemberParams{TeamID: teamID, UserID: s.UserID})
			if err != nil {
				respond.Error(w, http.StatusInternalServerError, "INTERNAL", "team membership lookup failed")
				return
			}
			tc := TeamCtx{TeamID: teamID, OrgID: team.OrganizationID, UserID: s.UserID, Role: role, IsTeamMember: n > 0}
			if !authz.CanAccessTeam(tc.Role, tc.IsTeamMember) {
				respond.Error(w, http.StatusForbidden, "FORBIDDEN", "Team access denied")
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), teamKey, tc)))
		})
	}
}

// RequireTeamAdmin runs after RequireTeam.
func RequireTeamAdmin() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tc, ok := TeamFromContext(r.Context())
			if !ok || !authz.CanManageTeams(tc.Role) {
				respond.Error(w, http.StatusForbidden, "FORBIDDEN", "Organization admin required")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CaptureHTTP makes the raw writer and request reachable from a strict
// handler's context (needed to set a session cookie).
func CaptureHTTP() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), httpKey, httpPair{w: w, r: r})))
		})
	}
}

// RateLimit allows limit requests per window per hashed client address.
func RateLimit(d Deps, name string, limit int, window time.Duration) func(http.Handler) http.Handler {
	if d.RateLimit == nil || d.Hasher == nil {
		panic("middleware: RateLimit needs a store and a hasher")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			bucket := d.Hasher.Hash(clientip.FromRequest(r, d.TrustedProxyHops))[:32]
			key, _ := ratelimit.Key(name, bucket, time.Now(), window)
			count, retry, err := d.RateLimit.Hit(r.Context(), key, window)
			if err != nil {
				respond.Error(w, http.StatusInternalServerError, "INTERNAL", "rate limit check failed")
				return
			}
			if count > limit {
				w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(retry.Seconds()))))
				respond.Error(w, http.StatusTooManyRequests, "RATE_LIMITED", "Too many requests, try again later")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// memberRole returns the caller's highest role in orgID, "" when not a member.
func memberRole(ctx context.Context, q *gen.Queries, orgID, userID string) (string, error) {
	roles, err := q.GetMemberRoles(ctx, gen.GetMemberRolesParams{OrganizationID: orgID, UserID: userID})
	if err != nil {
		return "", err
	}
	return authz.Highest(roles), nil
}
```

Run: `cd apps/server && mise exec -- go vet ./... && mise exec -- go test -p 1 -count=1 ./internal/api/middleware/ -v`
Expected: 4 PASS. (`GetMemberRoles` returns `[]string` because the query selects one column; if sqlc emitted `[]interface{}` or a row struct, adjust `memberRole` to collect the strings.)

- [ ] **Step 3: Commit**

```bash
git add apps/server/internal/api/middleware
git commit -m "feat(server): session, organization, team, proxy and rate-limit middleware"
```

---

### Task 7: OpenAPI spec for the phase-2 endpoints, codegen, auth tiers and mounting

**Files:**
- Modify: `openapi/snarvei.yaml`, `apps/server/internal/api/api.go`
- Create: `apps/server/internal/api/tiers.go`, `apps/server/internal/api/errors.go`, `apps/server/internal/api/stubs.go` (temporary: every new operation returns 501 until Tasks 9 and 10 replace it)
- Regenerate: `apps/server/internal/api/gen/*.gen.go`, `apps/server/internal/api/snarvei.yaml`

**Interfaces:**
- Produces: `api.Deps` gains `Q *gen.Queries; Auth auth.Service; Storage storage.Storage; Email email.Sender; RateLimit ratelimit.Store; Hasher *clientip.Hasher; TrustedProxyHops int; AppURL string; TestHooks bool; Mail *email.Recording` (Mail non-nil only when TestHooks). `NewHandler` mounts Limen at `/api/auth/`, wraps everything in `TrustedProxy`, applies per-operation middleware chains from `operationTiers`, and panics at build time if an operation has no tier. `apiError(w-less) func(err error) (status int, code, message string)` in `errors.go` maps `auth` sentinels and `pgx.ErrNoRows` for handlers.

- [ ] **Step 1: Extend `openapi/snarvei.yaml`**

Append these paths under `paths:` (keep the three existing ones) and these schemas under `components.schemas` (keep `PublicConfig` and `Error`). Every error response references `#/components/schemas/Error`.

```yaml
  /api/me:
    get:
      operationId: getMe
      summary: The signed-in user and a minimal view of the current session (never the token).
      tags: [me]
      responses:
        "200": { description: Me., content: { application/json: { schema: { $ref: "#/components/schemas/Me" } } } }
        "401": { $ref: "#/components/responses/Unauthenticated" }
    patch:
      operationId: updateMe
      summary: Update the display name.
      tags: [me]
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [name]
              properties:
                name: { type: string, minLength: 1, maxLength: 120 }
      responses:
        "200": { description: Updated., content: { application/json: { schema: { $ref: "#/components/schemas/Me" } } } }
        "400": { $ref: "#/components/responses/ValidationFailed" }
        "401": { $ref: "#/components/responses/Unauthenticated" }
    delete:
      operationId: deleteMe
      summary: Delete the account. Refused with LAST_OWNER when the user is the sole owner of an organization.
      tags: [me]
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [password]
              properties:
                password: { type: string, minLength: 1 }
      responses:
        "204": { description: Deleted; every session is revoked. }
        "401": { $ref: "#/components/responses/Unauthenticated" }
        "409": { description: LAST_OWNER., content: { application/json: { schema: { $ref: "#/components/schemas/Error" } } } }
  /api/me/email:
    post:
      operationId: requestEmailChange
      summary: Send a confirmation link to a new address. Rate limited.
      tags: [me]
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [newEmail, password]
              properties:
                newEmail: { type: string, format: email }
                password: { type: string, minLength: 1 }
      responses:
        "202": { description: Confirmation mail sent (or dropped when mail is off). }
        "400": { $ref: "#/components/responses/ValidationFailed" }
        "401": { $ref: "#/components/responses/Unauthenticated" }
        "409": { description: EMAIL_TAKEN., content: { application/json: { schema: { $ref: "#/components/schemas/Error" } } } }
        "429": { $ref: "#/components/responses/RateLimited" }
  /api/me/email/confirm:
    post:
      operationId: confirmEmailChange
      summary: Apply the address change carried by the token (single use, one hour).
      tags: [me]
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [token]
              properties:
                token: { type: string, minLength: 1 }
      responses:
        "200": { description: Changed., content: { application/json: { schema: { $ref: "#/components/schemas/Me" } } } }
        "400": { $ref: "#/components/responses/ValidationFailed" }
        "401": { $ref: "#/components/responses/Unauthenticated" }
        "409": { description: EMAIL_TAKEN., content: { application/json: { schema: { $ref: "#/components/schemas/Error" } } } }
  /api/me/sessions:
    get:
      operationId: listMySessions
      summary: Active sessions of the signed-in user, newest first. Never the token.
      tags: [me]
      responses:
        "200": { description: Sessions., content: { application/json: { schema: { type: array, items: { $ref: "#/components/schemas/SessionSummary" } } } } }
        "401": { $ref: "#/components/responses/Unauthenticated" }
    delete:
      operationId: revokeOtherSessions
      summary: Revoke every session except the current one.
      tags: [me]
      responses:
        "204": { description: Revoked. }
        "401": { $ref: "#/components/responses/Unauthenticated" }
  /api/me/sessions/{sessionId}:
    delete:
      operationId: revokeMySession
      summary: Revoke one of the user's own sessions.
      tags: [me]
      parameters: [{ name: sessionId, in: path, required: true, schema: { type: string } }]
      responses:
        "204": { description: Revoked. }
        "401": { $ref: "#/components/responses/Unauthenticated" }
        "404": { $ref: "#/components/responses/NotFound" }
  /api/organizations:
    get:
      operationId: listOrganizations
      summary: Organizations the caller belongs to, with their role.
      tags: [organizations]
      responses:
        "200": { description: Organizations., content: { application/json: { schema: { type: array, items: { $ref: "#/components/schemas/Organization" } } } } }
        "401": { $ref: "#/components/responses/Unauthenticated" }
    post:
      operationId: createOrganization
      summary: Create an organization; the caller becomes its owner.
      tags: [organizations]
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [name, slug]
              properties:
                name: { type: string, minLength: 1, maxLength: 120 }
                slug: { type: string, pattern: "^[a-z0-9]+(?:-[a-z0-9]+)*$", minLength: 2, maxLength: 64 }
      responses:
        "201": { description: Created., content: { application/json: { schema: { $ref: "#/components/schemas/Organization" } } } }
        "400": { $ref: "#/components/responses/ValidationFailed" }
        "401": { $ref: "#/components/responses/Unauthenticated" }
        "409": { description: SLUG_TAKEN., content: { application/json: { schema: { $ref: "#/components/schemas/Error" } } } }
  /api/organizations/{orgId}/switch:
    post:
      operationId: switchOrganization
      summary: Make this organization the session's active one.
      tags: [organizations]
      parameters: [{ $ref: "#/components/parameters/OrgId" }]
      responses:
        "204": { description: Switched. }
        "401": { $ref: "#/components/responses/Unauthenticated" }
        "403": { $ref: "#/components/responses/Forbidden" }
  /api/organizations/{orgId}/members:
    get:
      operationId: listOrganizationMembers
      summary: Members with their roles.
      tags: [organizations]
      parameters: [{ $ref: "#/components/parameters/OrgId" }]
      responses:
        "200": { description: Members., content: { application/json: { schema: { type: array, items: { $ref: "#/components/schemas/Member" } } } } }
        "401": { $ref: "#/components/responses/Unauthenticated" }
        "403": { $ref: "#/components/responses/Forbidden" }
  /api/organizations/{orgId}/invitations:
    get:
      operationId: listInvitations
      summary: Pending invitations (owner/admin).
      tags: [invitations]
      parameters: [{ $ref: "#/components/parameters/OrgId" }]
      responses:
        "200": { description: Invitations., content: { application/json: { schema: { type: array, items: { $ref: "#/components/schemas/Invitation" } } } } }
        "401": { $ref: "#/components/responses/Unauthenticated" }
        "403": { $ref: "#/components/responses/Forbidden" }
    post:
      operationId: createInvitation
      summary: Invite an email address, optionally straight into a team (owner/admin).
      tags: [invitations]
      parameters: [{ $ref: "#/components/parameters/OrgId" }]
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [email, role]
              properties:
                email: { type: string, format: email }
                role: { type: string, enum: [admin, member] }
                teamId: { type: string }
      responses:
        "201": { description: Created; the invitee is emailed a link., content: { application/json: { schema: { $ref: "#/components/schemas/Invitation" } } } }
        "400": { $ref: "#/components/responses/ValidationFailed" }
        "401": { $ref: "#/components/responses/Unauthenticated" }
        "403": { $ref: "#/components/responses/Forbidden" }
        "404": { $ref: "#/components/responses/NotFound" }
        "409": { description: ALREADY_MEMBER or INVITATION_EXISTS., content: { application/json: { schema: { $ref: "#/components/schemas/Error" } } } }
  /api/organizations/{orgId}/invitations/{invitationId}:
    delete:
      operationId: cancelInvitation
      summary: Cancel a pending invitation (owner/admin).
      tags: [invitations]
      parameters:
        - { $ref: "#/components/parameters/OrgId" }
        - { name: invitationId, in: path, required: true, schema: { type: string } }
      responses:
        "204": { description: Cancelled. }
        "401": { $ref: "#/components/responses/Unauthenticated" }
        "403": { $ref: "#/components/responses/Forbidden" }
        "404": { $ref: "#/components/responses/NotFound" }
  /api/invitations/{invitationId}:
    get:
      operationId: getInvitation
      summary: Public view of an invitation for the accept page. Never the email.
      tags: [invitations]
      parameters: [{ $ref: "#/components/parameters/InvitationId" }]
      responses:
        "200": { description: Invitation., content: { application/json: { schema: { $ref: "#/components/schemas/PublicInvitation" } } } }
        "404": { $ref: "#/components/responses/NotFound" }
  /api/invitations/{invitationId}/accept:
    post:
      operationId: acceptInvitation
      summary: Accept; the session's email must match. Adds the team membership when the invitation carries one.
      tags: [invitations]
      parameters: [{ $ref: "#/components/parameters/InvitationId" }]
      responses:
        "200": { description: Joined., content: { application/json: { schema: { $ref: "#/components/schemas/Organization" } } } }
        "401": { $ref: "#/components/responses/Unauthenticated" }
        "403": { description: INVITATION_EMAIL_MISMATCH., content: { application/json: { schema: { $ref: "#/components/schemas/Error" } } } }
        "404": { $ref: "#/components/responses/NotFound" }
        "410": { description: INVITATION_INVALID., content: { application/json: { schema: { $ref: "#/components/schemas/Error" } } } }
  /api/invitations/{invitationId}/reject:
    post:
      operationId: rejectInvitation
      summary: Reject; the session's email must match.
      tags: [invitations]
      parameters: [{ $ref: "#/components/parameters/InvitationId" }]
      responses:
        "204": { description: Rejected. }
        "401": { $ref: "#/components/responses/Unauthenticated" }
        "403": { description: INVITATION_EMAIL_MISMATCH., content: { application/json: { schema: { $ref: "#/components/schemas/Error" } } } }
        "404": { $ref: "#/components/responses/NotFound" }
        "410": { description: INVITATION_INVALID., content: { application/json: { schema: { $ref: "#/components/schemas/Error" } } } }
  /api/invitations/{invitationId}/register:
    post:
      operationId: registerWithInvitation
      summary: Create the account for an invitee whose email has no account, accept, and start a session. Rate limited.
      tags: [invitations]
      parameters: [{ $ref: "#/components/parameters/InvitationId" }]
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [name, password]
              properties:
                name: { type: string, minLength: 1, maxLength: 120 }
                password: { type: string, minLength: 8 }
      responses:
        "201": { description: Registered and signed in (session cookie set)., content: { application/json: { schema: { $ref: "#/components/schemas/Me" } } } }
        "400": { $ref: "#/components/responses/ValidationFailed" }
        "404": { $ref: "#/components/responses/NotFound" }
        "409": { description: EMAIL_TAKEN (sign in and accept instead)., content: { application/json: { schema: { $ref: "#/components/schemas/Error" } } } }
        "410": { description: INVITATION_INVALID., content: { application/json: { schema: { $ref: "#/components/schemas/Error" } } } }
        "429": { $ref: "#/components/responses/RateLimited" }
  /api/organizations/{orgId}/teams:
    get:
      operationId: listTeams
      summary: Teams the caller can see (all for owner/admin, own teams for members).
      tags: [teams]
      parameters: [{ $ref: "#/components/parameters/OrgId" }]
      responses:
        "200": { description: Teams., content: { application/json: { schema: { type: array, items: { $ref: "#/components/schemas/Team" } } } } }
        "401": { $ref: "#/components/responses/Unauthenticated" }
        "403": { $ref: "#/components/responses/Forbidden" }
    post:
      operationId: createTeam
      summary: Create a team (owner/admin).
      tags: [teams]
      parameters: [{ $ref: "#/components/parameters/OrgId" }]
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [name]
              properties:
                name: { type: string, minLength: 1, maxLength: 120 }
      responses:
        "201": { description: Created., content: { application/json: { schema: { $ref: "#/components/schemas/Team" } } } }
        "400": { $ref: "#/components/responses/ValidationFailed" }
        "401": { $ref: "#/components/responses/Unauthenticated" }
        "403": { $ref: "#/components/responses/Forbidden" }
        "409": { description: TEAM_EXISTS., content: { application/json: { schema: { $ref: "#/components/schemas/Error" } } } }
  /api/teams/{teamId}/members:
    get:
      operationId: listTeamMembers
      summary: Members of a team.
      tags: [teams]
      parameters: [{ $ref: "#/components/parameters/TeamId" }]
      responses:
        "200": { description: Members., content: { application/json: { schema: { type: array, items: { $ref: "#/components/schemas/TeamMember" } } } } }
        "401": { $ref: "#/components/responses/Unauthenticated" }
        "403": { $ref: "#/components/responses/Forbidden" }
        "404": { $ref: "#/components/responses/NotFound" }
    post:
      operationId: addTeamMember
      summary: Add an organization member to the team (owner/admin).
      tags: [teams]
      parameters: [{ $ref: "#/components/parameters/TeamId" }]
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [userId]
              properties:
                userId: { type: string }
      responses:
        "204": { description: Added (idempotent). }
        "400": { $ref: "#/components/responses/ValidationFailed" }
        "401": { $ref: "#/components/responses/Unauthenticated" }
        "403": { $ref: "#/components/responses/Forbidden" }
        "404": { description: Team or user not found in the organization., content: { application/json: { schema: { $ref: "#/components/schemas/Error" } } } }
  /api/teams/{teamId}/members/{userId}:
    delete:
      operationId: removeTeamMember
      summary: Remove a member from the team (owner/admin).
      tags: [teams]
      parameters:
        - { $ref: "#/components/parameters/TeamId" }
        - { name: userId, in: path, required: true, schema: { type: string } }
      responses:
        "204": { description: Removed. }
        "401": { $ref: "#/components/responses/Unauthenticated" }
        "403": { $ref: "#/components/responses/Forbidden" }
        "404": { $ref: "#/components/responses/NotFound" }
```

Under `components:` add:

```yaml
  parameters:
    OrgId: { name: orgId, in: path, required: true, schema: { type: string } }
    TeamId: { name: teamId, in: path, required: true, schema: { type: string } }
    InvitationId: { name: invitationId, in: path, required: true, schema: { type: string } }
  responses:
    Unauthenticated: { description: UNAUTHENTICATED., content: { application/json: { schema: { $ref: "#/components/schemas/Error" } } } }
    Forbidden: { description: FORBIDDEN., content: { application/json: { schema: { $ref: "#/components/schemas/Error" } } } }
    NotFound: { description: NOT_FOUND., content: { application/json: { schema: { $ref: "#/components/schemas/Error" } } } }
    ValidationFailed: { description: VALIDATION_FAILED., content: { application/json: { schema: { $ref: "#/components/schemas/Error" } } } }
    RateLimited: { description: RATE_LIMITED., content: { application/json: { schema: { $ref: "#/components/schemas/Error" } } } }
```

And these schemas:

```yaml
    User:
      type: object
      required: [id, name, email, image, twoFactorEnabled]
      properties:
        id: { type: string }
        name: { type: string }
        email: { type: string }
        image: { type: string, nullable: true }
        twoFactorEnabled: { type: boolean }
    SessionInfo:
      type: object
      required: [id, expiresAt, activeOrganizationId]
      properties:
        id: { type: string }
        expiresAt: { type: string, format: date-time }
        activeOrganizationId: { type: string, nullable: true }
    Me:
      type: object
      required: [user, session]
      properties:
        user: { $ref: "#/components/schemas/User" }
        session: { $ref: "#/components/schemas/SessionInfo" }
    SessionSummary:
      type: object
      required: [id, createdAt, lastAccess, expiresAt, userAgent, current]
      properties:
        id: { type: string }
        createdAt: { type: string, format: date-time }
        lastAccess: { type: string, format: date-time, nullable: true }
        expiresAt: { type: string, format: date-time }
        userAgent: { type: string, nullable: true }
        current: { type: boolean }
    Organization:
      type: object
      required: [id, name, slug, role]
      properties:
        id: { type: string }
        name: { type: string }
        slug: { type: string }
        role: { type: string, enum: [owner, admin, member] }
    Member:
      type: object
      required: [id, userId, name, email, role, createdAt]
      properties:
        id: { type: string }
        userId: { type: string }
        name: { type: string }
        email: { type: string }
        role: { type: string }
        createdAt: { type: string, format: date-time }
    Invitation:
      type: object
      required: [id, email, role, status, expiresAt, teamId, teamName, createdAt]
      properties:
        id: { type: string }
        email: { type: string }
        role: { type: string }
        status: { type: string }
        expiresAt: { type: string, format: date-time, nullable: true }
        teamId: { type: string, nullable: true }
        teamName: { type: string, nullable: true }
        createdAt: { type: string, format: date-time }
    PublicInvitation:
      type: object
      required: [id, organizationName, inviterName, role, status, teamName, expiresAt, hasAccount]
      properties:
        id: { type: string }
        organizationName: { type: string }
        inviterName: { type: string, nullable: true }
        role: { type: string }
        status: { type: string }
        teamName: { type: string, nullable: true }
        expiresAt: { type: string, format: date-time, nullable: true }
        hasAccount: { type: boolean }
    Team:
      type: object
      required: [id, organizationId, name, memberCount, createdAt]
      properties:
        id: { type: string }
        organizationId: { type: string }
        name: { type: string }
        memberCount: { type: integer }
        createdAt: { type: string, format: date-time }
    TeamMember:
      type: object
      required: [userId, name, email, createdAt]
      properties:
        userId: { type: string }
        name: { type: string }
        email: { type: string }
        createdAt: { type: string, format: date-time }
```

Run: `cd apps/server && mise exec -- go generate ./... && grep -c "RequestObject" internal/api/gen/server.gen.go`
Expected: generation succeeds; the strict interface now has 27 methods (3 existing + 24 new).

- [ ] **Step 2: Write `apps/server/internal/api/errors.go`**

```go
package api

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/refsdal/snarvei/server/internal/api/gen"
	"github.com/refsdal/snarvei/server/internal/auth"
	"github.com/refsdal/snarvei/server/internal/db"
)

// httpError is what handlers return to the strict-server machinery for
// expected failures; responseErrorHandler renders it as the envelope.
type httpError struct {
	status  int
	code    string
	message string
}

func (e *httpError) Error() string { return e.code + ": " + e.message }

func fail(status int, code, message string) error { return &httpError{status, code, message} }

// classify maps the auth package's sentinels and common database errors to
// envelopes; anything else stays an internal error (logged, masked).
func classify(err error) error {
	var he *httpError
	if errors.As(err, &he) {
		return err
	}
	var policy *auth.PasswordPolicyError
	switch {
	case errors.As(err, &policy):
		return fail(http.StatusBadRequest, "VALIDATION_FAILED", "Password "+policy.Requirement)
	case errors.Is(err, auth.ErrInvalidPassword):
		return fail(http.StatusUnauthorized, "INVALID_PASSWORD", "Password is incorrect")
	case errors.Is(err, auth.ErrEmailTaken):
		return fail(http.StatusConflict, "EMAIL_TAKEN", "An account already exists for that email")
	case errors.Is(err, auth.ErrSlugTaken):
		return fail(http.StatusConflict, "SLUG_TAKEN", "That slug is already in use")
	case errors.Is(err, auth.ErrNotMember), errors.Is(err, auth.ErrForbidden):
		return fail(http.StatusForbidden, "FORBIDDEN", "Access denied")
	case errors.Is(err, auth.ErrAlreadyMember):
		return fail(http.StatusConflict, "ALREADY_MEMBER", "Already a member of this organization")
	case errors.Is(err, auth.ErrInvitationExists):
		return fail(http.StatusConflict, "INVITATION_EXISTS", "A pending invitation already exists for that email")
	case errors.Is(err, auth.ErrInvitationEmailMismatch):
		return fail(http.StatusForbidden, "INVITATION_EMAIL_MISMATCH", "This invitation was sent to a different email address")
	case errors.Is(err, auth.ErrInvitationInvalid):
		return fail(http.StatusGone, "INVITATION_INVALID", "This invitation is no longer valid")
	case errors.Is(err, auth.ErrUnknownRole):
		return fail(http.StatusBadRequest, "VALIDATION_FAILED", "Unknown role")
	case errors.Is(err, auth.ErrNotFound), errors.Is(err, auth.ErrSessionNotFound), errors.Is(err, pgx.ErrNoRows):
		return fail(http.StatusNotFound, "NOT_FOUND", "Not found")
	case db.IsUniqueViolation(err):
		return fail(http.StatusConflict, "CONFLICT", "Already exists")
	}
	return err
}

// errorResponse renders an httpError through gen's error type so strict
// handlers can return typed 4xx bodies where the spec declares them.
func envelope(e *httpError) gen.Error {
	return gen.Error{Code: e.code, Message: e.message}
}
```

- [ ] **Step 3: Write `tiers.go` and rewrite `api.go`**

`tiers.go`:

```go
package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/refsdal/snarvei/server/internal/api/gen"
	"github.com/refsdal/snarvei/server/internal/api/middleware"
)

type tier int

const (
	tierPublic tier = iota
	tierPublicCapture // public + CaptureHTTP (sets a cookie)
	tierSession
	tierSessionRateLimited
	tierOrg
	tierOrgAdmin
	tierTeam
	tierTeamAdmin
)

// operationTiers names the chain every spec operation runs behind. A spec
// operation missing here panics at NewHandler time (assertTierCoverage), so
// a new endpoint can never ship unguarded by accident.
var operationTiers = map[string]tier{
	"Healthz": tierPublic, "Readyz": tierPublic, "GetConfig": tierPublic,
	"GetMe": tierSession, "UpdateMe": tierSession, "DeleteMe": tierSession,
	"RequestEmailChange": tierSessionRateLimited, "ConfirmEmailChange": tierSession,
	"ListMySessions": tierSession, "RevokeOtherSessions": tierSession, "RevokeMySession": tierSession,
	"ListOrganizations": tierSession, "CreateOrganization": tierSession,
	"SwitchOrganization": tierOrg, "ListOrganizationMembers": tierOrg,
	"ListInvitations": tierOrgAdmin, "CreateInvitation": tierOrgAdmin, "CancelInvitation": tierOrgAdmin,
	"GetInvitation": tierPublic, "AcceptInvitation": tierSession, "RejectInvitation": tierSession,
	"RegisterWithInvitation": tierPublicCapture,
	"ListTeams": tierOrg, "CreateTeam": tierOrgAdmin,
	"ListTeamMembers": tierTeam, "AddTeamMember": tierTeamAdmin, "RemoveTeamMember": tierTeamAdmin,
}

const (
	writeLimit  = 30
	writeWindow = time.Minute
)

func (d Deps) mwDeps() middleware.Deps {
	return middleware.Deps{Auth: d.Auth, Q: d.Q, RateLimit: d.RateLimit, Hasher: d.Hasher, TrustedProxyHops: d.TrustedProxyHops}
}

// chain returns the http middleware for a tier.
func (d Deps) chain(t tier) func(http.Handler) http.Handler {
	md := d.mwDeps()
	session, require := middleware.Session(md), middleware.RequireSession()
	org, orgAdmin := middleware.RequireOrg(md), middleware.RequireOrgAdmin()
	team, teamAdmin := middleware.RequireTeam(md), middleware.RequireTeamAdmin()
	capture := middleware.CaptureHTTP()
	limited := middleware.RateLimit(md, "write", writeLimit, writeWindow)
	switch t {
	case tierPublic:
		return func(h http.Handler) http.Handler { return h }
	case tierPublicCapture:
		return func(h http.Handler) http.Handler { return limited(capture(h)) }
	case tierSession:
		return func(h http.Handler) http.Handler { return session(require(capture(h))) }
	case tierSessionRateLimited:
		return func(h http.Handler) http.Handler { return session(require(limited(h))) }
	case tierOrg:
		return func(h http.Handler) http.Handler { return session(org(h)) }
	case tierOrgAdmin:
		return func(h http.Handler) http.Handler { return session(org(orgAdmin(h))) }
	case tierTeam:
		return func(h http.Handler) http.Handler { return session(team(h)) }
	case tierTeamAdmin:
		return func(h http.Handler) http.Handler { return session(team(teamAdmin(h))) }
	}
	panic(fmt.Sprintf("api: unknown tier %d", t))
}

// tierMiddleware adapts the per-operation chain to the strict-server shape.
func (d Deps) tierMiddleware() gen.StrictMiddlewareFunc {
	chains := map[tier]func(http.Handler) http.Handler{}
	for t := tierPublic; t <= tierTeamAdmin; t++ {
		chains[t] = d.chain(t)
	}
	return func(f gen.StrictHandlerFunc, operationID string) gen.StrictHandlerFunc {
		t, ok := operationTiers[operationID]
		if !ok {
			panic("api: no tier for operation " + operationID)
		}
		mw := chains[t]
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request, request any) (any, error) {
			var resp any
			var err error
			terminal := http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
				resp, err = f(req.Context(), w, req, request)
			})
			mw(terminal).ServeHTTP(w, r)
			return resp, err
		}
	}
}

// assertTierCoverage panics when a spec operation has no tier.
func assertTierCoverage(spec *openapi3.T) {
	for _, item := range spec.Paths.Map() {
		for _, op := range item.Operations() {
			id := exportName(op.OperationID)
			if _, ok := operationTiers[id]; !ok {
				panic("api: operation " + op.OperationID + " has no entry in operationTiers")
			}
		}
	}
}

// exportName mirrors oapi-codegen's operationId → Go method name rule (first
// letter upper-cased) for the ids this spec uses.
func exportName(id string) string {
	if id == "" {
		return id
	}
	return string(id[0]-'a'+'A') + id[1:]
}
```

Rewrite `api.go` (keep `loadSpec`, `withSpecValidation`, `handleNotFound`, `requestErrorHandler`, `noStore`; change `Deps`, `responseErrorHandler` and `NewHandler`):

```go
// Package api owns every route in openapi/snarvei.yaml plus the hand-routed
// binary endpoints. It never reads the environment or constructs a
// dependency: cmd/snarvei hands it a Deps.
package api

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/jackc/pgx/v5/pgxpool"
	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"

	"github.com/refsdal/snarvei/server/internal/api/gen"
	"github.com/refsdal/snarvei/server/internal/api/middleware"
	"github.com/refsdal/snarvei/server/internal/api/respond"
	"github.com/refsdal/snarvei/server/internal/auth"
	"github.com/refsdal/snarvei/server/internal/clientip"
	dbgen "github.com/refsdal/snarvei/server/internal/db/gen"
	"github.com/refsdal/snarvei/server/internal/email"
	"github.com/refsdal/snarvei/server/internal/ratelimit"
	"github.com/refsdal/snarvei/server/internal/storage"
)

//go:embed snarvei.yaml
var specYAML []byte

// Deps is everything the handlers need.
type Deps struct {
	Pool    *pgxpool.Pool
	Q       *dbgen.Queries
	Auth    auth.Service
	Storage storage.Storage
	Email   email.Sender
	// Mail is set only when TestHooks is on: the same Recording the Email
	// field points at, exposed at GET /api/_test/mail for Playwright.
	Mail      *email.Recording
	RateLimit ratelimit.Store
	Hasher    *clientip.Hasher
	Log       *slog.Logger

	AppURL           string
	AppName          string
	OpenSignup       bool
	Version          string
	TrustedProxyHops int
	TestHooks        bool
}

func (d Deps) log() *slog.Logger {
	if d.Log == nil {
		return slog.Default()
	}
	return d.Log
}

// (loadSpec, withSpecValidation, handleNotFound, requestErrorHandler, noStore unchanged from phase 1)

func (d Deps) responseErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	err = classify(err)
	var he *httpError
	if errors.As(err, &he) {
		respond.Error(w, he.status, he.code, he.message)
		return
	}
	d.log().Error("unhandled error", "event", "request.error", "method", r.Method, "path", r.URL.Path, "error", err.Error())
	respond.Error(w, http.StatusInternalServerError, "INTERNAL", "Internal error")
}

// NewHandler builds the handler for every server-owned path.
func NewHandler(d Deps) http.Handler {
	spec := loadSpec()
	assertTierCoverage(spec)
	if d.Auth == nil || d.Q == nil || d.RateLimit == nil || d.Hasher == nil {
		panic("api: NewHandler needs Auth, Q, RateLimit and Hasher")
	}

	mux := http.NewServeMux()
	mux.Handle(auth.BasePath+"/", d.Auth.Handler())
	d.mountImageRoutes(mux)
	if d.TestHooks {
		d.mountTestHooks(mux)
	}
	mux.Handle("/", withSpecValidation(spec, http.HandlerFunc(handleNotFound)))

	strict := gen.NewStrictHandlerWithOptions(d, []gen.StrictMiddlewareFunc{d.tierMiddleware()}, gen.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  requestErrorHandler,
		ResponseErrorHandlerFunc: d.responseErrorHandler,
	})
	gen.HandlerWithOptions(strict, gen.StdHTTPServerOptions{
		BaseRouter: mux,
		Middlewares: []gen.MiddlewareFunc{
			func(next http.Handler) http.Handler { return withSpecValidation(spec, next) },
		},
	})

	return middleware.TrustedProxy(d.TrustedProxyHops)(noStore(mux))
}
```

`stubs.go` (temporary; Tasks 9 and 10 delete it method by method):

```go
package api

import (
	"context"
	"net/http"

	"github.com/refsdal/snarvei/server/internal/api/gen"
)

func (d Deps) mountImageRoutes(*http.ServeMux) {}
func (d Deps) mountTestHooks(*http.ServeMux)   {}

var notImplemented = fail(http.StatusNotImplemented, "NOT_IMPLEMENTED", "Not implemented yet")

func (d Deps) GetMe(context.Context, gen.GetMeRequestObject) (gen.GetMeResponseObject, error) { return nil, notImplemented }
// ... one identical stub per remaining operation in gen.StrictServerInterface, until `go vet` is clean.
```

Write one stub per operation listed in `operationTiers` (except the three existing ones). `go vet ./...` must pass.

- [ ] **Step 4: Update the existing tests and verify**

`api_test.go`'s `handler(t)` and `composed_test.go` must now supply `Auth`, `Q`, `RateLimit`, `Hasher`: build them inline (a real `auth.New` over `rig.Pool` with `email.NewRecording()`, `dbgen.New(rig.Pool)`, `ratelimit.NewPostgres(...)`, `clientip.NewHasher("", secret)`); `TestHealthz` keeps a nil pool but still passes the four required fields (Auth can be the real one over the rig pool). Run:

```bash
cd apps/server && mise exec -- go vet ./... && mise exec -- go test -p 1 -count=1 ./... 2>&1 | tail -15
```

Expected: all green; `TestUnknownAPIPathIsJSON404` still passes for `/api/nope`; a `GET /api/me` without a session now returns `401 UNAUTHENTICATED` (add that assertion to `api_test.go`).

- [ ] **Step 5: Commit**

```bash
git add openapi apps/server
git commit -m "feat(api): phase-2 endpoints in the spec, auth tiers, Limen mount and error mapping"
```

---

### Task 8: `testrig.AppRig`: the HTTP-level test rig

**Files:**
- Create: `apps/server/internal/testrig/http.go`, `apps/server/internal/testrig/http_test.go`

**Interfaces:**
- Produces:
  ```go
  package testrig
  const Password = "Testpass123"
  type AppRig struct { T *testing.T; Rig *Rig; Deps api.Deps; Mail *email.Recording; Store *storage.Memory; Handler http.Handler }
  func App(t *testing.T) *AppRig                                   // OpenSignup true, TestHooks true
  func (a *AppRig) SignUp(name, email string) string               // user id
  func (a *AppRig) SignIn(email string) string                     // "snarvei_session=<token>" cookie header value
  func (a *AppRig) NewOrg(name, slug, ownerEmail string) (orgID, cookie string)   // sign-up + create + switch
  func (a *AppRig) Join(orgID, ownerID, email, role string) (userID, cookie string)  // invite + accept + sign in + switch
  type Response struct { Code int; Header http.Header; Body []byte; JSON map[string]any; Array []map[string]any }
  func (a *AppRig) Do(method, path string, body any, cookie string) Response   // body: nil, string, or a value marshalled to JSON
  func (a *AppRig) DoRaw(req *http.Request) *httptest.ResponseRecorder
  ```

- [ ] **Step 1: Write `http.go`**

```go
package testrig

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/refsdal/snarvei/server/internal/api"
	"github.com/refsdal/snarvei/server/internal/auth"
	"github.com/refsdal/snarvei/server/internal/clientip"
	"github.com/refsdal/snarvei/server/internal/db/gen"
	"github.com/refsdal/snarvei/server/internal/email"
	"github.com/refsdal/snarvei/server/internal/ratelimit"
	"github.com/refsdal/snarvei/server/internal/storage"
)

const (
	appURL   = "http://127.0.0.1"
	secret   = "snarvei-testrig-auth-secret-value-0123456789"
	Password = "Testpass123"
)

// AppRig is api.NewHandler over a migrated Postgres, memory storage and a
// recording mailer, driven with real requests.
type AppRig struct {
	T       *testing.T
	Rig     *Rig
	Deps    api.Deps
	Mail    *email.Recording
	Store   *storage.Memory
	Handler http.Handler
}

// App builds the rig with open sign-up and test hooks on.
func App(t *testing.T) *AppRig {
	t.Helper()
	rig := Setup(t)
	mail := email.NewRecording()
	hasher := clientip.NewHasher("", secret)
	svc, err := auth.New(auth.Config{AppURL: appURL, AppName: "Snarvei", Secret: secret, OpenSignup: true, Pool: rig.Pool, ClientIP: hasher.Extractor(0), Email: mail})
	if err != nil {
		t.Fatalf("testrig: auth.New: %v", err)
	}
	q := gen.New(rig.Pool)
	store := storage.NewMemory()
	a := &AppRig{T: t, Rig: rig, Mail: mail, Store: store}
	a.Deps = api.Deps{
		Pool: rig.Pool, Q: q, Auth: svc, Storage: store, Email: mail, Mail: mail,
		RateLimit: ratelimit.NewPostgres(q), Hasher: hasher,
		AppURL: appURL, AppName: "Snarvei", OpenSignup: true, Version: "test", TestHooks: true,
	}
	a.Handler = api.NewHandler(a.Deps)
	return a
}

// Response is a decoded reply.
type Response struct {
	Code   int
	Header http.Header
	Body   []byte
	JSON   map[string]any
	Array  []map[string]any
}

// Do sends a request through the handler. body may be nil, a string, or any
// value (marshalled as JSON).
func (a *AppRig) Do(method, path string, body any, cookie string) Response {
	a.T.Helper()
	var reader io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		reader = strings.NewReader(b)
	default:
		raw, err := json.Marshal(b)
		if err != nil {
			a.T.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	if reader != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Origin", appURL)
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	rec := a.DoRaw(req)
	resp := Response{Code: rec.Code, Header: rec.Header(), Body: rec.Body.Bytes()}
	trimmed := bytes.TrimSpace(resp.Body)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		_ = json.Unmarshal(trimmed, &resp.JSON)
	} else if len(trimmed) > 0 && trimmed[0] == '[' {
		_ = json.Unmarshal(trimmed, &resp.Array)
	}
	return resp
}

// DoRaw serves req and returns the recorder.
func (a *AppRig) DoRaw(req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	a.Handler.ServeHTTP(rec, req)
	return rec
}

// SignUp creates a user with Password and returns its id.
func (a *AppRig) SignUp(name, addr string) string {
	a.T.Helper()
	id, err := a.Deps.Auth.CreateUser(context.Background(), name, addr, Password)
	if err != nil {
		a.T.Fatalf("testrig: SignUp(%q): %v", addr, err)
	}
	return id
}

// SignIn drives the real credential route and returns a Cookie header value.
func (a *AppRig) SignIn(addr string) string {
	a.T.Helper()
	resp := a.Do(http.MethodPost, auth.BasePath+"/signin/credential", map[string]string{"credential": addr, "password": Password}, "")
	if resp.Code != http.StatusOK {
		a.T.Fatalf("testrig: sign in %q: %d %s", addr, resp.Code, resp.Body)
	}
	for _, c := range (&http.Response{Header: resp.Header}).Cookies() {
		if c.Name == auth.SessionCookieName && c.Value != "" {
			return c.Name + "=" + c.Value
		}
	}
	a.T.Fatalf("testrig: sign in %q: no session cookie", addr)
	return ""
}

func token(cookie string) string { return strings.TrimPrefix(cookie, auth.SessionCookieName+"=") }

// NewOrg signs up ownerEmail, creates an organization and returns its id
// plus a cookie whose session has that organization active.
func (a *AppRig) NewOrg(name, slug, ownerEmail string) (string, string) {
	a.T.Helper()
	ctx := context.Background()
	ownerID := a.SignUp("Owner "+name, ownerEmail)
	org, err := a.Deps.Auth.CreateOrganization(ctx, ownerID, name, slug)
	if err != nil {
		a.T.Fatalf("testrig: CreateOrganization(%q): %v", name, err)
	}
	cookie := a.SignIn(ownerEmail)
	if err := a.Deps.Auth.SetActiveOrganization(ctx, token(cookie), org.ID); err != nil {
		a.T.Fatalf("testrig: SetActiveOrganization: %v", err)
	}
	return org.ID, cookie
}

// Join invites addr into orgID as role from ownerID, signs the invitee up,
// accepts, signs in and activates the organization.
func (a *AppRig) Join(orgID, ownerID, addr, role string) (string, string) {
	a.T.Helper()
	ctx := context.Background()
	inv, err := a.Deps.Auth.CreateInvitation(ctx, ownerID, orgID, addr, role)
	if err != nil {
		a.T.Fatalf("testrig: CreateInvitation(%q): %v", addr, err)
	}
	userID := a.SignUp("Member "+addr, addr)
	if _, err := a.Deps.Auth.AcceptInvitation(ctx, userID, inv.ID); err != nil {
		a.T.Fatalf("testrig: AcceptInvitation: %v", err)
	}
	cookie := a.SignIn(addr)
	if err := a.Deps.Auth.SetActiveOrganization(ctx, token(cookie), orgID); err != nil {
		a.T.Fatalf("testrig: SetActiveOrganization: %v", err)
	}
	return userID, cookie
}
```

- [ ] **Step 2: Write `http_test.go`**

```go
package testrig_test

import (
	"net/http"
	"testing"

	"github.com/refsdal/snarvei/server/internal/testrig"
)

func TestAppRigRoundTrip(t *testing.T) {
	a := testrig.App(t)
	if resp := a.Do(http.MethodGet, "/api/config", nil, ""); resp.Code != 200 || resp.JSON["openSignup"] != true {
		t.Fatalf("config: %d %s", resp.Code, resp.Body)
	}
	orgID, ownerCookie := a.NewOrg("Acme", "acme", "owner@example.com")
	if orgID == "" || ownerCookie == "" {
		t.Fatal("NewOrg")
	}
	ownerID := a.SignUp("Second", "second@example.com")
	_ = ownerID
	if resp := a.Do(http.MethodGet, "/api/auth/me", nil, ownerCookie); resp.Code != 200 {
		t.Fatalf("limen me: %d %s", resp.Code, resp.Body)
	}
	if resp := a.Do(http.MethodGet, "/api/auth/me", nil, ""); resp.Code != 401 {
		t.Fatalf("anonymous limen me: %d", resp.Code)
	}
}
```

Run: `cd apps/server && mise exec -- go vet ./... && mise exec -- go test -p 1 -count=1 ./internal/testrig/ -v`
Expected: PASS. (Limen's `/me` may answer 401 or 404 for anonymous; accept either in the test if it is 404.)

- [ ] **Step 3: Commit**

```bash
git add apps/server/internal/testrig
git commit -m "test(server): HTTP-level test rig with sign-up, sign-in, organization and join helpers"
```

---

### Task 9: `/api/me` handlers, profile images, sessions, email change, deletion

**Files:**
- Create: `apps/server/internal/api/me.go`, `apps/server/internal/api/images.go`, `apps/server/internal/api/me_test.go`
- Modify: `apps/server/internal/api/stubs.go` (remove the replaced stubs; delete the file once empty in Task 10)

**Interfaces:**
- Consumes: `middleware.SessionFromContext`, `gen.Queries` (`GetUserProfile`, `UpdateUserName`, `UpdateUserImage`, `UpdateUserEmail`, `ListUserSessions`, `GetUserSessionByID`, `CreateEmailChangeRequest`, `DeleteEmailChangeRequestsForUser`, `GetEmailChangeRequest`, `ListOrganizationsWhereSoleOwner`, `CountUsersByEmail`), `auth.Service` (`VerifyPassword`, `RevokeSession`, `RevokeAllSessions`, `DeleteUser`), `storage.Storage`, `email.EmailChange`.
- Produces: `func (d Deps) mountImageRoutes(mux *http.ServeMux)` registering `POST /api/me/profile-image`, `DELETE /api/me/profile-image` (session chain) and `GET /images/profile/{userId}/{file}` (public).

- [ ] **Step 1: Write the failing tests `me_test.go`**

```go
package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/refsdal/snarvei/server/internal/testrig"
)

func TestGetAndUpdateMe(t *testing.T) {
	a := testrig.App(t)
	id := a.SignUp("Kari", "kari@example.com")
	cookie := a.SignIn("kari@example.com")

	resp := a.Do(http.MethodGet, "/api/me", nil, cookie)
	if resp.Code != 200 {
		t.Fatalf("me: %d %s", resp.Code, resp.Body)
	}
	user := resp.JSON["user"].(map[string]any)
	session := resp.JSON["session"].(map[string]any)
	if user["id"] != id || user["name"] != "Kari" || user["email"] != "kari@example.com" || user["image"] != nil || user["twoFactorEnabled"] != false {
		t.Fatalf("user: %v", user)
	}
	if session["id"] == "" || session["activeOrganizationId"] != nil || strings.Contains(string(resp.Body), "token") {
		t.Fatalf("session: %v", session)
	}
	if resp := a.Do(http.MethodGet, "/api/me", nil, ""); resp.Code != 401 || resp.JSON["code"] != "UNAUTHENTICATED" {
		t.Fatalf("anonymous: %d %s", resp.Code, resp.Body)
	}

	resp = a.Do(http.MethodPatch, "/api/me", map[string]string{"name": "  Kari N.  "}, cookie)
	if resp.Code != 200 || resp.JSON["user"].(map[string]any)["name"] != "Kari N." {
		t.Fatalf("patch: %d %s", resp.Code, resp.Body)
	}
	if resp := a.Do(http.MethodPatch, "/api/me", map[string]string{"name": ""}, cookie); resp.Code != 400 || resp.JSON["code"] != "VALIDATION_FAILED" {
		t.Fatalf("empty name: %d %s", resp.Code, resp.Body)
	}
}

func pngBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(1, 1, color.RGBA{255, 0, 0, 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func upload(t *testing.T, a *testrig.AppRig, cookie string, field string, data []byte) testrig.Response {
	t.Helper()
	var body bytes.Buffer
	mp := multipart.NewWriter(&body)
	part, _ := mp.CreateFormFile(field, "avatar.bin")
	_, _ = part.Write(data)
	_ = mp.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/me/profile-image", &body)
	req.Header.Set("Content-Type", mp.FormDataContentType())
	req.Header.Set("Origin", "http://127.0.0.1")
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	rec := a.DoRaw(req)
	resp := testrig.Response{Code: rec.Code, Header: rec.Header(), Body: rec.Body.Bytes()}
	if trimmed := bytes.TrimSpace(resp.Body); len(trimmed) > 0 && trimmed[0] == '{' {
		_ = json.Unmarshal(trimmed, &resp.JSON)
	}
	return resp
}

func TestProfileImageLifecycle(t *testing.T) {
	a := testrig.App(t)
	a.SignUp("Kari", "kari@example.com")
	cookie := a.SignIn("kari@example.com")

	if resp := upload(t, a, "", "file", pngBytes(t)); resp.Code != 401 {
		t.Fatalf("anonymous upload: %d", resp.Code)
	}
	if resp := upload(t, a, cookie, "file", []byte("not an image at all")); resp.Code != 400 {
		t.Fatalf("non-image: %d %s", resp.Code, resp.Body)
	}
	if resp := upload(t, a, cookie, "wrong", pngBytes(t)); resp.Code != 400 {
		t.Fatalf("wrong field: %d %s", resp.Code, resp.Body)
	}
	if resp := upload(t, a, cookie, "file", bytes.Repeat([]byte{0}, 2<<20+1)); resp.Code != 400 && resp.Code != 413 {
		t.Fatalf("oversized: %d %s", resp.Code, resp.Body)
	}

	resp := upload(t, a, cookie, "file", pngBytes(t))
	if resp.Code != 200 {
		t.Fatalf("upload: %d %s", resp.Code, resp.Body)
	}
	imageURL, _ := resp.JSON["imageUrl"].(string)
	if !strings.HasPrefix(imageURL, "/images/profile/") || !strings.HasSuffix(imageURL, ".png") {
		t.Fatalf("imageUrl: %q", imageURL)
	}
	me := a.Do(http.MethodGet, "/api/me", nil, cookie)
	if me.JSON["user"].(map[string]any)["image"] != imageURL {
		t.Fatalf("me.image: %v", me.JSON)
	}

	get := a.Do(http.MethodGet, imageURL, nil, "")
	if get.Code != 200 || get.Header.Get("Content-Type") != "image/png" || get.Header.Get("Cache-Control") != "public, max-age=31536000, immutable" || !bytes.Equal(get.Body, pngBytes(t)) {
		t.Fatalf("serve: %d %s %s", get.Code, get.Header.Get("Content-Type"), get.Header.Get("Cache-Control"))
	}
	if miss := a.Do(http.MethodGet, "/images/profile/nobody/none.png", nil, ""); miss.Code != 404 {
		t.Fatalf("missing image: %d", miss.Code)
	}

	second := upload(t, a, cookie, "file", pngBytes(t))
	if a.Do(http.MethodGet, imageURL, nil, "").Code != 404 {
		t.Fatal("previous image must be deleted after replacement")
	}
	secondURL := second.JSON["imageUrl"].(string)

	if del := a.Do(http.MethodDelete, "/api/me/profile-image", nil, cookie); del.Code != 200 || del.JSON["imageUrl"] != nil {
		t.Fatalf("delete: %d %s", del.Code, del.Body)
	}
	if a.Do(http.MethodGet, secondURL, nil, "").Code != 404 {
		t.Fatal("deleted image still served")
	}
	if me := a.Do(http.MethodGet, "/api/me", nil, cookie); me.JSON["user"].(map[string]any)["image"] != nil {
		t.Fatal("me.image not cleared")
	}
}

func TestSessions(t *testing.T) {
	a := testrig.App(t)
	a.SignUp("Kari", "kari@example.com")
	c1 := a.SignIn("kari@example.com")
	c2 := a.SignIn("kari@example.com")
	c3 := a.SignIn("kari@example.com")

	list := a.Do(http.MethodGet, "/api/me/sessions", nil, c1)
	if list.Code != 200 || len(list.Array) != 3 || strings.Contains(string(list.Body), "token") {
		t.Fatalf("list: %d %s", list.Code, list.Body)
	}
	current, otherID := 0, ""
	for _, s := range list.Array {
		if s["current"] == true {
			current++
		} else if otherID == "" {
			otherID = s["id"].(string)
		}
	}
	if current != 1 || otherID == "" {
		t.Fatalf("current flag: %v", list.Array)
	}
	if resp := a.Do(http.MethodDelete, "/api/me/sessions/"+otherID, nil, c1); resp.Code != 204 {
		t.Fatalf("revoke one: %d %s", resp.Code, resp.Body)
	}
	if resp := a.Do(http.MethodDelete, "/api/me/sessions/"+otherID, nil, c1); resp.Code != 404 {
		t.Fatalf("revoke twice: %d", resp.Code)
	}
	if resp := a.Do(http.MethodDelete, "/api/me/sessions", nil, c1); resp.Code != 204 {
		t.Fatalf("revoke others: %d", resp.Code)
	}
	if a.Do(http.MethodGet, "/api/me", nil, c1).Code != 200 {
		t.Fatal("current session must survive")
	}
	if a.Do(http.MethodGet, "/api/me", nil, c2).Code != 401 || a.Do(http.MethodGet, "/api/me", nil, c3).Code != 401 {
		t.Fatal("other sessions must be gone")
	}
	other := a.SignUp("Other", "other@example.com")
	_ = other
	oc := a.SignIn("other@example.com")
	mine := a.Do(http.MethodGet, "/api/me/sessions", nil, c1).Array[0]["id"].(string)
	if resp := a.Do(http.MethodDelete, "/api/me/sessions/"+mine, nil, oc); resp.Code != 404 {
		t.Fatalf("revoking someone else's session must look like 404: %d", resp.Code)
	}
}

func TestEmailChange(t *testing.T) {
	a := testrig.App(t)
	a.SignUp("Kari", "kari@example.com")
	a.SignUp("Taken", "taken@example.com")
	cookie := a.SignIn("kari@example.com")

	if resp := a.Do(http.MethodPost, "/api/me/email", map[string]string{"newEmail": "new@example.com", "password": "wrong"}, cookie); resp.Code != 401 || resp.JSON["code"] != "INVALID_PASSWORD" {
		t.Fatalf("wrong password: %d %s", resp.Code, resp.Body)
	}
	if resp := a.Do(http.MethodPost, "/api/me/email", map[string]string{"newEmail": "taken@example.com", "password": testrig.Password}, cookie); resp.Code != 409 || resp.JSON["code"] != "EMAIL_TAKEN" {
		t.Fatalf("taken: %d %s", resp.Code, resp.Body)
	}
	if resp := a.Do(http.MethodPost, "/api/me/email", map[string]string{"newEmail": "new@example.com", "password": testrig.Password}, cookie); resp.Code != 202 {
		t.Fatalf("request: %d %s", resp.Code, resp.Body)
	}
	msg, ok := a.Mail.Last("new@example.com")
	if !ok || !strings.Contains(msg.Text, "/app/settings?emailToken=") {
		t.Fatalf("mail: %+v", msg)
	}
	tok := strings.Fields(msg.Text[strings.Index(msg.Text, "emailToken=")+len("emailToken="):])[0]

	if resp := a.Do(http.MethodPost, "/api/me/email/confirm", map[string]string{"token": "nope"}, cookie); resp.Code != 400 {
		t.Fatalf("bad token: %d %s", resp.Code, resp.Body)
	}
	resp := a.Do(http.MethodPost, "/api/me/email/confirm", map[string]string{"token": tok}, cookie)
	if resp.Code != 200 || resp.JSON["user"].(map[string]any)["email"] != "new@example.com" {
		t.Fatalf("confirm: %d %s", resp.Code, resp.Body)
	}
	if resp := a.Do(http.MethodPost, "/api/me/email/confirm", map[string]string{"token": tok}, cookie); resp.Code != 400 {
		t.Fatalf("token reuse: %d", resp.Code)
	}
	if a.SignIn("new@example.com") == "" {
		t.Fatal("sign in with the new address")
	}
}

func TestDeleteMe(t *testing.T) {
	a := testrig.App(t)
	orgID, ownerCookie := a.NewOrg("Acme", "acme", "owner@example.com")
	_ = orgID
	if resp := a.Do(http.MethodDelete, "/api/me", map[string]string{"password": "wrong"}, ownerCookie); resp.Code != 401 {
		t.Fatalf("wrong password: %d", resp.Code)
	}
	if resp := a.Do(http.MethodDelete, "/api/me", map[string]string{"password": testrig.Password}, ownerCookie); resp.Code != 409 || resp.JSON["code"] != "LAST_OWNER" {
		t.Fatalf("sole owner: %d %s", resp.Code, resp.Body)
	}
	a.SignUp("Loner", "loner@example.com")
	loner := a.SignIn("loner@example.com")
	if resp := a.Do(http.MethodDelete, "/api/me", map[string]string{"password": testrig.Password}, loner); resp.Code != 204 {
		t.Fatalf("delete: %d %s", resp.Code, resp.Body)
	}
	if a.Do(http.MethodGet, "/api/me", nil, loner).Code != 401 {
		t.Fatal("deleted user still signed in")
	}
	var n int
	if err := a.Rig.Pool.QueryRow(context.Background(), `SELECT count(*) FROM users WHERE email = 'loner@example.com'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("user row remains: %v %d", err, n)
	}
}
```

- [ ] **Step 2: Run to verify failure (501s), then write `me.go`**

```go
package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/refsdal/snarvei/server/internal/api/gen"
	"github.com/refsdal/snarvei/server/internal/api/middleware"
	"github.com/refsdal/snarvei/server/internal/auth"
	"github.com/refsdal/snarvei/server/internal/db"
	dbgen "github.com/refsdal/snarvei/server/internal/db/gen"
	"github.com/refsdal/snarvei/server/internal/email"
)

const emailChangeTTL = time.Hour

func ts(t pgtype.Timestamptz) time.Time { return t.Time }

func (d Deps) me(ctx context.Context, s *auth.Session) (gen.Me, error) {
	row, err := d.Q.GetUserProfile(ctx, s.UserID)
	if err != nil {
		return gen.Me{}, err
	}
	var active *string
	if s.ActiveOrganizationID != "" {
		id := s.ActiveOrganizationID
		active = &id
	}
	return gen.Me{
		User:    gen.User{Id: row.ID, Name: row.Name, Email: row.Email, Image: row.Image, TwoFactorEnabled: row.TwoFactorEnabled},
		Session: gen.SessionInfo{Id: s.SessionID, ExpiresAt: s.ExpiresAt, ActiveOrganizationId: active},
	}, nil
}

func (d Deps) GetMe(ctx context.Context, _ gen.GetMeRequestObject) (gen.GetMeResponseObject, error) {
	me, err := d.me(ctx, middleware.SessionFromContext(ctx))
	if err != nil {
		return nil, err
	}
	return gen.GetMe200JSONResponse(me), nil
}

func (d Deps) UpdateMe(ctx context.Context, req gen.UpdateMeRequestObject) (gen.UpdateMeResponseObject, error) {
	s := middleware.SessionFromContext(ctx)
	name := strings.TrimSpace(req.Body.Name)
	if name == "" || len(name) > 120 {
		return nil, fail(http.StatusBadRequest, "VALIDATION_FAILED", "Name is required")
	}
	if err := d.Q.UpdateUserName(ctx, dbgen.UpdateUserNameParams{ID: s.UserID, Name: &name}); err != nil {
		return nil, err
	}
	me, err := d.me(ctx, s)
	if err != nil {
		return nil, err
	}
	return gen.UpdateMe200JSONResponse(me), nil
}

func (d Deps) DeleteMe(ctx context.Context, req gen.DeleteMeRequestObject) (gen.DeleteMeResponseObject, error) {
	s := middleware.SessionFromContext(ctx)
	if err := d.Auth.VerifyPassword(ctx, s.UserID, req.Body.Password); err != nil {
		return nil, err
	}
	sole, err := d.Q.ListOrganizationsWhereSoleOwner(ctx, s.UserID)
	if err != nil {
		return nil, err
	}
	if len(sole) > 0 {
		return nil, fail(http.StatusConflict, "LAST_OWNER", "Transfer ownership of "+sole[0].Name+" before deleting your account")
	}
	if err := d.Auth.DeleteUser(ctx, s.UserID); err != nil {
		return nil, err
	}
	return gen.DeleteMe204Response{}, nil
}

func (d Deps) RequestEmailChange(ctx context.Context, req gen.RequestEmailChangeRequestObject) (gen.RequestEmailChangeResponseObject, error) {
	s := middleware.SessionFromContext(ctx)
	newEmail := strings.ToLower(strings.TrimSpace(string(req.Body.NewEmail)))
	if newEmail == "" || !strings.Contains(newEmail, "@") {
		return nil, fail(http.StatusBadRequest, "VALIDATION_FAILED", "A valid email is required")
	}
	if err := d.Auth.VerifyPassword(ctx, s.UserID, req.Body.Password); err != nil {
		return nil, err
	}
	if n, err := d.Q.CountUsersByEmail(ctx, newEmail); err != nil {
		return nil, err
	} else if n > 0 {
		return nil, fail(http.StatusConflict, "EMAIL_TAKEN", "An account already exists for that email")
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	tok := hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(tok))
	if err := d.Q.DeleteEmailChangeRequestsForUser(ctx, s.UserID); err != nil {
		return nil, err
	}
	if err := d.Q.CreateEmailChangeRequest(ctx, dbgen.CreateEmailChangeRequestParams{
		ID: auth.NewID(), UserID: s.UserID, NewEmail: newEmail, TokenHash: hex.EncodeToString(sum[:]),
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(emailChangeTTL), Valid: true},
	}); err != nil {
		return nil, err
	}
	link := strings.TrimRight(d.AppURL, "/") + "/app/settings?emailToken=" + tok
	if err := d.Email.Send(ctx, email.EmailChange(d.AppName, newEmail, link).To(newEmail)); err != nil {
		d.log().Warn("email change mail failed", "event", "email.send_failed", "to", newEmail, "error", err.Error())
	}
	return gen.RequestEmailChange202Response{}, nil
}

func (d Deps) ConfirmEmailChange(ctx context.Context, req gen.ConfirmEmailChangeRequestObject) (gen.ConfirmEmailChangeResponseObject, error) {
	s := middleware.SessionFromContext(ctx)
	sum := sha256.Sum256([]byte(strings.TrimSpace(req.Body.Token)))
	row, err := d.Q.GetEmailChangeRequest(ctx, hex.EncodeToString(sum[:]))
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (row.UserID != s.UserID || ts(row.ExpiresAt).Before(time.Now()))) {
		return nil, fail(http.StatusBadRequest, "VALIDATION_FAILED", "This link is invalid or has expired")
	}
	if err != nil {
		return nil, err
	}
	if err := d.Q.UpdateUserEmail(ctx, dbgen.UpdateUserEmailParams{ID: s.UserID, Email: row.NewEmail}); err != nil {
		if db.IsUniqueViolation(err) {
			return nil, fail(http.StatusConflict, "EMAIL_TAKEN", "An account already exists for that email")
		}
		return nil, err
	}
	if err := d.Q.DeleteEmailChangeRequestsForUser(ctx, s.UserID); err != nil {
		return nil, err
	}
	me, err := d.me(ctx, s)
	if err != nil {
		return nil, err
	}
	return gen.ConfirmEmailChange200JSONResponse(me), nil
}

func (d Deps) ListMySessions(ctx context.Context, _ gen.ListMySessionsRequestObject) (gen.ListMySessionsResponseObject, error) {
	s := middleware.SessionFromContext(ctx)
	rows, err := d.Q.ListUserSessions(ctx, s.UserID)
	if err != nil {
		return nil, err
	}
	out := make(gen.ListMySessions200JSONResponse, 0, len(rows))
	for _, r := range rows {
		var ua *string
		var meta map[string]any
		if r.Metadata != "" && json.Unmarshal([]byte(r.Metadata), &meta) == nil {
			if v, ok := meta["user_agent"].(string); ok && v != "" {
				ua = &v
			}
		}
		var last *time.Time
		if r.LastAccess.Valid {
			t := r.LastAccess.Time
			last = &t
		}
		out = append(out, gen.SessionSummary{Id: r.ID, CreatedAt: ts(r.CreatedAt), LastAccess: last, ExpiresAt: ts(r.ExpiresAt), UserAgent: ua, Current: r.Token == s.Token})
	}
	return out, nil
}

func (d Deps) RevokeMySession(ctx context.Context, req gen.RevokeMySessionRequestObject) (gen.RevokeMySessionResponseObject, error) {
	s := middleware.SessionFromContext(ctx)
	row, err := d.Q.GetUserSessionByID(ctx, dbgen.GetUserSessionByIDParams{ID: req.SessionId, UserID: s.UserID})
	if err != nil {
		return nil, err // pgx.ErrNoRows → 404
	}
	if err := d.Auth.RevokeSession(ctx, row.Token); err != nil {
		return nil, err
	}
	return gen.RevokeMySession204Response{}, nil
}

func (d Deps) RevokeOtherSessions(ctx context.Context, _ gen.RevokeOtherSessionsRequestObject) (gen.RevokeOtherSessionsResponseObject, error) {
	s := middleware.SessionFromContext(ctx)
	rows, err := d.Q.ListUserSessions(ctx, s.UserID)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		if r.Token != s.Token {
			if err := d.Auth.RevokeSession(ctx, r.Token); err != nil && !errors.Is(err, auth.ErrNotFound) {
				return nil, err
			}
		}
	}
	return gen.RevokeOtherSessions204Response{}, nil
}
```

Generated field names to match exactly: `req.Body.NewEmail` is `openapi_types.Email` (convert with `string(...)`), `gen.User.Image` is `*string`, `gen.SessionInfo.ActiveOrganizationId` is `*string`, `gen.SessionSummary.LastAccess` is `*time.Time`. Open `types.gen.go` and adapt.

- [ ] **Step 3: Write `images.go`**

```go
package api

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/refsdal/snarvei/server/internal/api/middleware"
	"github.com/refsdal/snarvei/server/internal/api/respond"
	"github.com/refsdal/snarvei/server/internal/auth"
	dbgen "github.com/refsdal/snarvei/server/internal/db/gen"
)

const (
	maxProfileImageBytes = 2 << 20
	profileImagePrefix   = "/images/profile/"
)

var imageExtensions = map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/webp": "webp"}

// mountImageRoutes registers the multipart upload, the delete and the public
// image stream. Binary bodies have no place in the JSON strict server.
func (d Deps) mountImageRoutes(mux *http.ServeMux) {
	session := d.chain(tierSession)
	mux.Handle("POST /api/me/profile-image", session(http.HandlerFunc(d.uploadProfileImage)))
	mux.Handle("DELETE /api/me/profile-image", session(http.HandlerFunc(d.deleteProfileImage)))
	mux.HandleFunc("GET /images/profile/{userId}/{file}", d.serveProfileImage)
}

func storageKey(publicPath string) string { return strings.TrimPrefix(publicPath, "/images/") }

func (d Deps) uploadProfileImage(w http.ResponseWriter, r *http.Request) {
	s := middleware.SessionFromContext(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, maxProfileImageBytes+64<<10)
	if err := r.ParseMultipartForm(maxProfileImageBytes); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			respond.Error(w, http.StatusBadRequest, "VALIDATION_FAILED", "Profile image must be 2 MiB or smaller")
			return
		}
		respond.Error(w, http.StatusBadRequest, "VALIDATION_FAILED", "Expected a multipart upload")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "VALIDATION_FAILED", "Profile image file is required")
		return
	}
	defer file.Close()
	if header.Size > maxProfileImageBytes {
		respond.Error(w, http.StatusBadRequest, "VALIDATION_FAILED", "Profile image must be 2 MiB or smaller")
		return
	}
	data, err := io.ReadAll(io.LimitReader(file, maxProfileImageBytes+1))
	if err != nil || len(data) > maxProfileImageBytes {
		respond.Error(w, http.StatusBadRequest, "VALIDATION_FAILED", "Profile image must be 2 MiB or smaller")
		return
	}
	ctype := http.DetectContentType(data)
	ext, ok := imageExtensions[ctype]
	if !ok {
		respond.Error(w, http.StatusBadRequest, "VALIDATION_FAILED", "Unsupported image type")
		return
	}
	key := "profile/" + s.UserID + "/" + auth.NewID() + "." + ext
	if err := d.Storage.Put(r.Context(), key, bytes.NewReader(data), int64(len(data)), ctype); err != nil {
		d.responseErrorHandler(w, r, err)
		return
	}
	publicPath := "/images/" + key
	if err := d.Q.UpdateUserImage(r.Context(), dbgen.UpdateUserImageParams{ID: s.UserID, Image: &publicPath}); err != nil {
		_ = d.Storage.Delete(r.Context(), key)
		d.responseErrorHandler(w, r, err)
		return
	}
	if s.Image != nil && strings.HasPrefix(*s.Image, profileImagePrefix+s.UserID+"/") {
		_ = d.Storage.Delete(r.Context(), storageKey(*s.Image))
	}
	respond.JSON(w, http.StatusOK, map[string]any{"imageUrl": publicPath})
}

func (d Deps) deleteProfileImage(w http.ResponseWriter, r *http.Request) {
	s := middleware.SessionFromContext(r.Context())
	if err := d.Q.UpdateUserImage(r.Context(), dbgen.UpdateUserImageParams{ID: s.UserID, Image: nil}); err != nil {
		d.responseErrorHandler(w, r, err)
		return
	}
	if s.Image != nil && strings.HasPrefix(*s.Image, profileImagePrefix+s.UserID+"/") {
		_ = d.Storage.Delete(r.Context(), storageKey(*s.Image))
	}
	respond.JSON(w, http.StatusOK, map[string]any{"imageUrl": nil})
}

func (d Deps) serveProfileImage(w http.ResponseWriter, r *http.Request) {
	userID, file := r.PathValue("userId"), r.PathValue("file")
	ext := strings.TrimPrefix(path.Ext(file), ".")
	ctype := ""
	for t, e := range imageExtensions {
		if e == ext {
			ctype = t
		}
	}
	if ctype == "" || strings.ContainsAny(userID+file, "/\\") {
		respond.Error(w, http.StatusNotFound, "NOT_FOUND", "Not found")
		return
	}
	rc, found, err := d.Storage.GetStream(r.Context(), "profile/"+userID+"/"+file)
	if err != nil {
		d.responseErrorHandler(w, r, err)
		return
	}
	if !found {
		respond.Error(w, http.StatusNotFound, "NOT_FOUND", "Not found")
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.Copy(w, rc)
}
```

`d.chain(tierSession)` ends in `CaptureHTTP`, which is harmless here. Remove the corresponding stubs (and the stub `mountImageRoutes`) from `stubs.go`.

- [ ] **Step 4: Run the tests**

Run: `cd apps/server && mise exec -- go vet ./... && mise exec -- go test -p 1 -count=1 ./internal/api/ -run 'TestGetAndUpdateMe|TestProfileImageLifecycle|TestSessions|TestEmailChange|TestDeleteMe' -v 2>&1 | tail -30`
Expected: 5 PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/server
git commit -m "feat(api): /api/me profile, image, sessions, email change and account deletion"
```

---

### Task 10: Organizations, members, invitations, teams handlers

**Files:**
- Create: `apps/server/internal/api/organizations.go`, `invitations.go`, `teams.go`, `organizations_test.go`, `invitations_test.go`, `teams_test.go`
- Delete: `apps/server/internal/api/stubs.go` (every stub is now implemented; `mountTestHooks` moves to Task 11's `testhooks.go`, so keep a one-line no-op `mountTestHooks` in `organizations.go` until then)

**Interfaces:**
- Consumes: `middleware.SessionFromContext/OrgFromContext/TeamFromContext/HTTPFromContext`, `auth.Service`, `gen.Queries` (`ListOrganizationsForUser`, `GetOrganization`, `ListOrganizationMembers`, `GetInvitation`, `ListPendingInvitations`, `SetInvitationTeam`, `GetTeam`, `CreateTeam`, `ListTeams`, `ListTeamsForMember`, `AddTeamMember`, `RemoveTeamMember`, `ListTeamMembers`, `CountOrganizationMembership`, `CountUsersByEmail`), `authz`, `auth.RolesFromJSON`.

- [ ] **Step 1: Write the failing tests**

`organizations_test.go`:

```go
package api_test

import (
	"net/http"
	"testing"

	"github.com/refsdal/snarvei/server/internal/testrig"
)

func TestOrganizationsCreateListSwitch(t *testing.T) {
	a := testrig.App(t)
	a.SignUp("Kari", "kari@example.com")
	cookie := a.SignIn("kari@example.com")

	if resp := a.Do(http.MethodGet, "/api/organizations", nil, cookie); resp.Code != 200 || len(resp.Array) != 0 {
		t.Fatalf("empty list: %d %s", resp.Code, resp.Body)
	}
	resp := a.Do(http.MethodPost, "/api/organizations", map[string]string{"name": "Acme", "slug": "acme"}, cookie)
	if resp.Code != 201 || resp.JSON["slug"] != "acme" || resp.JSON["role"] != "owner" {
		t.Fatalf("create: %d %s", resp.Code, resp.Body)
	}
	orgID := resp.JSON["id"].(string)
	if resp := a.Do(http.MethodPost, "/api/organizations", map[string]string{"name": "Acme 2", "slug": "acme"}, cookie); resp.Code != 409 || resp.JSON["code"] != "SLUG_TAKEN" {
		t.Fatalf("slug taken: %d %s", resp.Code, resp.Body)
	}
	if resp := a.Do(http.MethodPost, "/api/organizations", map[string]string{"name": "Bad", "slug": "Not Valid"}, cookie); resp.Code != 400 {
		t.Fatalf("bad slug: %d %s", resp.Code, resp.Body)
	}
	list := a.Do(http.MethodGet, "/api/organizations", nil, cookie)
	if len(list.Array) != 1 || list.Array[0]["id"] != orgID || list.Array[0]["role"] != "owner" {
		t.Fatalf("list: %s", list.Body)
	}
	if me := a.Do(http.MethodGet, "/api/me", nil, cookie); me.JSON["session"].(map[string]any)["activeOrganizationId"] != nil {
		t.Fatal("creating must not switch by itself")
	}
	if resp := a.Do(http.MethodPost, "/api/organizations/"+orgID+"/switch", nil, cookie); resp.Code != 204 {
		t.Fatalf("switch: %d %s", resp.Code, resp.Body)
	}
	if me := a.Do(http.MethodGet, "/api/me", nil, cookie); me.JSON["session"].(map[string]any)["activeOrganizationId"] != orgID {
		t.Fatalf("active org: %s", me.Body)
	}
	a.SignUp("Stranger", "stranger@example.com")
	stranger := a.SignIn("stranger@example.com")
	if resp := a.Do(http.MethodPost, "/api/organizations/"+orgID+"/switch", nil, stranger); resp.Code != 403 {
		t.Fatalf("stranger switch: %d", resp.Code)
	}
	members := a.Do(http.MethodGet, "/api/organizations/"+orgID+"/members", nil, cookie)
	if members.Code != 200 || len(members.Array) != 1 || members.Array[0]["role"] != "owner" || members.Array[0]["email"] != "kari@example.com" {
		t.Fatalf("members: %d %s", members.Code, members.Body)
	}
	if resp := a.Do(http.MethodGet, "/api/organizations/"+orgID+"/members", nil, stranger); resp.Code != 403 {
		t.Fatalf("stranger members: %d", resp.Code)
	}
}
```

`invitations_test.go`:

```go
package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/refsdal/snarvei/server/internal/testrig"
)

func TestInvitationFlowWithTeam(t *testing.T) {
	a := testrig.App(t)
	orgID, owner := a.NewOrg("Acme", "acme", "owner@example.com")
	team := a.Do(http.MethodPost, "/api/organizations/"+orgID+"/teams", map[string]string{"name": "Marketing"}, owner)
	if team.Code != 201 {
		t.Fatalf("team: %d %s", team.Code, team.Body)
	}
	teamID := team.JSON["id"].(string)

	resp := a.Do(http.MethodPost, "/api/organizations/"+orgID+"/invitations", map[string]string{"email": "new@example.com", "role": "member", "teamId": teamID}, owner)
	if resp.Code != 201 || resp.JSON["teamId"] != teamID || resp.JSON["role"] != "member" || resp.JSON["status"] != "pending" {
		t.Fatalf("invite: %d %s", resp.Code, resp.Body)
	}
	invID := resp.JSON["id"].(string)
	msg, ok := a.Mail.Last("new@example.com")
	if !ok || !strings.Contains(msg.Text, "/app/invitations/"+invID) {
		t.Fatalf("mail: %+v", msg)
	}
	if resp := a.Do(http.MethodPost, "/api/organizations/"+orgID+"/invitations", map[string]string{"email": "new@example.com", "role": "member"}, owner); resp.Code != 409 || resp.JSON["code"] != "INVITATION_EXISTS" {
		t.Fatalf("duplicate: %d %s", resp.Code, resp.Body)
	}
	if resp := a.Do(http.MethodPost, "/api/organizations/"+orgID+"/invitations", map[string]string{"email": "x@example.com", "role": "member", "teamId": "nope"}, owner); resp.Code != 404 {
		t.Fatalf("unknown team: %d %s", resp.Code, resp.Body)
	}
	list := a.Do(http.MethodGet, "/api/organizations/"+orgID+"/invitations", nil, owner)
	if list.Code != 200 || len(list.Array) != 1 || list.Array[0]["teamName"] != "Marketing" {
		t.Fatalf("list: %d %s", list.Code, list.Body)
	}

	// Public view: no email, hasAccount false.
	pub := a.Do(http.MethodGet, "/api/invitations/"+invID, nil, "")
	if pub.Code != 200 || pub.JSON["organizationName"] != "Acme" || pub.JSON["teamName"] != "Marketing" || pub.JSON["hasAccount"] != false || strings.Contains(string(pub.Body), "new@example.com") {
		t.Fatalf("public: %d %s", pub.Code, pub.Body)
	}
	if a.Do(http.MethodGet, "/api/invitations/nope", nil, "").Code != 404 {
		t.Fatal("unknown invitation")
	}

	// Register through the invitation: account created, accepted, signed in, team joined.
	reg := a.Do(http.MethodPost, "/api/invitations/"+invID+"/register", map[string]string{"name": "New Person", "password": testrig.Password}, "")
	if reg.Code != 201 || reg.JSON["user"].(map[string]any)["email"] != "new@example.com" {
		t.Fatalf("register: %d %s", reg.Code, reg.Body)
	}
	var newCookie string
	for _, c := range (&http.Response{Header: reg.Header}).Cookies() {
		if c.Name == "snarvei_session" {
			newCookie = c.Name + "=" + c.Value
		}
	}
	if newCookie == "" {
		t.Fatal("register must set the session cookie")
	}
	if reg.JSON["session"].(map[string]any)["activeOrganizationId"] != orgID {
		t.Fatalf("register must activate the organization: %s", reg.Body)
	}
	members := a.Do(http.MethodGet, "/api/teams/"+teamID+"/members", nil, newCookie)
	if members.Code != 200 || len(members.Array) != 1 || members.Array[0]["email"] != "new@example.com" {
		t.Fatalf("team members after register: %d %s", members.Code, members.Body)
	}
	if resp := a.Do(http.MethodPost, "/api/invitations/"+invID+"/register", map[string]string{"name": "Again", "password": testrig.Password}, ""); resp.Code != 410 {
		t.Fatalf("register twice: %d %s", resp.Code, resp.Body)
	}
	if pub := a.Do(http.MethodGet, "/api/invitations/"+invID, nil, ""); pub.JSON["status"] != "accepted" || pub.JSON["hasAccount"] != true {
		t.Fatalf("public after accept: %s", pub.Body)
	}
}

func TestInvitationAcceptRejectCancel(t *testing.T) {
	a := testrig.App(t)
	orgID, owner := a.NewOrg("Acme", "acme", "owner@example.com")
	a.SignUp("Existing", "existing@example.com")
	existing := a.SignIn("existing@example.com")
	a.SignUp("Other", "other@example.com")
	other := a.SignIn("other@example.com")

	inv := a.Do(http.MethodPost, "/api/organizations/"+orgID+"/invitations", map[string]string{"email": "existing@example.com", "role": "admin"}, owner)
	invID := inv.JSON["id"].(string)
	if pub := a.Do(http.MethodGet, "/api/invitations/"+invID, nil, ""); pub.JSON["hasAccount"] != true {
		t.Fatalf("hasAccount: %s", pub.Body)
	}
	if resp := a.Do(http.MethodPost, "/api/invitations/"+invID+"/register", map[string]string{"name": "X", "password": testrig.Password}, ""); resp.Code != 409 || resp.JSON["code"] != "EMAIL_TAKEN" {
		t.Fatalf("register with existing account: %d %s", resp.Code, resp.Body)
	}
	if resp := a.Do(http.MethodPost, "/api/invitations/"+invID+"/accept", nil, ""); resp.Code != 401 {
		t.Fatalf("anonymous accept: %d", resp.Code)
	}
	if resp := a.Do(http.MethodPost, "/api/invitations/"+invID+"/accept", nil, other); resp.Code != 403 || resp.JSON["code"] != "INVITATION_EMAIL_MISMATCH" {
		t.Fatalf("wrong user accept: %d %s", resp.Code, resp.Body)
	}
	resp := a.Do(http.MethodPost, "/api/invitations/"+invID+"/accept", nil, existing)
	if resp.Code != 200 || resp.JSON["id"] != orgID || resp.JSON["role"] != "admin" {
		t.Fatalf("accept: %d %s", resp.Code, resp.Body)
	}
	if me := a.Do(http.MethodGet, "/api/me", nil, existing); me.JSON["session"].(map[string]any)["activeOrganizationId"] != orgID {
		t.Fatal("accept must activate the organization")
	}
	if resp := a.Do(http.MethodPost, "/api/invitations/"+invID+"/accept", nil, existing); resp.Code != 410 {
		t.Fatalf("accept twice: %d", resp.Code)
	}

	inv2 := a.Do(http.MethodPost, "/api/organizations/"+orgID+"/invitations", map[string]string{"email": "other@example.com", "role": "member"}, existing) // admins may invite
	if inv2.Code != 201 {
		t.Fatalf("admin invite: %d %s", inv2.Code, inv2.Body)
	}
	if resp := a.Do(http.MethodPost, "/api/invitations/"+inv2.JSON["id"].(string)+"/reject", nil, other); resp.Code != 204 {
		t.Fatalf("reject: %d %s", resp.Code, resp.Body)
	}
	if list := a.Do(http.MethodGet, "/api/organizations/"+orgID+"/invitations", nil, owner); len(list.Array) != 0 {
		t.Fatalf("rejected must leave the pending list: %s", list.Body)
	}

	inv3 := a.Do(http.MethodPost, "/api/organizations/"+orgID+"/invitations", map[string]string{"email": "third@example.com", "role": "member"}, owner)
	inv3ID := inv3.JSON["id"].(string)
	_, memberCookie := a.Join(orgID, ownerIDOf(t, a, "owner@example.com"), "plain@example.com", "member")
	if resp := a.Do(http.MethodDelete, "/api/organizations/"+orgID+"/invitations/"+inv3ID, nil, memberCookie); resp.Code != 403 {
		t.Fatalf("member cancel: %d", resp.Code)
	}
	if resp := a.Do(http.MethodPost, "/api/organizations/"+orgID+"/invitations", map[string]string{"email": "z@example.com", "role": "member"}, memberCookie); resp.Code != 403 {
		t.Fatalf("member invite: %d", resp.Code)
	}
	if resp := a.Do(http.MethodDelete, "/api/organizations/"+orgID+"/invitations/"+inv3ID, nil, owner); resp.Code != 204 {
		t.Fatalf("cancel: %d %s", resp.Code, resp.Body)
	}
	if resp := a.Do(http.MethodDelete, "/api/organizations/"+orgID+"/invitations/"+inv3ID, nil, owner); resp.Code != 404 {
		t.Fatalf("cancel twice: %d", resp.Code)
	}
}

func ownerIDOf(t *testing.T, a *testrig.AppRig, email string) string {
	t.Helper()
	var id string
	if err := a.Rig.Pool.QueryRow(context.Background(), `SELECT id FROM users WHERE email = $1`, email).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestRegisterIsRateLimited(t *testing.T) {
	a := testrig.App(t)
	for i := 0; i < 31; i++ {
		resp := a.Do(http.MethodPost, "/api/invitations/nope/register", map[string]string{"name": "X", "password": testrig.Password}, "")
		if i < 30 && resp.Code != 404 {
			t.Fatalf("attempt %d: %d %s", i, resp.Code, resp.Body)
		}
		if i == 30 && (resp.Code != 429 || resp.Header.Get("Retry-After") == "") {
			t.Fatalf("attempt 31: %d", resp.Code)
		}
	}
}
```

`teams_test.go`:

```go
package api_test

import (
	"net/http"
	"testing"

	"github.com/refsdal/snarvei/server/internal/testrig"
)

func TestTeamsAndMembership(t *testing.T) {
	a := testrig.App(t)
	orgID, owner := a.NewOrg("Acme", "acme", "owner@example.com")
	ownerID := ownerIDOf(t, a, "owner@example.com")
	memberID, member := a.Join(orgID, ownerID, "member@example.com", "member")
	adminID, admin := a.Join(orgID, ownerID, "admin@example.com", "admin")
	_ = adminID
	a.SignUp("Outsider", "outsider@example.com")
	outsider := a.SignIn("outsider@example.com")

	if resp := a.Do(http.MethodPost, "/api/organizations/"+orgID+"/teams", map[string]string{"name": "Marketing"}, member); resp.Code != 403 {
		t.Fatalf("member create team: %d", resp.Code)
	}
	created := a.Do(http.MethodPost, "/api/organizations/"+orgID+"/teams", map[string]string{"name": "Marketing"}, admin)
	if created.Code != 201 || created.JSON["name"] != "Marketing" || created.JSON["memberCount"] != float64(0) {
		t.Fatalf("create: %d %s", created.Code, created.Body)
	}
	teamID := created.JSON["id"].(string)
	if resp := a.Do(http.MethodPost, "/api/organizations/"+orgID+"/teams", map[string]string{"name": "marketing"}, owner); resp.Code != 409 {
		t.Fatalf("duplicate name (case-insensitive): %d %s", resp.Code, resp.Body)
	}
	second := a.Do(http.MethodPost, "/api/organizations/"+orgID+"/teams", map[string]string{"name": "Sales"}, owner)

	if list := a.Do(http.MethodGet, "/api/organizations/"+orgID+"/teams", nil, owner); len(list.Array) != 2 {
		t.Fatalf("owner sees all: %s", list.Body)
	}
	if list := a.Do(http.MethodGet, "/api/organizations/"+orgID+"/teams", nil, member); len(list.Array) != 0 {
		t.Fatalf("member sees none yet: %s", list.Body)
	}
	if resp := a.Do(http.MethodGet, "/api/organizations/"+orgID+"/teams", nil, outsider); resp.Code != 403 {
		t.Fatalf("outsider: %d", resp.Code)
	}

	if resp := a.Do(http.MethodPost, "/api/teams/"+teamID+"/members", map[string]string{"userId": memberID}, member); resp.Code != 403 {
		t.Fatalf("member adds self: %d", resp.Code)
	}
	if resp := a.Do(http.MethodPost, "/api/teams/"+teamID+"/members", map[string]string{"userId": "nobody"}, owner); resp.Code != 404 {
		t.Fatalf("unknown user: %d %s", resp.Code, resp.Body)
	}
	if resp := a.Do(http.MethodPost, "/api/teams/"+teamID+"/members", map[string]string{"userId": memberID}, owner); resp.Code != 204 {
		t.Fatalf("add: %d %s", resp.Code, resp.Body)
	}
	if resp := a.Do(http.MethodPost, "/api/teams/"+teamID+"/members", map[string]string{"userId": memberID}, owner); resp.Code != 204 {
		t.Fatalf("add is idempotent: %d", resp.Code)
	}
	if list := a.Do(http.MethodGet, "/api/organizations/"+orgID+"/teams", nil, member); len(list.Array) != 1 || list.Array[0]["id"] != teamID || list.Array[0]["memberCount"] != float64(1) {
		t.Fatalf("member sees own team: %s", list.Body)
	}
	if resp := a.Do(http.MethodGet, "/api/teams/"+second.JSON["id"].(string)+"/members", nil, member); resp.Code != 403 {
		t.Fatalf("member reads other team: %d", resp.Code)
	}
	if resp := a.Do(http.MethodGet, "/api/teams/"+teamID+"/members", nil, member); resp.Code != 200 || len(resp.Array) != 1 || resp.Array[0]["userId"] != memberID {
		t.Fatalf("team members: %d %s", resp.Code, resp.Body)
	}
	if resp := a.Do(http.MethodGet, "/api/teams/nope/members", nil, owner); resp.Code != 404 {
		t.Fatalf("unknown team: %d", resp.Code)
	}
	if resp := a.Do(http.MethodDelete, "/api/teams/"+teamID+"/members/"+memberID, nil, member); resp.Code != 403 {
		t.Fatalf("member removes: %d", resp.Code)
	}
	if resp := a.Do(http.MethodDelete, "/api/teams/"+teamID+"/members/"+memberID, nil, admin); resp.Code != 204 {
		t.Fatalf("remove: %d %s", resp.Code, resp.Body)
	}
	if resp := a.Do(http.MethodDelete, "/api/teams/"+teamID+"/members/"+memberID, nil, admin); resp.Code != 404 {
		t.Fatalf("remove twice: %d", resp.Code)
	}
}
```

- [ ] **Step 2: Write `organizations.go`**

```go
package api

import (
	"context"
	"net/http"
	"regexp"
	"strings"

	"github.com/refsdal/snarvei/server/internal/api/gen"
	"github.com/refsdal/snarvei/server/internal/api/middleware"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func (d Deps) mountTestHooks(*http.ServeMux) {} // replaced in Task 11

func (d Deps) ListOrganizations(ctx context.Context, _ gen.ListOrganizationsRequestObject) (gen.ListOrganizationsResponseObject, error) {
	s := middleware.SessionFromContext(ctx)
	rows, err := d.Q.ListOrganizationsForUser(ctx, s.UserID)
	if err != nil {
		return nil, err
	}
	out := make(gen.ListOrganizations200JSONResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, gen.Organization{Id: r.ID, Name: r.Name, Slug: r.Slug, Role: gen.OrganizationRole(roleString(r.Role))})
	}
	return out, nil
}

func (d Deps) CreateOrganization(ctx context.Context, req gen.CreateOrganizationRequestObject) (gen.CreateOrganizationResponseObject, error) {
	s := middleware.SessionFromContext(ctx)
	name := strings.TrimSpace(req.Body.Name)
	slug := strings.ToLower(strings.TrimSpace(req.Body.Slug))
	if name == "" || len(name) > 120 || !slugPattern.MatchString(slug) || len(slug) < 2 || len(slug) > 64 {
		return nil, fail(http.StatusBadRequest, "VALIDATION_FAILED", "Name and a slug of lowercase letters, digits and single hyphens are required")
	}
	org, err := d.Auth.CreateOrganization(ctx, s.UserID, name, slug)
	if err != nil {
		return nil, err
	}
	return gen.CreateOrganization201JSONResponse{Id: org.ID, Name: org.Name, Slug: org.Slug, Role: gen.Owner}, nil
}

func (d Deps) SwitchOrganization(ctx context.Context, _ gen.SwitchOrganizationRequestObject) (gen.SwitchOrganizationResponseObject, error) {
	s := middleware.SessionFromContext(ctx)
	o, _ := middleware.OrgFromContext(ctx)
	if err := d.Auth.SetActiveOrganization(ctx, s.Token, o.OrgID); err != nil {
		return nil, err
	}
	return gen.SwitchOrganization204Response{}, nil
}

func (d Deps) ListOrganizationMembers(ctx context.Context, _ gen.ListOrganizationMembersRequestObject) (gen.ListOrganizationMembersResponseObject, error) {
	o, _ := middleware.OrgFromContext(ctx)
	rows, err := d.Q.ListOrganizationMembers(ctx, o.OrgID)
	if err != nil {
		return nil, err
	}
	out := make(gen.ListOrganizationMembers200JSONResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, gen.Member{Id: r.MemberID, UserId: r.UserID, Name: r.Name, Email: r.Email, Role: roleString(r.Role), CreatedAt: ts(r.CreatedAt)})
	}
	return out, nil
}

// roleString unwraps sqlc's nullable role column (a subquery result).
func roleString(v any) string {
	switch r := v.(type) {
	case string:
		return r
	case *string:
		if r != nil {
			return *r
		}
	}
	return ""
}
```

(`gen.OrganizationRole`'s enum constants are generated from `[owner, admin, member]`; use the generated names, for example `gen.Owner` or `gen.OrganizationRoleOwner`.)

- [ ] **Step 3: Write `invitations.go`**

```go
package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/refsdal/snarvei/server/internal/api/gen"
	"github.com/refsdal/snarvei/server/internal/api/middleware"
	"github.com/refsdal/snarvei/server/internal/auth"
	"github.com/refsdal/snarvei/server/internal/authz"
	dbgen "github.com/refsdal/snarvei/server/internal/db/gen"
)

func optTime(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

func optString(s *string) *string { return s }

func firstRole(raw string) string {
	roles := auth.RolesFromJSON(raw)
	if len(roles) == 0 {
		return ""
	}
	return roles[0]
}

func (d Deps) ListInvitations(ctx context.Context, _ gen.ListInvitationsRequestObject) (gen.ListInvitationsResponseObject, error) {
	o, _ := middleware.OrgFromContext(ctx)
	rows, err := d.Q.ListPendingInvitations(ctx, o.OrgID)
	if err != nil {
		return nil, err
	}
	out := make(gen.ListInvitations200JSONResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, gen.Invitation{Id: r.ID, Email: r.Email, Role: firstRole(r.Roles), Status: r.Status, ExpiresAt: optTime(r.ExpiresAt), TeamId: r.TeamID, TeamName: r.TeamName, CreatedAt: ts(r.CreatedAt)})
	}
	return out, nil
}

func (d Deps) CreateInvitation(ctx context.Context, req gen.CreateInvitationRequestObject) (gen.CreateInvitationResponseObject, error) {
	o, _ := middleware.OrgFromContext(ctx)
	role := string(req.Body.Role)
	if !authz.IsValidInviteRole(role) {
		return nil, fail(http.StatusBadRequest, "VALIDATION_FAILED", "Role must be admin or member")
	}
	var team *dbgen.Teams
	if req.Body.TeamId != nil && *req.Body.TeamId != "" {
		t, err := d.Q.GetTeam(ctx, *req.Body.TeamId)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && t.OrganizationID != o.OrgID) {
			return nil, fail(http.StatusNotFound, "NOT_FOUND", "Team not found")
		}
		if err != nil {
			return nil, err
		}
		team = &t
	}
	inv, err := d.Auth.CreateInvitation(ctx, o.UserID, o.OrgID, strings.TrimSpace(string(req.Body.Email)), role)
	if err != nil {
		return nil, err
	}
	if team != nil {
		if err := d.Q.SetInvitationTeam(ctx, dbgen.SetInvitationTeamParams{InvitationID: inv.ID, TeamID: team.ID}); err != nil {
			return nil, err
		}
	}
	row, err := d.Q.GetInvitation(ctx, inv.ID)
	if err != nil {
		return nil, err
	}
	return gen.CreateInvitation201JSONResponse{Id: row.ID, Email: row.Email, Role: firstRole(row.Roles), Status: row.Status, ExpiresAt: optTime(row.ExpiresAt), TeamId: row.TeamID, TeamName: row.TeamName, CreatedAt: ts(row.CreatedAt)}, nil
}

func (d Deps) CancelInvitation(ctx context.Context, req gen.CancelInvitationRequestObject) (gen.CancelInvitationResponseObject, error) {
	o, _ := middleware.OrgFromContext(ctx)
	row, err := d.Q.GetInvitation(ctx, req.InvitationId)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (row.OrganizationID != o.OrgID || row.Status != "pending")) {
		return nil, fail(http.StatusNotFound, "NOT_FOUND", "Invitation not found")
	}
	if err != nil {
		return nil, err
	}
	if err := d.Auth.CancelInvitation(ctx, o.UserID, o.OrgID, req.InvitationId); err != nil {
		return nil, err
	}
	return gen.CancelInvitation204Response{}, nil
}

func (d Deps) GetInvitation(ctx context.Context, req gen.GetInvitationRequestObject) (gen.GetInvitationResponseObject, error) {
	row, err := d.Q.GetInvitation(ctx, req.InvitationId)
	if err != nil {
		return nil, err
	}
	n, err := d.Q.CountUsersByEmail(ctx, row.Email)
	if err != nil {
		return nil, err
	}
	var inviter *string
	if row.InviterName != "" {
		inviter = &row.InviterName
	}
	return gen.GetInvitation200JSONResponse{Id: row.ID, OrganizationName: row.OrganizationName, InviterName: inviter, Role: firstRole(row.Roles), Status: row.Status, TeamName: row.TeamName, ExpiresAt: optTime(row.ExpiresAt), HasAccount: n > 0}, nil
}

// joinTeamIfInvited adds the team membership an invitation carried.
func (d Deps) joinTeamIfInvited(ctx context.Context, invitationID, userID string) error {
	row, err := d.Q.GetInvitation(ctx, invitationID)
	if err != nil {
		return err
	}
	if row.TeamID != nil {
		return d.Q.AddTeamMember(ctx, dbgen.AddTeamMemberParams{TeamID: *row.TeamID, UserID: userID})
	}
	return nil
}

func (d Deps) AcceptInvitation(ctx context.Context, req gen.AcceptInvitationRequestObject) (gen.AcceptInvitationResponseObject, error) {
	s := middleware.SessionFromContext(ctx)
	inv, err := d.Auth.AcceptInvitation(ctx, s.UserID, req.InvitationId)
	if err != nil {
		return nil, err
	}
	if err := d.joinTeamIfInvited(ctx, inv.ID, s.UserID); err != nil {
		return nil, err
	}
	if err := d.Auth.SetActiveOrganization(ctx, s.Token, inv.OrganizationID); err != nil {
		return nil, err
	}
	org, err := d.Q.GetOrganization(ctx, inv.OrganizationID)
	if err != nil {
		return nil, err
	}
	return gen.AcceptInvitation200JSONResponse{Id: org.ID, Name: org.Name, Slug: org.Slug, Role: gen.OrganizationRole(inv.Role)}, nil
}

func (d Deps) RejectInvitation(ctx context.Context, req gen.RejectInvitationRequestObject) (gen.RejectInvitationResponseObject, error) {
	s := middleware.SessionFromContext(ctx)
	if err := d.Auth.RejectInvitation(ctx, s.UserID, req.InvitationId); err != nil {
		return nil, err
	}
	return gen.RejectInvitation204Response{}, nil
}

func (d Deps) RegisterWithInvitation(ctx context.Context, req gen.RegisterWithInvitationRequestObject) (gen.RegisterWithInvitationResponseObject, error) {
	row, err := d.Q.GetInvitation(ctx, req.InvitationId)
	if err != nil {
		return nil, err
	}
	if row.Status != "pending" || (row.ExpiresAt.Valid && row.ExpiresAt.Time.Before(time.Now())) {
		return nil, fail(http.StatusGone, "INVITATION_INVALID", "This invitation is no longer valid")
	}
	if n, err := d.Q.CountUsersByEmail(ctx, row.Email); err != nil {
		return nil, err
	} else if n > 0 {
		return nil, fail(http.StatusConflict, "EMAIL_TAKEN", "An account already exists for this email; sign in to accept")
	}
	w, r, ok := middleware.HTTPFromContext(ctx)
	if !ok {
		return nil, errors.New("api: RegisterWithInvitation needs CaptureHTTP")
	}
	userID, err := d.Auth.CreateUser(ctx, req.Body.Name, row.Email, req.Body.Password)
	if err != nil {
		return nil, err
	}
	inv, err := d.Auth.AcceptInvitation(ctx, userID, req.InvitationId)
	if err != nil {
		return nil, err
	}
	if err := d.joinTeamIfInvited(ctx, inv.ID, userID); err != nil {
		return nil, err
	}
	if err := d.Auth.StartSession(ctx, w, r, userID); err != nil {
		return nil, err
	}
	// The cookie is on w; resolve the fresh session from the Set-Cookie value
	// to activate the organization and build the Me body.
	token := ""
	for _, c := range (&http.Response{Header: w.Header()}).Cookies() {
		if c.Name == auth.SessionCookieName {
			token = c.Value
		}
	}
	if token == "" {
		return nil, errors.New("api: StartSession set no cookie")
	}
	if err := d.Auth.SetActiveOrganization(ctx, token, inv.OrganizationID); err != nil {
		return nil, err
	}
	probe := r.Clone(ctx)
	probe.Header = http.Header{"Cookie": {auth.SessionCookieName + "=" + token}}
	s, err := d.Auth.SessionFromRequest(nil, probe)
	if err != nil || s == nil {
		return nil, errors.New("api: fresh session did not resolve")
	}
	me, err := d.me(ctx, s)
	if err != nil {
		return nil, err
	}
	return gen.RegisterWithInvitation201JSONResponse(me), nil
}
```

`SessionFromRequest(nil, probe)` is legal: session.go only writes the cookie when `w != nil`.

- [ ] **Step 4: Write `teams.go`**

```go
package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/refsdal/snarvei/server/internal/api/gen"
	"github.com/refsdal/snarvei/server/internal/api/middleware"
	"github.com/refsdal/snarvei/server/internal/auth"
	"github.com/refsdal/snarvei/server/internal/authz"
	"github.com/refsdal/snarvei/server/internal/db"
	dbgen "github.com/refsdal/snarvei/server/internal/db/gen"
)

func (d Deps) ListTeams(ctx context.Context, _ gen.ListTeamsRequestObject) (gen.ListTeamsResponseObject, error) {
	o, _ := middleware.OrgFromContext(ctx)
	out := gen.ListTeams200JSONResponse{}
	if authz.IsOrgAdmin(o.Role) {
		rows, err := d.Q.ListTeams(ctx, o.OrgID)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, gen.Team{Id: r.ID, OrganizationId: r.OrganizationID, Name: r.Name, MemberCount: int(r.MemberCount), CreatedAt: ts(r.CreatedAt)})
		}
		return out, nil
	}
	rows, err := d.Q.ListTeamsForMember(ctx, dbgen.ListTeamsForMemberParams{OrganizationID: o.OrgID, UserID: o.UserID})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out = append(out, gen.Team{Id: r.ID, OrganizationId: r.OrganizationID, Name: r.Name, MemberCount: int(r.MemberCount), CreatedAt: ts(r.CreatedAt)})
	}
	return out, nil
}

func (d Deps) CreateTeam(ctx context.Context, req gen.CreateTeamRequestObject) (gen.CreateTeamResponseObject, error) {
	o, _ := middleware.OrgFromContext(ctx)
	name := strings.TrimSpace(req.Body.Name)
	if name == "" || len(name) > 120 {
		return nil, fail(http.StatusBadRequest, "VALIDATION_FAILED", "Team name is required")
	}
	existing, err := d.Q.ListTeams(ctx, o.OrgID)
	if err != nil {
		return nil, err
	}
	for _, t := range existing {
		if strings.EqualFold(t.Name, name) {
			return nil, fail(http.StatusConflict, "TEAM_EXISTS", "A team with that name already exists")
		}
	}
	row, err := d.Q.CreateTeam(ctx, dbgen.CreateTeamParams{ID: auth.NewID(), OrganizationID: o.OrgID, Name: name})
	if err != nil {
		if db.IsUniqueViolation(err) {
			return nil, fail(http.StatusConflict, "TEAM_EXISTS", "A team with that name already exists")
		}
		return nil, err
	}
	return gen.CreateTeam201JSONResponse{Id: row.ID, OrganizationId: row.OrganizationID, Name: row.Name, MemberCount: 0, CreatedAt: ts(row.CreatedAt)}, nil
}

func (d Deps) ListTeamMembers(ctx context.Context, _ gen.ListTeamMembersRequestObject) (gen.ListTeamMembersResponseObject, error) {
	tc, _ := middleware.TeamFromContext(ctx)
	rows, err := d.Q.ListTeamMembers(ctx, tc.TeamID)
	if err != nil {
		return nil, err
	}
	out := make(gen.ListTeamMembers200JSONResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, gen.TeamMember{UserId: r.UserID, Name: r.Name, Email: r.Email, CreatedAt: ts(r.CreatedAt)})
	}
	return out, nil
}

func (d Deps) AddTeamMember(ctx context.Context, req gen.AddTeamMemberRequestObject) (gen.AddTeamMemberResponseObject, error) {
	tc, _ := middleware.TeamFromContext(ctx)
	n, err := d.Q.CountOrganizationMembership(ctx, dbgen.CountOrganizationMembershipParams{OrganizationID: tc.OrgID, UserID: req.Body.UserId})
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, fail(http.StatusNotFound, "NOT_FOUND", "User is not a member of this organization")
	}
	if err := d.Q.AddTeamMember(ctx, dbgen.AddTeamMemberParams{TeamID: tc.TeamID, UserID: req.Body.UserId}); err != nil {
		return nil, err
	}
	return gen.AddTeamMember204Response{}, nil
}

func (d Deps) RemoveTeamMember(ctx context.Context, req gen.RemoveTeamMemberRequestObject) (gen.RemoveTeamMemberResponseObject, error) {
	tc, _ := middleware.TeamFromContext(ctx)
	removed, err := d.Q.RemoveTeamMember(ctx, dbgen.RemoveTeamMemberParams{TeamID: tc.TeamID, UserID: req.UserId})
	if err != nil {
		return nil, err
	}
	if removed == 0 {
		return nil, fail(http.StatusNotFound, "NOT_FOUND", "Not a member of this team")
	}
	return gen.RemoveTeamMember204Response{}, nil
}
```

Delete `stubs.go`. Match generated names (`gen.Team.MemberCount` is `int`; `req.Body.TeamId` is `*string`; `req.Body.Email` is `openapi_types.Email`).

- [ ] **Step 5: Run the whole API suite**

Run: `cd apps/server && mise exec -- go vet ./... && mise exec -- go test -p 1 -count=1 ./internal/api/ -v 2>&1 | grep -E "^(=== RUN|--- |PASS|FAIL|ok)" | tail -40`
Expected: every test PASS. `TestRegisterIsRateLimited` needs the `write` limiter at 30/min: it is shared with `RequestEmailChange`, so run it in its own rig (it is).

- [ ] **Step 6: Commit**

```bash
git add apps/server
git commit -m "feat(api): organizations, members, invitations with team attachment, teams and team membership"
```

---

### Task 11: Composition root: storage, email, auth, rate limiter, slog, test hooks

**Files:**
- Modify: `apps/server/cmd/snarvei/main.go`, `apps/server/cmd/snarvei/main_test.go`
- Create: `apps/server/internal/api/testhooks.go`, `apps/server/internal/api/testhooks_test.go`

**Interfaces:**
- Consumes: `config.Config` (all fields), every package above.
- Produces: `buildDeps(ctx, cfg) (api.Deps, func(), error)`; JSON logging via `slog`; `GET /api/_test/mail` (only with `E2E_TEST_HOOKS=1`) returning `{"messages":[{"to","subject","text"}]}` newest first, and `DELETE /api/_test/mail` clearing it.

- [ ] **Step 1: Write `testhooks.go` and its test**

```go
package api

import (
	"net/http"

	"github.com/refsdal/snarvei/server/internal/api/respond"
)

// mountTestHooks exposes the recorded mailbox for the Playwright suite.
// Only mounted when Deps.TestHooks is true (E2E_TEST_HOOKS=1, loopback
// APP_URL only, enforced by config).
func (d Deps) mountTestHooks(mux *http.ServeMux) {
	if d.Mail == nil {
		panic("api: TestHooks requires a recording mailer")
	}
	mux.HandleFunc("GET /api/_test/mail", func(w http.ResponseWriter, r *http.Request) {
		msgs := d.Mail.Messages()
		out := make([]map[string]string, 0, len(msgs))
		for i := len(msgs) - 1; i >= 0; i-- {
			out = append(out, map[string]string{"to": msgs[i].To, "subject": msgs[i].Subject, "text": msgs[i].Text})
		}
		respond.JSON(w, http.StatusOK, map[string]any{"messages": out})
	})
	mux.HandleFunc("DELETE /api/_test/mail", func(w http.ResponseWriter, r *http.Request) {
		d.Mail.Reset()
		w.WriteHeader(http.StatusNoContent)
	})
}
```

Remove the no-op `mountTestHooks` from `organizations.go`. Test (`testhooks_test.go`):

```go
package api_test

import (
	"net/http"
	"testing"

	"github.com/refsdal/snarvei/server/internal/testrig"
)

func TestMailHookListsNewestFirstAndClears(t *testing.T) {
	a := testrig.App(t)
	orgID, owner := a.NewOrg("Acme", "acme", "owner@example.com")
	a.Do(http.MethodPost, "/api/organizations/"+orgID+"/invitations", map[string]string{"email": "a@example.com", "role": "member"}, owner)
	a.Do(http.MethodPost, "/api/organizations/"+orgID+"/invitations", map[string]string{"email": "b@example.com", "role": "member"}, owner)
	resp := a.Do(http.MethodGet, "/api/_test/mail", nil, "")
	msgs := resp.JSON["messages"].([]any)
	if resp.Code != 200 || len(msgs) != 2 || msgs[0].(map[string]any)["to"] != "b@example.com" {
		t.Fatalf("mail hook: %d %s", resp.Code, resp.Body)
	}
	if a.Do(http.MethodDelete, "/api/_test/mail", nil, "").Code != 204 {
		t.Fatal("clear")
	}
	if resp := a.Do(http.MethodGet, "/api/_test/mail", nil, ""); len(resp.JSON["messages"].([]any)) != 0 {
		t.Fatal("not cleared")
	}
}

func TestMailHookAbsentWithoutTestHooks(t *testing.T) {
	a := testrig.App(t)
	deps := a.Deps
	deps.TestHooks = false
	a.Handler = apiNewHandler(deps)
	if resp := a.Do(http.MethodGet, "/api/_test/mail", nil, ""); resp.Code != 404 {
		t.Fatalf("hook mounted without TestHooks: %d", resp.Code)
	}
}
```

with `func apiNewHandler(d api.Deps) http.Handler { return api.NewHandler(d) }` and the `api` import added at the top of the test file.

- [ ] **Step 2: Rewrite `serveMode`/`serve` in `main.go` to build the full Deps**

Replace the pool-and-Deps block inside `serve` with a call to `buildDeps`, and add `buildDeps`, `buildStorage`, `buildEmail` and the slog setup:

```go
// in run(), before dispatching: configure JSON logging once.
func setupLogging(level string) {
	var lvl slog.Level
	_ = lvl.UnmarshalText([]byte(strings.ToUpper(level)))
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})))
}

// buildDeps assembles every collaborator from validated config. The only
// place in the program that constructs a client.
func buildDeps(ctx context.Context, cfg *config.Config) (api.Deps, func(), error) {
	pool, err := db.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return api.Deps{}, nil, err
	}
	closePool := func() { pool.Close() }

	q := dbgen.New(pool)
	hasher := clientip.NewHasher(cfg.IPHashPepper, cfg.AuthSecret)
	sender, mailbox := buildEmail(cfg)
	authService, err := auth.New(auth.Config{
		AppURL: cfg.AppURL, AppName: cfg.AppName, Secret: cfg.AuthSecret, OpenSignup: cfg.OpenSignup,
		Pool: pool, ClientIP: hasher.Extractor(cfg.TrustedProxyHops), Email: sender, Log: slog.Default(),
	})
	if err != nil {
		closePool()
		return api.Deps{}, nil, err
	}
	store, err := buildStorage(cfg)
	if err != nil {
		closePool()
		return api.Deps{}, nil, err
	}
	return api.Deps{
		Pool: pool, Q: q, Auth: authService, Storage: store, Email: sender, Mail: mailbox,
		RateLimit: ratelimit.NewPostgres(q), Hasher: hasher, Log: slog.Default(),
		AppURL: cfg.AppURL, AppName: cfg.AppName, OpenSignup: cfg.OpenSignup, Version: version,
		TrustedProxyHops: cfg.TrustedProxyHops, TestHooks: cfg.E2ETestHooks,
	}, closePool, nil
}

func buildStorage(cfg *config.Config) (storage.Storage, error) {
	switch cfg.StorageDriver {
	case "fs":
		return storage.NewFS(cfg.StorageFSPath)
	case "s3":
		return storage.NewS3(storage.S3Config{Bucket: cfg.S3Bucket, Endpoint: cfg.S3Endpoint, AccessKeyID: cfg.S3AccessKeyID, SecretAccessKey: cfg.S3SecretAccessKey, Region: cfg.S3Region}), nil
	}
	return nil, fmt.Errorf("unknown storage driver %q", cfg.StorageDriver)
}

// buildEmail picks SMTP when configured; with test hooks on, mail is captured
// in memory for the e2e suite instead (the hook endpoint reads it back).
func buildEmail(cfg *config.Config) (email.Sender, *email.Recording) {
	if cfg.E2ETestHooks {
		rec := email.NewRecording()
		return rec, rec
	}
	if cfg.EmailEnabled() {
		return email.NewSMTP(email.SMTPConfig{Host: cfg.SMTPHost, Port: cfg.SMTPPort, Username: cfg.SMTPUsername, Password: cfg.SMTPPassword, From: cfg.EmailFrom}), nil
	}
	return email.NewNoop(slog.Default()), nil
}
```

`serve` becomes: config → migrate (if asked) → `deps, closeDeps, err := buildDeps(context.Background(), cfg)` → `defer closeDeps()` → server as before with `web.Handler(api.NewHandler(deps))`. Boot log lines (via `slog.Info`): `"snarvei listening"` with `version`, `port`, `app_url`, `storage`, `disabled` (from `cfg.DisabledSubsystems()`), plus `"test hooks enabled"` when on. `run` calls `setupLogging` after config loads in `serveMode`/`migrateMode` (config carries `LogLevel`); `healthcheckMode` and the unknown-mode path keep plain stderr output.

Update `TestServeLifecycle` in `main_test.go`: the `config.Load` map must now include everything `buildDeps` needs (it already has `STORAGE_DRIVER=fs` and a temp `STORAGE_FS_PATH`). The test's fixed `serve(cfg, false, sig)` signature stays.

- [ ] **Step 3: Verify**

```bash
cd apps/server && mise exec -- go vet ./... && mise exec -- go test -p 1 -count=1 ./... 2>&1 | tail -15
DATABASE_URL=postgres://snarvei:snarvei@127.0.0.1:55432/snarvei_test APP_URL=http://localhost:3000 AUTH_SECRET=local-dev-secret-at-least-32-bytes-long STORAGE_DRIVER=fs STORAGE_FS_PATH=/tmp E2E_TEST_HOOKS=1 mise exec -- go run ./cmd/snarvei &
sleep 3
curl -s -X POST localhost:3000/api/auth/signup/credential -H 'content-type: application/json' -H 'origin: http://localhost:3000' -d '{"name":"Dev","email":"dev@example.com","password":"Devpass123"}' -c /tmp/c.txt | head -c 200; echo
curl -s -b /tmp/c.txt localhost:3000/api/me; echo
curl -s -b /tmp/c.txt -X POST localhost:3000/api/organizations -H 'content-type: application/json' -H 'origin: http://localhost:3000' -d '{"name":"Dev Org","slug":"dev-org"}'; echo
curl -s localhost:3000/api/_test/mail; echo
kill %1
```

Expected: suite green; sign-up sets a cookie and `/api/me` shows the user; the organization is created with role owner; the mail hook answers `{"messages":[]}`; the boot log is JSON lines.

- [ ] **Step 4: Commit**

```bash
git add apps/server
git commit -m "feat(server): compose auth, storage, email and rate limiting; JSON logging; e2e mail hook"
```

---

### Task 12: Playwright API flows, CI probe, docs note, pull request

**Files:**
- Create: `e2e/auth-api.spec.ts`
- Modify: `.github/workflows/ci.yml` (smoke: sign-in probe), `scripts/e2e-stack.sh` (`E2E_TEST_HOOKS=1`), `AGENTS.md` (banner line)

- [ ] **Step 1: Turn the test hooks on in the e2e stack**

In `scripts/e2e-stack.sh` add `-e E2E_TEST_HOOKS=1 \` to the app `docker run` (after `-e OPEN_SIGNUP=1`). `APP_URL` there is `http://127.0.0.1:3300`, a loopback origin, so config accepts it.

- [ ] **Step 2: Write `e2e/auth-api.spec.ts`**

These drive the real image through Playwright's request context (the SPA still uses the old auth client until phase 4, so browser flows come then).

```ts
import { expect, test, type APIRequestContext } from "@playwright/test";

const unique = () => Math.random().toString(36).slice(2, 10);
const PASSWORD = "Playwright123";
const ORIGIN = process.env.E2E_BASE_URL ?? "http://127.0.0.1:3300";
const headers = { origin: ORIGIN, "content-type": "application/json" };

async function signUp(request: APIRequestContext, name: string, email: string) {
  const res = await request.post("/api/auth/signup/credential", { headers, data: { name, email, password: PASSWORD } });
  expect(res.status(), await res.text()).toBe(200);
}

async function signIn(request: APIRequestContext, email: string) {
  const res = await request.post("/api/auth/signin/credential", { headers, data: { credential: email, password: PASSWORD } });
  expect(res.status(), await res.text()).toBe(200);
}

test("sign up, sign in, profile", async ({ request }) => {
  const email = `kari-${unique()}@example.com`;
  await signUp(request, "Kari", email);
  const me = await request.get("/api/me");
  expect(me.status()).toBe(200);
  const body = await me.json();
  expect(body.user).toMatchObject({ name: "Kari", email, image: null, twoFactorEnabled: false });
  expect(JSON.stringify(body)).not.toContain("token");

  const bad = await request.post("/api/auth/signin/credential", { headers, data: { credential: email, password: "wrong" } });
  expect(bad.status()).toBe(401);

  const patched = await request.patch("/api/me", { headers, data: { name: "Kari Nordmann" } });
  expect((await patched.json()).user.name).toBe("Kari Nordmann");
});

test("organization, team, invitation with team, registration through the invitation", async ({ request, playwright }) => {
  const owner = `owner-${unique()}@example.com`;
  await signUp(request, "Owner", owner);
  const org = await request.post("/api/organizations", { headers, data: { name: "Acme", slug: `acme-${unique()}` } });
  expect(org.status(), await org.text()).toBe(201);
  const orgId = (await org.json()).id;
  expect((await request.post(`/api/organizations/${orgId}/switch`, { headers })).status()).toBe(204);

  const team = await request.post(`/api/organizations/${orgId}/teams`, { headers, data: { name: "Marketing" } });
  expect(team.status()).toBe(201);
  const teamId = (await team.json()).id;

  await request.delete("/api/_test/mail");
  const invitee = `new-${unique()}@example.com`;
  const inv = await request.post(`/api/organizations/${orgId}/invitations`, { headers, data: { email: invitee, role: "member", teamId } });
  expect(inv.status(), await inv.text()).toBe(201);
  const invitationId = (await inv.json()).id;

  const mail = await (await request.get("/api/_test/mail")).json();
  expect(mail.messages[0].to).toBe(invitee);
  expect(mail.messages[0].text).toContain(`/app/invitations/${invitationId}`);

  // A second, anonymous context plays the invitee.
  const guest = await playwright.request.newContext({ baseURL: ORIGIN });
  const pub = await guest.get(`/api/invitations/${invitationId}`);
  expect(await pub.json()).toMatchObject({ organizationName: "Acme", teamName: "Marketing", role: "member", hasAccount: false });
  expect(await pub.text()).not.toContain(invitee);

  const reg = await guest.post(`/api/invitations/${invitationId}/register`, { headers, data: { name: "New Person", password: PASSWORD } });
  expect(reg.status(), await reg.text()).toBe(201);
  expect(reg.headers()["set-cookie"]).toContain("snarvei_session=");
  const regBody = await reg.json();
  expect(regBody.session.activeOrganizationId).toBe(orgId);

  const teams = await guest.get(`/api/organizations/${orgId}/teams`);
  expect((await teams.json()).map((t: { id: string }) => t.id)).toEqual([teamId]);
  const members = await guest.get(`/api/teams/${teamId}/members`);
  expect((await members.json()).map((m: { email: string }) => m.email)).toEqual([invitee]);

  // A member cannot invite or create teams.
  expect((await guest.post(`/api/organizations/${orgId}/teams`, { headers, data: { name: "Nope" } })).status()).toBe(403);
  expect((await guest.post(`/api/organizations/${orgId}/invitations`, { headers, data: { email: "x@example.com", role: "member" } })).status()).toBe(403);
  await guest.dispose();
});

test("existing account accepts an invitation; strangers are refused", async ({ request, playwright }) => {
  const owner = `owner-${unique()}@example.com`;
  await signUp(request, "Owner", owner);
  const orgId = (await (await request.post("/api/organizations", { headers, data: { name: "Beta", slug: `beta-${unique()}` } })).json()).id;
  await request.post(`/api/organizations/${orgId}/switch`, { headers });

  const existing = `existing-${unique()}@example.com`;
  const invitee = await playwright.request.newContext({ baseURL: ORIGIN });
  await signUp(invitee, "Existing", existing);
  const stranger = await playwright.request.newContext({ baseURL: ORIGIN });
  await signUp(stranger, "Stranger", `stranger-${unique()}@example.com`);

  const inv = await request.post(`/api/organizations/${orgId}/invitations`, { headers, data: { email: existing, role: "admin" } });
  const invitationId = (await inv.json()).id;

  expect((await stranger.post(`/api/invitations/${invitationId}/accept`, { headers })).status()).toBe(403);
  expect((await stranger.get(`/api/organizations/${orgId}/members`)).status()).toBe(403);

  const accepted = await invitee.post(`/api/invitations/${invitationId}/accept`, { headers });
  expect(accepted.status(), await accepted.text()).toBe(200);
  expect(await accepted.json()).toMatchObject({ id: orgId, role: "admin" });
  const members = await invitee.get(`/api/organizations/${orgId}/members`);
  expect((await members.json()).map((m: { role: string }) => m.role).sort()).toEqual(["admin", "owner"]);
  await invitee.dispose();
  await stranger.dispose();
});

test("password reset flow through the mailbox", async ({ request, playwright }) => {
  const email = `reset-${unique()}@example.com`;
  await signUp(request, "Reset Me", email);
  await request.delete("/api/_test/mail");
  const anon = await playwright.request.newContext({ baseURL: ORIGIN });
  expect((await anon.post("/api/auth/passwords/request-reset", { headers, data: { email } })).status()).toBe(200);
  const mail = await (await anon.get("/api/_test/mail")).json();
  const token = /token=([A-Za-z0-9._-]+)/.exec(mail.messages[0].text)?.[1];
  expect(token).toBeTruthy();
  expect((await anon.post("/api/auth/passwords/reset", { headers, data: { token, new_password: "Changed456" } })).status()).toBe(200);
  expect((await request.get("/api/me")).status()).toBe(401); // old session revoked
  const signin = await anon.post("/api/auth/signin/credential", { headers, data: { credential: email, password: "Changed456" } });
  expect(signin.status()).toBe(200);
  await anon.dispose();
});
```

- [ ] **Step 3: Add the sign-in probe to the CI smoke test**

In `.github/workflows/ci.yml`'s smoke step, after the `/api/nope` check and before the healthcheck line, add:

```bash
          # A sign-in with unknown credentials must answer 401: Limen loaded,
          # its Postgres adapter resolved, and a query ran end to end.
          status=$(curl -s -o /dev/null -w '%{http_code}' \
            -X POST http://localhost:3000/api/auth/signin/credential \
            -H 'content-type: application/json' -H 'origin: http://localhost:3000' \
            -d '{"credential":"nobody@example.invalid","password":"wrong-password"}')
          test "$status" = "401" || { echo "sign-in probe returned $status, expected 401"; exit 1; }
```

- [ ] **Step 4: Run everything locally**

```bash
bun run check && bun run test
(cd apps/server && mise exec -- go vet ./... && mise exec -- go test -p 1 -count=1 ./...)
E2E_REBUILD=1 mise run e2e 2>&1 | tail -15
git status --short
```

Expected: Go suite green; Playwright reports 11 passed (7 smoke + 4 auth-api); tree clean apart from intended changes.

- [ ] **Step 5: Docs line, commit, PR**

Add to the banner paragraph at the top of `AGENTS.md`: `Phase 2 (auth, organizations, teams, invitations, profile, email, storage) is implemented; see the OpenAPI spec for the live routes.`

```bash
git add e2e scripts/e2e-stack.sh .github/workflows/ci.yml AGENTS.md
git commit -m "test(e2e): API-level Playwright flows for auth, organizations, teams and invitations; CI sign-in probe"
git push -u origin HEAD
gh pr create --base main --title "feat: auth, organizations, teams and invitations on Limen (phase 2)" --body-file - <<'EOF'
Phase 2 of the Go migration (spec section 11). Design: docs/superpowers/specs/2026-09-04-go-backend-migration-design.md; plan: docs/superpowers/plans/2026-09-05-go-migration-phase-2-auth-tenancy.md.

- `internal/auth`: Limen (credential-password, two-factor, organization) confined behind `auth.Service` with a route allowlist; sessions, password reset mail, invitations mail.
- `/api/me`: profile, profile image (fs/s3 storage port), sessions list/revoke, email change with confirmation token, account deletion with last-owner guard.
- Organizations (create/list/switch/members), invitations (create with optional team, list, cancel, public view, accept/reject, register-through-invitation with rate limiting), teams and team membership with owner/admin/member rules from `internal/authz`.
- Middleware for session, organization, team, trusted proxy and rate limiting; keyed IP hashing (no raw IPs stored); SMTP email with no-op fallback; JSON logging.
- Tests: Go suites per package against real Postgres; Playwright API-level flows against the image (sign-up/in, org+team+invite+register, accept/refuse, password reset); CI sign-in probe.

Not in this PR: links, redirect, analytics (phase 3); the SPA port (phase 4, the SPA still calls the old auth client); release pipeline and docs rewrite (phase 5).

🤖 Generated with [Claude Code](https://claude.com/claude-code)

https://claude.ai/code/session_01UdGgRFBUoiwkd9PLH7zUJE
EOF
gh pr checks --watch
```

Expected: both CI jobs green.

---

## Self-review

**Spec coverage (phase 2 items in section 11 and the sections they cite):** `internal/auth` with allowlist (T5, constraint list = spec section 3 allowlist); `/api/me` family incl. sessions, email change, deletion (T9; spec section 2 Me); organizations, teams, invitations incl. team attachment and register (T10; spec section 2 Organizations/Teams/Invitations); `internal/email` SMTP + templates (T2; spec sections 3 and 7); `storage` with profile images (T1, T9); `authz` (T3); client IP + hashing + `CF-IPCountry` (T1; section 3); rate limiter with the 30/min write rule (T1, T6, T7); config values already exist from phase 1; test hooks (T11; section 8/9); sqlc drift guard (T1; section 9); Playwright flows (T12).

**Deviations from the spec, decided here:** (1) organization listing and member roles are sqlc queries, not `auth.Service` methods (section 3's interface sketch); (2) accept-invitation adds the team membership right after Limen's own write rather than in one transaction (Limen owns its transaction); the insert is idempotent; (3) `GET /api/me/sessions` is Snarvei's own SQL over Limen's `sessions` table, reading the user agent from Limen's metadata JSON; (4) two-factor routes are allowlisted but have no Go tests beyond reachability (the flow is exercised in phase 4's UI tests); (5) `GET /api/teams/{teamId}/members` is `tierTeam`, not owner/admin only: any member of the team may list their teammates (spec updated to match).

**Placeholder scan:** no TBD/TODO; the only "adapt to generated names" notes point at concrete files (`types.gen.go`, sqlc output) and name the fields.

**Type consistency:** `auth.Service` methods used in T6, T8, T9, T10 match T5's interface (`CreateUser(ctx,name,email,password)`, `AcceptInvitation(ctx,userID,invitationID) (*Invitation, error)`, `SetActiveOrganization(ctx,token,orgID)`, `StartSession(ctx,w,r,userID)`, `DeleteUser`, `RevokeSession(ctx,token)`); `middleware.Deps{Auth,Q,RateLimit,Hasher,TrustedProxyHops}` matches `api.Deps.mwDeps()`; `ratelimit.Store.Hit` returns `(count, retryAfter, err)` in T1 and T6; `email.Recording.Last/Messages/Reset` used in T5, T9, T11; `testrig.AppRig.Do` returns `Response{Code,Header,Body,JSON,Array}` as used in T9/T10; sqlc method names in T4 match their call sites in T5, T6, T9, T10.
