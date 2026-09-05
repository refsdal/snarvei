# Contributing

Thanks for considering it. Reading this first will save you a rejected PR.

## Ground rules

- **Discuss features before building them.** Open an issue first — see
  `AGENTS.md` for the locked product and architecture decisions, and the
  design spec under `docs/superpowers/specs/` for the reasoning behind them.
  Bug fixes can go straight to a PR.
- **Conventional Commits are load-bearing.** `feat:`/`fix:`/`perf:` decide
  the version bump and the changelog; merging to `main` releases
  automatically. A mislabeled commit ships a mislabeled release.
- **Generated code is committed.** After touching `openapi/snarvei.yaml` or
  `apps/server/internal/db/queries/*.sql`, run `go generate ./...` (from
  `apps/server`) and `bun run gen:client` (from the root), and commit the
  output. Drift in the Go-generated code (`internal/api/gen`, the embedded
  spec, `internal/db/gen`) fails `go test`. Drift in the generated client
  (`apps/frontend/src/lib/api-schema.d.ts`) has no local test — it is only
  caught by CI's "Generated API client is current" step — so run
  `bun run gen:client` and check `git diff` yourself before committing.

## Getting set up

```sh
mise install                                          # pinned Go, Bun + codegen tools
bun install
docker compose -f docker-compose.test.yml up -d --wait # Postgres on :55432, for tests and dev

mise run test    # the full suite: Go against real Postgres + frontend
mise run check   # lint, typecheck, goreleaser config
mise run e2e     # Playwright against the real container image (needs Docker)
```

## Running locally

```sh
DATABASE_URL="postgres://snarvei:snarvei@localhost:55432/snarvei_test?sslmode=disable" \
APP_URL=http://localhost:5173 \
AUTH_SECRET="$(openssl rand -base64 32)" \
OPEN_SIGNUP=1 \
STORAGE_DRIVER=fs STORAGE_FS_PATH=/tmp/snarvei-dev \
bun run dev:server    # go run ./cmd/snarvei, :3000

bun run dev            # Vite, :5173, proxying /api /l /healthz /readyz /openapi.json /scalar /images /robots.txt to :3000
```

There is no seed script. `OPEN_SIGNUP=1` lets the "Create account" form on
the landing page bootstrap your first account — the same path a self-hoster
takes, so it is the path that stays tested.

## Notes that bite people

- Go tests must run `-p 1` (several packages share and truncate domain
  tables against one database); `mise run test` does this for you.
- Tests run against a **real Postgres**, never a mock — that is the point.
- The container image is COPY-only: run `bash scripts/build-artifacts.sh`
  before `docker build .`, or use `mise run image` / `mise run snapshot`,
  which do it for you.
- `APP_URL` must be exactly the origin the browser uses
  (`http://localhost:5173` in the dev loop above, not `127.0.0.1`) — Limen
  rejects non-GET auth requests whose `Origin` header does not match it.

## Pins bumped by hand

Dependabot covers GitHub Actions, the Go module, the bun workspace and the
Dockerfile base images. It does not cover:

- The tool versions in `.mise.toml`: `go`, `bun`, `sqlc`, `oapi-codegen`,
  `goose`, `goreleaser`, `svu`, `cosign`, `syft`.
- `openapi-typescript@7.13.0`, pinned inline in the root `package.json`
  `gen:client` script rather than as a `package.json` dependency.
- `@scalar/api-reference@1.67.0` in
  `apps/server/internal/web/scalar.html`, pinned by URL with a Subresource
  Integrity hash (the file's own comment explains how to recompute it).

Bump these by hand when a new version is worth taking; nothing will remind
you otherwise.

## Pull requests

CI runs the full suite, builds the image from natively-built artifacts,
smoke-tests it, runs the Playwright suite against it, and — for PRs from
this repository — pushes a preview image
(`ghcr.io/refsdal/snarvei:<next-version>-pr.<number>`) you can run. Keep
PRs small and scoped; every commit should compile and pass tests on its
own.
