# Snarvei operations runbook

Single-operator runbook for wherever the container runs. Keep it short and
accurate; update it in the same change as the behaviour it describes.

## Where it runs

Snarvei ships as one image, `ghcr.io/refsdal/snarvei`, tagged
`:0.1.0` (never moves), `:0.1` (patches), `:0` (minors), `:latest` and
`:sha-<commit>` — see `README.md` → "Self-hosting" for the full ladder. It is
one process: a single static Go binary that serves the API, the redirect and
the SPA together, behind whatever container runtime and reverse proxy the
operator already has. There is no separate worker, cron or landing mode —
Snarvei has no scheduled work.

- `GET /healthz` — liveness. Constructs nothing, touches nothing (a slow
  database must never turn into a restart loop). `200 {"ok":true,"service":"snarvei","version":"<tag>"}`.
- `GET /readyz` — readiness. Runs `SELECT 1` against Postgres. `200
  {"ok":true}` or `503 {"ok":false,"error":"..."}`. Both probes send
  `Cache-Control: no-store`.
- `PORT` (default `3000`) is the only thing that changes what port the
  process listens on; there is no separate metrics or admin port.

## Configuration and secrets

Every setting is an environment variable, validated once at startup with
every problem reported at once — a misconfigured container crash-loops with
a list, not one restart per mistake. [`.env.example`](../.env.example) is
the complete contract; `README.md` → "Configuration" has the same table with
defaults and notes.

Rotation: **rotating `AUTH_SECRET` signs everyone out** (it signs sessions)
**and changes every click-analytics IP hash** unless a dedicated
`IP_HASH_PEPPER` is also set — set `IP_HASH_PEPPER` before the first
rotation if stable hashes across a secret rotation matter to you.

## Deploy and verify

1. Migrate, then point the deployment at the new tag. A single instance can
   rely on the default dispatch mode, which migrates itself under a
   Postgres advisory lock before it starts serving. With more than one
   replica, run the migration as an explicit one-off first:

   ```sh
   docker run --rm -e DATABASE_URL=... [other required vars] \
     ghcr.io/refsdal/snarvei:<tag> migrate
   ```

2. Verify:

   ```sh
   curl -s https://<host>/healthz   # {"ok":true,"service":"snarvei","version":"<tag>"}
   ```

   Confirm `version` matches the tag just deployed. Open `/scalar` and
   confirm the API reference loads. Follow a real short link
   (`/l/<slug>`) and confirm a `302` (or the link's configured status) with
   `Cache-Control: no-store`.

## Rollback

- Point the deployment back at the previous image tag. **Migrations are
  forward-only** — there is no down migration to run, so a code rollback
  does not undo a schema change. Plan schema changes expand/contract
  (`AGENTS.md` → "Migrations Under Goose") precisely so an old binary keeps
  working against a newer schema during a rollback window.
- Postgres backup and restore is the operator's responsibility — an
  ordinary `pg_dump`/`pg_basebackup` schedule against whatever runs the
  database. Snarvei does not manage or trigger backups itself.
- Profile images live on the `snarvei-data` volume (or the configured S3
  bucket). Back it up alongside Postgres — restoring one without the other
  leaves profile images pointing at files that no longer exist, or a
  database with no matching images.

## Common failures

| Symptom | Log event | Action |
| --- | --- | --- |
| Container exits immediately with a list of problems | (stderr, before logging starts) | A required variable is missing or invalid; every problem is printed at once — fix them all, then restart |
| `/readyz` returns 503 | (the `{"ok":false}` body, no dedicated log event) | Postgres unreachable or broken — check the database, its network path, and its own logs |
| Invitations / reset / change-email mail never arrives | `email.not_configured` (all five `SMTP_*`/`EMAIL_FROM` not set) or `email.send_failed` (provider rejected it) | Set all five SMTP variables together, or check the SMTP provider's error in the log line |
| Users get `429` | (a `Retry-After` header on the response) | Expected under abuse. `/l/*` and a handful of write endpoints are rate-limited per hashed IP by Snarvei's own Postgres-backed limiter, safe across replicas; `/api/auth/*` sign-in/sign-up/two-factor limits are Limen's own and — for now — in-memory per replica, so the effective limit scales with replica count |
| Redirect works but no click recorded | `click.record_failed` | The async click write failed (link id and slug are in the log line) — check Postgres health and capacity |
| Server logs `click.drain_timeout` on shutdown | `click.drain_timeout` | The in-flight click recorder did not finish within its 5 s shutdown budget — a handful of clicks near a restart can be lost, matching the previous best-effort guarantee; frequent occurrences point at a slow or overloaded database |
| Unexpected `500` on an API route | `request.error` (has `requestId`, `userId`, `path`) | Inspect the stack in the container logs; correlate with the `version` reported by `/healthz` |

## Where to look

- Container logs: one JSON line per event on stdout (`time`, `level`,
  `event`, plus `requestId`/`userId`/`path`/`status`/`durationMs` where
  relevant). Filter by `event` for any of the names above.
- `/healthz` reports the running `version` — compare it against the tag you
  expect to be deployed.
- GitHub Actions (`.github/workflows/release.yml`) for the release history:
  which commits shipped in which version, and the GitHub Release page for
  the changelog, checksums and cosign signatures.

## Local development quickstart

```sh
mise install
bun install
docker compose -f docker-compose.test.yml up -d --wait   # Postgres on 127.0.0.1:55432, db snarvei_test

DATABASE_URL="postgres://snarvei:snarvei@localhost:55432/snarvei_test?sslmode=disable" \
APP_URL=http://localhost:5173 \
AUTH_SECRET="$(openssl rand -base64 32)" \
OPEN_SIGNUP=1 \
STORAGE_DRIVER=fs STORAGE_FS_PATH=/tmp/snarvei-dev \
bun run dev:server    # go run ./cmd/snarvei, :3000

bun run dev            # Vite, :5173, proxies the server-owned paths to :3000

mise run test   # Go (real Postgres) + frontend unit tests
mise run check  # lint, typecheck, goreleaser config
mise run e2e    # Playwright against the real image
```

See `README.md` → "Development" for the full explanation, and
`CONTRIBUTING.md` for the contributor workflow.
