# AGENTS.md

This file is the execution brief for future AI agents and contributors
working on `snarvei` — the contributor deep-dive; `README.md` is for users.

## Product Summary

Snarvei is an organization-aware URL shortener and redirect management
system.

Core goals:

1. Create short links under a shared short domain.
2. Redirect users through `GET /l/{slug}`.
3. Allow redirect targets to be changed after the short URL has already
   been distributed.
4. Track click analytics per link.
5. Support multiple organizations with invitations and organization
   membership.
6. Use Teams as the only internal permission boundary in V1.

## Locked Decisions

These decisions are made and should be treated as defaults unless the user
explicitly changes them. They come from
`docs/superpowers/specs/2026-09-04-go-backend-migration-design.md`
("Decisions taken during brainstorming").

| Topic | Decision |
| --- | --- |
| Backend | Go, stdlib `net/http`, no framework. One static, CGO-free binary. |
| Database | Postgres via pgx v5 + sqlc + goose. No SQLite driver. |
| Auth | [Limen](https://github.com/thecodearcher/limen), confined behind `internal/auth`. Teams are Snarvei's own tables, not a Limen plugin. |
| Frontend | React + TanStack Router/Query, an `openapi-fetch` client generated from the spec, `limen-auth` for the auth client. |
| JS toolchain | bun + biome. No pnpm, ESLint or Prettier. |
| Email | SMTP. Absent configuration drops mail with a redacted log line, never a stub inbox. |
| Storage | A `storage.Storage` port with `fs` and `s3` drivers for profile images. |
| Image | Distroless `static` (nonroot), built natively — nothing compiles inside Docker. |
| Releases | Merging to `main` is releasing: svu computes the version from Conventional Commits, GoReleaser publishes. |
| License | AGPL-3.0-only. |
| Networking | Behind a trusted proxy via `TRUSTED_PROXY_HOPS`; `X-Forwarded-For` is never trusted at hop 0. |

## The Deps Rule

`internal/api` never reads the environment or constructs a collaborator.
Every handler is a method on `api.Deps` (`apps/server/internal/api/api.go`),
a plain struct holding the pool, the sqlc queries, the auth service,
storage, email, the rate limiter, the IP hasher and the click recorder.
`cmd/snarvei` builds one `Deps` at startup and hands it to
`api.NewHandler(deps)`, which panics immediately if a required collaborator
is `nil` — a missing dependency is a boot-time crash, not a runtime `nil`
pointer three requests later. Tests build a `Deps` the same way, through
`internal/testrig`, against a real Postgres database rather than a mock.

## The Limen Boundary

`internal/auth` is the only package that imports Limen. It exposes a small
`Service` interface (session lookup, organizations, invitations, member
roles, password verification) — nothing else in the module names a Limen
type.

Limen's own HTTP surface is an allowlist, not a blocklist:
`apps/server/internal/auth/routes.go` lists every route id the pinned
plugins can mount (`knownRouteIDs`) and computes which ones stay enabled
(`allowedRouteIDs`); everything else is passed to
`limen.WithHTTPDisabledPaths`. A test probes the concrete paths so a route
added upstream can never become silently reachable. Disabled on purpose:
session listing/revocation and every organization/invitation route (Snarvei
re-implements these as its own API so invitations can carry a team and
authorization stays in one place), `usernames-check`, `passwords-set`, and
email verification.

Sign-up is gated by `OPEN_SIGNUP`; when it is `0`,
`POST /api/auth/signup/credential` is disabled and the only way to create an
account is `POST /api/invitations/{id}/register`, which is itself
rate-limited and marked `tierPublicCapture` (public, but sets a session
cookie on success). Limen's own database-backed rate limiter on
`/api/auth/*` is, in practice, in-memory per replica for now (its Postgres
store cannot hold pgx's int64 counters as-is); `/l/*` and a handful of write
endpoints (`POST /api/invitations/{id}/register`, `POST /api/me/email`,
`POST /api/links`) go through Snarvei's own Postgres-backed
`internal/ratelimit` instead, which is safe across replicas.

## Spec-First Workflow

`openapi/snarvei.yaml` is hand-written and authoritative. Editing it means
running, from the repo root:

```sh
cd apps/server && go generate ./...   # oapi-codegen: strict server + types; copies the spec for go:embed
bun run gen:client                    # openapi-typescript: apps/frontend/src/lib/api-schema.d.ts
```

Both outputs are committed and never produced in CI or in the image. A Go
test regenerates into a temp directory and diffs it against the committed
`internal/api/gen` output (and the embedded spec copy); a bun test does the
same for `api-schema.d.ts`. Drift fails the suite.

Every spec operation must also appear in `operationTiers`
(`apps/server/internal/api/tiers.go`), which names the middleware chain
(public, session, org, org-admin, team, team-admin, or one of the
rate-limited variants) the operation runs behind. `assertTierCoverage` walks
the spec at `NewHandler` time and panics on any operation missing an entry —
a new endpoint can never ship unguarded by accident.

## Migrations Under Goose

Goose migrations live in `apps/server/internal/db/migrations`, embedded in
the binary, and are applied either by the default dispatch mode (under
Postgres advisory lock `MIGRATION_LOCK_KEY`) or by `snarvei migrate`.
Migrations are forward-only and follow expand/contract: add first, ship code
that works with both shapes, drop only once nothing depends on the old one.
sqlc queries live in `apps/server/internal/db/queries/*.sql`; `go generate`
regenerates `internal/db/gen`, which is committed and drift-guarded the same
way as the API code.

## Authorization Rules

Keep authorization centralized in `internal/authz` (pure functions, no I/O,
unit-tested) — never spread ad hoc across handlers.

1. Org `owner` and `admin` can see and mutate every Team and Link in the
   organization.
2. Org `member` can see and mutate only Links in Teams they belong to.
3. A Link mutation always checks both the caller's org membership and the
   Team's ownership of the Link.
4. Only `owner`/`admin` create Teams, manage Team membership, invite, or
   cancel invitations.
5. An owner cannot be demoted or removed by this API — there is no role
   management endpoint in V1.

Every sqlc query on `links`, `link_target_history`, `click_events`, `teams`
and `team_members` scopes on `organization_id`, directly or through a join
on the link's or team's org, so cross-org access is structurally impossible
rather than left to a handler to remember.

## Redirect and Analytics Privacy

`GET /l/{slug}` runs outside the session middleware: rate limit, look up the
active link, respond with the redirect (`Cache-Control: no-store`), then
hand a click event to an async recorder. A click row stores a keyed IP hash
(never a raw address), `utm_*` query parameters only, the referer reduced to
origin + path, and a user agent capped at 256 characters — never the full
query string, fragment or credentials. On shutdown the server stops
accepting, drains in-flight requests, then waits up to 5 seconds for the
recorder; a click can be lost on a hard kill, which matches the previous
`waitUntil` guarantee.

## Frontend Conventions

- Session truth is the `['me']` query (`apps/frontend/src/lib/data/keys.ts`
  is the single place every query key is defined:
  `['config']`, `['organizations']`, `['teams', orgId]`,
  `['teamMembers', teamId]`, `['members', orgId]`, `['invitations', orgId]`,
  `['invitation', id]`, `['links', orgId, filters]`, `['link', id]`,
  `['history', id, page]`, `['analytics', id, days]`, `['sessions']`).
  Mutations invalidate the affected keys.
- Routing is code-based and typed (`apps/frontend/src/router.tsx`, one file,
  one route tree). Pages read their own route's params/search through
  `getRouteApi`, and cross-route navigation goes through the typed helpers
  in `src/lib/routes.ts` (`orgParams` and friends) rather than hand-built
  path strings.
- `data-testid` attributes are a contract with `e2e/`: renaming or removing
  one without checking the Playwright specs breaks CI, not just a lint rule.

## Testing Expectations

- Go (`cd apps/server && go test -p 1 ./...`): against a real Postgres from
  `TEST_DATABASE_URL` (`docker-compose.test.yml`, port 55432), never a mock.
  `-p 1` is not optional — several packages share and truncate domain tables
  between tests and cannot run as concurrent packages against one database.
  `internal/testrig` applies migrations once per run and truncates between
  tests.
- Frontend: `bun run test` for route helpers and pure logic; `bun run check`
  for biome + `tsc --noEmit` + the `api-schema.d.ts` drift guard.
- Playwright (`e2e/`, `mise run e2e` or `bun run test:e2e`): runs against the
  real container image via `scripts/e2e-stack.sh`, with `OPEN_SIGNUP=1` and
  no `SMTP_*` configured, so mail is captured in memory and read back
  through `GET /api/_test/mail` (`DELETE` clears it) — enabled by
  `E2E_TEST_HOOKS=1`, refused unless `APP_URL` is a loopback origin, never
  turn this on anywhere real.

Run the commands that cover what changed before committing; do not rely on
CI to catch an avoidable regression.

## Release Model and Conventional Commits

Commit types decide the version: `feat` bumps the minor, `fix`/`perf` the
patch, `!`/`BREAKING CHANGE` the major (kept at 0 by `--v0` until a human
opts into 1.0). svu computes the version from commits since the last `v*`
tag; GoReleaser (`.goreleaser.yaml`) builds and publishes everything
downstream of the tag. See `README.md` → "Versioning and images" for the
full pipeline. Commit trailer conventions belong to whatever tooling is
producing a given commit, not to this file — AGENTS.md does not mandate
trailers.

## Operations Pointers

`docs/runbook.md` is the operational source of truth: where the container
runs, configuration and secrets, deploy/verify, rollback, common failures
and their log events. Update it in the same change as any behaviour it
describes.
