# Go Migration Phase 1: Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the Cloudflare Workers deployment with a Go server scaffold that migrates a Postgres database, serves the embedded (still react-router) SPA, answers `/healthz`, `/readyz` and `/api/config`, and ships as a distroless image built and smoke-tested by CI.

**Architecture:** One Go module at `apps/server` (stdlib `net/http`, pgx, goose, oapi-codegen strict server, kin-openapi validation) embeds the Vite build of `apps/frontend` with `go:embed`. `cmd/snarvei` is the composition root and dispatch table (default = migrate then serve, `server`, `migrate`, `healthcheck`). Nothing compiles inside Docker: `scripts/build-artifacts.sh` builds natively and the Dockerfile only COPYs.

**Tech Stack:** Go 1.27, pgx v5.10.0, goose v3.28.0, kin-openapi v0.149.0, oapi-codegen v2.8.0 (+ runtime v1.7.0, nethttp-middleware v1.2.0), Postgres 17, bun 1.4, biome, Vite 8, React 19, MUI 9, mise, distroless static.

**Spec:** `docs/superpowers/specs/2026-09-04-go-backend-migration-design.md` (sections 1, 2 (probes, fallback, headers), 4, 7, 8, 9, 11 phase 1)

## Global Constraints

- Module path is `github.com/refsdal/snarvei/server`; Go `1.27.0`; `CGO_ENABLED=0` everywhere.
- Dependency versions: `github.com/jackc/pgx/v5 v5.10.0`, `github.com/pressly/goose/v3 v3.28.0`, `github.com/getkin/kin-openapi v0.149.0`, `github.com/oapi-codegen/runtime v1.7.0`, `github.com/oapi-codegen/nethttp-middleware v1.2.0`; oapi-codegen CLI `v2.8.0`.
- Toolchain pins live only in `.mise.toml` (dev and CI). Node/pnpm are gone; JS is bun 1.4 + biome.
- Generated code (`internal/api/gen/*.gen.go`, `internal/api/snarvei.yaml`) is committed; CI and the image never run codegen; a test fails on drift.
- `openapi/snarvei.yaml` is the single source of truth for every HTTP route except `/api/auth/*`.
- Every API error body is `{"code": "...", "message": "..."}` (optional `details`). Unknown `/api/` paths answer `404 {"code":"NOT_FOUND"}`.
- `/healthz` constructs nothing and touches nothing. `/readyz` runs `SELECT 1` and answers `503 {"ok":false,"error":"..."}` on failure. Both send `Cache-Control: no-store` and no SPA security headers.
- Non-API responses carry exactly the header set in spec section 2, including the CSP string `default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'`.
- Configuration is environment only, validated once at startup, every problem reported at once. Defaults: `PORT=3000`, `APP_NAME=Snarvei`, `TRUSTED_PROXY_HOPS=0`, `OPEN_SIGNUP=1`, `MIGRATION_LOCK_KEY=1935762089`, `LOG_LEVEL=info`, `E2E_TEST_HOOKS=0`.
- Migrations are goose, forward-only, embedded, applied under a Postgres advisory lock.
- The image base is `gcr.io/distroless/static-debian12:nonroot` pinned by digest; `/data` is owned by uid 65532; `HEALTHCHECK` runs `/app/snarvei healthcheck`; nothing compiles in Docker.
- Go tests run with `go test -p 1 ./...` against `TEST_DATABASE_URL` (default `postgres://snarvei:snarvei@127.0.0.1:55432/snarvei_test`).
- Commit messages follow Conventional Commits (`feat:`, `fix:`, `chore:`, `docs:`, `ci:`, `test:`); svu computes versions from them later.
- Run every command from the repository root unless a step says otherwise. Go commands run from `apps/server`.
- The Pjokk repository at `~/projects/refsdal/pjokk` is the reference; where this plan says "copy from Pjokk", copy the file, then apply the listed edits. Everything else is written out in full here.

---

## File Structure

Created in this phase:

```
LICENSE                                   AGPL-3.0-only (copied from Pjokk)
.mise.toml                                toolchain pins + tasks
.env.example                              complete configuration contract
.dockerignore  Dockerfile  docker/data-skel/.keep
docker-compose.yml  docker-compose.selfhost.yml  docker-compose.test.yml
biome.json  tsconfig.base.json  package.json (root, bun workspaces)  bun.lock
scripts/build-artifacts.sh  scripts/spa-embed-overlay.sh  scripts/restore-embed-overlay.sh  scripts/build-image.sh
openapi/snarvei.yaml
apps/frontend/{index.html,package.json,tsconfig.json,vite.config.ts,public/,src/,test/}
apps/server/go.mod  go.sum  generate.go  sqlc.yaml
apps/server/cmd/snarvei/main.go  main_test.go
apps/server/internal/config/config.go  config_test.go
apps/server/internal/db/{db.go,migrations.go,migrate.go,migrate_test.go,migrations/00001_init.sql,queries/.keep}
apps/server/internal/testrig/rig.go
apps/server/internal/web/{web.go,web_test.go,composed_test.go,dist/index.html}
apps/server/internal/api/{api.go,system.go,api_test.go,spec_sync_test.go,generate_test.go,snarvei.yaml}
apps/server/internal/api/gen/{cfg-server.yaml,cfg-types.yaml,server.gen.go,types.gen.go}
apps/server/internal/api/respond/respond.go
.github/workflows/test.yml  ci.yml  .github/dependabot.yml
```

Deleted: `src/`, `tests/`, `public/_headers`, `index.html`, `wrangler.jsonc`, `worker-configuration.d.ts`, `.wrangler/`, `drizzle.config.ts`, `vitest.config.mts`, `playwright.config.ts`, `tsconfig.app.json`, `tsconfig.node.json`, `tsconfig.worker.json`, `eslint.config.js`, `.prettierrc`, `.prettierignore`, `.nvmrc`, `pnpm-lock.yaml`, `pnpm-workspace.yaml`, `.dev.vars`, `.dev.vars.example`, `.github/workflows/deploy-dev.yml`, `.github/workflows/deploy-production.yml`, `.github/workflows/README.md`, `dist/`, `test-results/`.

Responsibilities: `config` parses env; `db` owns migrations and the pool; `testrig` gives tests a migrated, truncated Postgres; `web` serves the embedded SPA; `api` owns every spec'd route and the JSON 404; `respond` is the error envelope shared by `api` and future middleware; `cmd/snarvei` is the only place that reads the environment and constructs collaborators.

---

### Task 1: Remove the Workers deployment and add the license

**Files:**
- Delete: `src/worker/`, `src/shared/`, `tests/`, `wrangler.jsonc`, `worker-configuration.d.ts`, `.wrangler/`, `drizzle.config.ts`, `vitest.config.mts`, `playwright.config.ts`, `tsconfig.worker.json`, `tsconfig.node.json`, `.dev.vars`, `.dev.vars.example`, `public/_headers`, `.github/workflows/deploy-dev.yml`, `.github/workflows/deploy-production.yml`, `.github/workflows/README.md`, `dist/`, `test-results/`
- Create: `LICENSE`
- Modify: `.gitignore`

**Interfaces:**
- Consumes: nothing.
- Produces: a tree where `src/react-app/` is the only application code left. Task 2 moves it.

- [ ] **Step 1: Confirm the frontend has no imports from the worker**

Run: `grep -rn "worker/" src/react-app | grep import`
Expected: no output. (`src/react-app/types.ts` imports `../shared/api-types`; that file is moved in Task 2, not deleted here.)

- [ ] **Step 2: Move the one shared file the frontend still needs**

```bash
git mv src/shared/api-types.ts src/react-app/lib/api-types.ts
sed -i 's#"\.\./shared/api-types"#"./lib/api-types"#' src/react-app/types.ts
```

Run: `grep -n "api-types" src/react-app/types.ts`
Expected: `import type { AnalyticsSummaryDto, HistoryItemDto, LinkDto, TeamMemberDto } from "./lib/api-types";`

- [ ] **Step 3: Delete the Workers deployment and worker sources**

```bash
git rm -r -q src/worker src/shared tests wrangler.jsonc worker-configuration.d.ts \
  drizzle.config.ts vitest.config.mts playwright.config.ts \
  tsconfig.worker.json tsconfig.node.json .dev.vars.example public/_headers \
  .github/workflows/deploy-dev.yml .github/workflows/deploy-production.yml \
  .github/workflows/README.md
rm -rf .wrangler .dev.vars dist test-results
```

- [ ] **Step 4: Add the AGPL-3.0 license**

```bash
cp ~/projects/refsdal/pjokk/LICENSE LICENSE
```

Run: `head -3 LICENSE`
Expected: the first line is `                    GNU AFFERO GENERAL PUBLIC LICENSE`.

- [ ] **Step 5: Replace `.gitignore`**

```gitignore
node_modules/
dist/
coverage/
*.tsbuildinfo
.idea/
.playwright-mcp/
.DS_Store
.env
.env.*
!.env.example

# apps/server embeds the SPA build from here; the committed placeholder
# index.html is repo content, not a build artifact.
!apps/server/internal/web/dist/
!apps/server/internal/web/dist/index.html

# Playwright (phase 5)
e2e/test-results/
e2e/playwright-report/
test-results/
playwright-report/
```

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "chore: remove the Cloudflare Workers deployment and worker sources

Snarvei leaves Workers for a Go server (see
docs/superpowers/specs/2026-09-04-go-backend-migration-design.md).
The React app stays and is moved to apps/frontend next. AGPL-3.0 added."
```

---

### Task 2: Move the SPA to `apps/frontend` on bun + biome

**Files:**
- Create: `package.json` (root), `biome.json`, `tsconfig.base.json`, `apps/frontend/package.json`, `apps/frontend/tsconfig.json`, `apps/frontend/vite.config.ts`, `apps/frontend/index.html`, `apps/frontend/test/routes.test.ts`, `bun.lock`
- Move: `src/react-app/**` → `apps/frontend/src/**`, `public/` → `apps/frontend/public/`
- Delete: `package.json` contents (rewritten), `index.html`, `tsconfig.json`, `tsconfig.app.json`, `eslint.config.js`, `.prettierrc`, `.prettierignore`, `.nvmrc`, `pnpm-lock.yaml`, `pnpm-workspace.yaml`, `vite.config.ts`

**Interfaces:**
- Consumes: nothing.
- Produces: `bun run build` writes the SPA to `dist/client/` at the repo root (`index.html` + `assets/*`); `bun run check` (biome + tsc) and `bun run test` pass. Task 8's overlay script copies `dist/client` into the Go embed dir.

- [ ] **Step 1: Move the sources**

```bash
mkdir -p apps/frontend
git mv src/react-app apps/frontend/src
git mv public apps/frontend/public
git rm -q index.html tsconfig.json tsconfig.app.json eslint.config.js .prettierrc .prettierignore .nvmrc pnpm-lock.yaml pnpm-workspace.yaml vite.config.ts
rmdir src
```

- [ ] **Step 2: Write the root `package.json`**

```json
{
  "name": "snarvei",
  "private": true,
  "license": "AGPL-3.0-only",
  "version": "0.0.0",
  "type": "module",
  "workspaces": ["apps/frontend"],
  "// backend": "apps/server is a Go module with no package.json: `cd apps/server && go run ./cmd/snarvei` (default mode), `... migrate`, `... server`, `... healthcheck`; `go test -p 1 ./...` for its suite. The scripts below cover the SPA only.",
  "scripts": {
    "dev": "bun run --filter @snarvei/frontend dev",
    "dev:server": "cd apps/server && go run ./cmd/snarvei",
    "build": "bun run --filter @snarvei/frontend build",
    "check": "biome check . && bun run typecheck",
    "typecheck": "bun run --filter '*' typecheck",
    "test": "bun run --filter '*' test",
    "lint": "biome check .",
    "lint:fix": "biome check --write ."
  },
  "devDependencies": {
    "@biomejs/biome": "^2.5.10",
    "@types/bun": "^1.4.0",
    "typescript": "^6.0.3"
  },
  "trustedDependencies": ["@biomejs/biome", "esbuild"]
}
```

- [ ] **Step 3: Write `apps/frontend/package.json`**

Keep the runtime dependencies the app imports today (MUI, Emotion, React, react-router-dom, react-qr-code, better-auth for the client until phase 4) and drop everything that belonged to the Worker.

```json
{
  "name": "@snarvei/frontend",
  "version": "0.0.0",
  "private": true,
  "license": "AGPL-3.0-only",
  "type": "module",
  "scripts": {
    "dev": "vite dev",
    "build": "vite build",
    "test": "bun test",
    "typecheck": "tsc -p tsconfig.json --noEmit"
  },
  "dependencies": {
    "@better-auth/passkey": "^1.7.1",
    "@emotion/react": "^11.14.0",
    "@emotion/styled": "^11.14.1",
    "@mui/icons-material": "^9.3.1",
    "@mui/material": "^9.3.1",
    "@mui/x-data-grid": "^9.11.0",
    "better-auth": "^1.7.1",
    "react": "^19.2.8",
    "react-dom": "^19.2.8",
    "react-qr-code": "^2.2.0",
    "react-router-dom": "^7.18.2"
  },
  "devDependencies": {
    "@types/node": "^26.2.0",
    "@types/react": "^19.2.18",
    "@types/react-dom": "^19.2.4",
    "@vitejs/plugin-react": "^6.1.0",
    "vite": "^8.2.2"
  }
}
```

- [ ] **Step 4: Write `tsconfig.base.json` and `apps/frontend/tsconfig.json`**

`tsconfig.base.json`:

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "module": "ESNext",
    "moduleResolution": "bundler",
    "strict": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true,
    "forceConsistentCasingInFileNames": true,
    "verbatimModuleSyntax": true,
    "isolatedModules": true,
    "skipLibCheck": true,
    "noEmit": true,
    "resolveJsonModule": true
  }
}
```

`apps/frontend/tsconfig.json`:

```json
{
  "extends": "../../tsconfig.base.json",
  "compilerOptions": {
    "lib": ["ES2023", "DOM", "DOM.Iterable"],
    "jsx": "react-jsx",
    "types": ["vite/client", "node", "bun"]
  },
  "include": ["src", "test", "vite.config.ts"]
}
```

- [ ] **Step 5: Write `apps/frontend/vite.config.ts` and `apps/frontend/index.html`**

`vite.config.ts`:

```ts
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// Builds the SPA only. The Go server (apps/server) embeds dist/client via
// go:embed; in development it runs on :3000 and Vite proxies the server-owned
// paths to it so the client's same-origin assumption (and cookies) hold.
const serverPaths = ["/api", "/l", "/healthz", "/readyz", "/openapi.json", "/scalar", "/images", "/robots.txt"];

export default defineConfig({
  define: {
    // Reported by the settings page; the Go server reports its own version on /healthz.
    __APP_VERSION__: JSON.stringify(process.env.APP_VERSION ?? "dev"),
  },
  build: {
    outDir: "../../dist/client",
    emptyOutDir: true,
  },
  server: {
    proxy: Object.fromEntries(serverPaths.map((p) => [p, "http://localhost:3000"])),
  },
  plugins: [react()],
});
```

`index.html`:

```html
<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Snarvei</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
```

- [ ] **Step 6: Check whether the app declares `__APP_VERSION__`**

Run: `grep -rn "__APP_VERSION__" apps/frontend/src`
Expected: a `declare const __APP_VERSION__: string;` somewhere (a `vite-env.d.ts` or `types.ts`). If nothing is found, create `apps/frontend/src/vite-env.d.ts`:

```ts
/// <reference types="vite/client" />
declare const __APP_VERSION__: string;
```

- [ ] **Step 7: Write `biome.json`**

```json
{
  "$schema": "./node_modules/@biomejs/biome/configuration_schema.json",
  "files": {
    "includes": ["apps/**", "scripts/**", "*.ts", "!apps/server", "!apps/frontend/src/lib/api-schema.d.ts"]
  },
  "formatter": {
    "enabled": true,
    "indentStyle": "space",
    "indentWidth": 2,
    "lineWidth": 120
  },
  "linter": {
    "enabled": true,
    "rules": {
      "preset": "recommended",
      "style": { "noNonNullAssertion": "off" },
      "correctness": { "useExhaustiveDependencies": "warn" }
    }
  },
  "assist": { "actions": { "source": { "organizeImports": "off" } } }
}
```

- [ ] **Step 8: Write the failing frontend test**

`apps/frontend/test/routes.test.ts`:

```ts
import { describe, expect, test } from "bun:test";
import { buildLinksPath, buildOrganizationPath, settingsPath } from "../src/lib/routes";

describe("route helpers", () => {
  test("prefers the organization slug over its id", () => {
    expect(buildOrganizationPath({ id: "org_1", slug: "acme" })).toBe("/app/acme/dashboard");
    expect(buildOrganizationPath({ id: "org_1", slug: undefined })).toBe("/app/org_1/dashboard");
  });

  test("builds link list and detail paths", () => {
    expect(buildLinksPath({ id: "org_1", slug: "acme" })).toBe("/app/acme/links");
    expect(buildLinksPath({ id: "org_1", slug: "acme" }, "lnk_9")).toBe("/app/acme/links/lnk_9");
  });

  test("settings path is fixed", () => {
    expect(settingsPath).toBe("/app/settings");
  });
});
```

- [ ] **Step 9: Install and run the checks**

```bash
mise use bun@1.4   # writes .mise.toml if missing; Task 3 replaces the file
bun install
bun run lint:fix
bun run check
bun run test
bun run build
```

Expected: `bun run check` exits 0 (fix any biome complaints it reports by hand; do not disable rules), `bun run test` reports 3 passing tests, `bun run build` writes `dist/client/index.html` and `dist/client/assets/*.js`.

Run: `ls dist/client`
Expected: `assets  index.html  vite.svg` (or similar; `index.html` must be present).

- [ ] **Step 10: Commit**

```bash
git add -A
git commit -m "chore(frontend): move the SPA to apps/frontend on bun + biome

Vite builds to dist/client for the Go server to embed; server-owned
paths are proxied to :3000 in development. ESLint, Prettier and pnpm
are replaced by biome and bun."
```

---

### Task 3: Go module, toolchain pins and the config package

**Files:**
- Create: `.mise.toml`, `apps/server/go.mod`, `apps/server/go.sum`, `apps/server/internal/config/config.go`, `apps/server/internal/config/config_test.go`
- Delete: the temporary `.mise.toml` Task 2 created

**Interfaces:**
- Produces:
  ```go
  package config
  type Config struct {
      DatabaseURL, AppURL, AuthSecret string
      StorageDriver, StorageFSPath string
      S3Bucket, S3Endpoint, S3AccessKeyID, S3SecretAccessKey, S3Region string
      Port int
      AppName string
      TrustedProxyHops int
      OpenSignup bool
      IPHashPepper string
      SMTPHost string; SMTPPort int; SMTPUsername, SMTPPassword, EmailFrom string
      MigrationLockKey int64
      LogLevel string
      E2ETestHooks bool
  }
  func Load(env map[string]string) (*Config, error)
  func FromOS() (*Config, error)
  func (c *Config) EmailEnabled() bool
  func (c *Config) Secure() bool           // APP_URL is https
  func (c *Config) DisabledSubsystems() []string
  ```

- [ ] **Step 1: Write `.mise.toml`**

Copy the version numbers from `~/projects/refsdal/pjokk/.mise.toml` for go, sqlc, oapi-codegen, goose, goreleaser, svu, cosign and syft (run `cat ~/projects/refsdal/pjokk/.mise.toml | head -15`), then write:

```toml
# Toolchain pin for Snarvei: the single source of truth for dev AND CI.
# `mise install` gives everyone the same Go + bun plus the codegen tools
# `go generate` expects on PATH. CI installs only what it runs (see
# .github/workflows); generated code is committed and drift-guarded.
[tools]
go = "1.27.0"
bun = "1.4"
"go:github.com/sqlc-dev/sqlc/cmd/sqlc" = "1.31.1"
"go:github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen" = "2.8.0"
"go:github.com/pressly/goose/v3/cmd/goose" = "3.28.0"
"aqua:goreleaser/goreleaser" = "2.18.0"
"aqua:caarlos0/svu" = "3.4.1"
"aqua:sigstore/cosign" = "3.1.3"
"aqua:anchore/syft" = "1.51.1"

[tasks.test]
description = "Full suite: Go (real Postgres from docker-compose.test.yml) + frontend"
run = [
  "cd apps/server && go vet ./... && go test -p 1 -count=1 ./...",
  "bun run test",
]

[tasks.check]
description = "Lint, typecheck"
run = ["bun run check"]

[tasks.artifacts]
description = "SPA + both server binaries -> dist/server/linux/<arch>/snarvei"
run = "bash scripts/build-artifacts.sh"

[tasks.image]
description = "Multi-arch image via buildx (runs artifacts first)"
run = "bash scripts/build-image.sh"
```

Run: `mise install && mise exec -- go version && mise exec -- bun --version`
Expected: `go version go1.27.0 linux/amd64` and `1.4.x`.

- [ ] **Step 2: Initialise the module and pin the dependencies**

```bash
mkdir -p apps/server && cd apps/server
go mod init github.com/refsdal/snarvei/server
go get github.com/jackc/pgx/v5@v5.10.0 github.com/pressly/goose/v3@v3.28.0 \
  github.com/getkin/kin-openapi@v0.149.0 github.com/oapi-codegen/runtime@v1.7.0 \
  github.com/oapi-codegen/nethttp-middleware@v1.2.0
```

Then edit `go.mod` so the `go` directive reads `go 1.27.0`.

- [ ] **Step 3: Write the failing config tests**

`apps/server/internal/config/config_test.go`:

```go
package config

import (
	"strings"
	"testing"
)

func minimal() map[string]string {
	return map[string]string{
		"DATABASE_URL":    "postgres://snarvei:snarvei@localhost:5432/snarvei",
		"APP_URL":         "https://snarvei.example.com",
		"AUTH_SECRET":     strings.Repeat("s", 32),
		"STORAGE_DRIVER":  "fs",
		"STORAGE_FS_PATH": "/data",
	}
}

func TestLoadMinimalAppliesDefaults(t *testing.T) {
	cfg, err := Load(minimal())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != 3000 || cfg.AppName != "Snarvei" || cfg.TrustedProxyHops != 0 || !cfg.OpenSignup ||
		cfg.MigrationLockKey != 1935762089 || cfg.LogLevel != "info" || cfg.E2ETestHooks {
		t.Fatalf("defaults not applied: %+v", cfg)
	}
	if !cfg.Secure() {
		t.Fatal("https APP_URL must be Secure")
	}
	if cfg.EmailEnabled() {
		t.Fatal("email must be off without SMTP_*")
	}
}

func TestLoadReportsEveryMissingRequiredVariable(t *testing.T) {
	_, err := Load(map[string]string{})
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"DATABASE_URL", "APP_URL", "AUTH_SECRET", "STORAGE_DRIVER"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %s: %v", want, err)
		}
	}
}

func TestLoadRejectsShortSecretAndBadURL(t *testing.T) {
	env := minimal()
	env["AUTH_SECRET"] = "short"
	env["APP_URL"] = "not-a-url"
	_, err := Load(env)
	if err == nil || !strings.Contains(err.Error(), "AUTH_SECRET") || !strings.Contains(err.Error(), "APP_URL") {
		t.Fatalf("expected both problems, got: %v", err)
	}
}

func TestLoadS3RequiresAllFour(t *testing.T) {
	env := minimal()
	env["STORAGE_DRIVER"] = "s3"
	delete(env, "STORAGE_FS_PATH")
	env["S3_BUCKET"] = "b"
	_, err := Load(env)
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"S3_ENDPOINT", "S3_ACCESS_KEY_ID", "S3_SECRET_ACCESS_KEY"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %s: %v", want, err)
		}
	}
	env["S3_ENDPOINT"] = "https://s3.example.com"
	env["S3_ACCESS_KEY_ID"] = "k"
	env["S3_SECRET_ACCESS_KEY"] = "s"
	cfg, err := Load(env)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.S3Region != "auto" {
		t.Fatalf("S3_REGION default = %q, want auto", cfg.S3Region)
	}
}

func TestLoadSMTPIsAllOrNothing(t *testing.T) {
	env := minimal()
	env["SMTP_HOST"] = "smtp.example.com"
	_, err := Load(env)
	if err == nil || !strings.Contains(err.Error(), "SMTP_PORT") || !strings.Contains(err.Error(), "EMAIL_FROM") {
		t.Fatalf("partial SMTP config must fail naming the missing ones, got: %v", err)
	}
	env["SMTP_PORT"] = "587"
	env["SMTP_USERNAME"] = "u"
	env["SMTP_PASSWORD"] = "p"
	env["EMAIL_FROM"] = "Snarvei <no-reply@example.com>"
	cfg, err := Load(env)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.EmailEnabled() || cfg.SMTPPort != 587 {
		t.Fatalf("email should be enabled: %+v", cfg)
	}
}

func TestLoadSwitches(t *testing.T) {
	env := minimal()
	env["OPEN_SIGNUP"] = "0"
	env["PORT"] = "8080"
	env["TRUSTED_PROXY_HOPS"] = "2"
	env["LOG_LEVEL"] = "debug"
	env["MIGRATION_LOCK_KEY"] = "42"
	cfg, err := Load(env)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.OpenSignup || cfg.Port != 8080 || cfg.TrustedProxyHops != 2 || cfg.LogLevel != "debug" || cfg.MigrationLockKey != 42 {
		t.Fatalf("switches not applied: %+v", cfg)
	}

	env["OPEN_SIGNUP"] = "yes"
	env["PORT"] = "-1"
	env["LOG_LEVEL"] = "loud"
	if _, err := Load(env); err == nil || !strings.Contains(err.Error(), "OPEN_SIGNUP") ||
		!strings.Contains(err.Error(), "PORT") || !strings.Contains(err.Error(), "LOG_LEVEL") {
		t.Fatalf("expected OPEN_SIGNUP, PORT and LOG_LEVEL problems, got: %v", err)
	}
}

func TestE2ETestHooksOnlyOnLoopback(t *testing.T) {
	env := minimal()
	env["E2E_TEST_HOOKS"] = "1"
	if _, err := Load(env); err == nil || !strings.Contains(err.Error(), "E2E_TEST_HOOKS") {
		t.Fatalf("hooks on a public APP_URL must fail, got: %v", err)
	}
	env["APP_URL"] = "http://127.0.0.1:3300"
	cfg, err := Load(env)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.E2ETestHooks || cfg.Secure() {
		t.Fatalf("expected hooks on and Secure off: %+v", cfg)
	}
}

func TestDisabledSubsystems(t *testing.T) {
	cfg, err := Load(minimal())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := strings.Join(cfg.DisabledSubsystems(), ","); got != "email" {
		t.Fatalf("DisabledSubsystems = %q, want email", got)
	}
}
```

- [ ] **Step 4: Run the tests to verify they fail**

Run: `cd apps/server && go test ./internal/config/`
Expected: compile error, `undefined: Load`.

- [ ] **Step 5: Write `apps/server/internal/config/config.go`**

```go
// Package config loads and validates the process configuration from
// environment variables: parse once at startup, report EVERY problem at
// once, so a misconfigured container crash-loops with a list rather than
// one restart per mistake. Nothing outside cmd/snarvei calls FromOS.
package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// Config is the validated process configuration. Every field is populated
// by Load; there is no lazy or partial state.
type Config struct {
	DatabaseURL string
	AppURL      string
	AuthSecret  string

	StorageDriver     string // "fs" | "s3"
	StorageFSPath     string
	S3Bucket          string
	S3Endpoint        string
	S3AccessKeyID     string
	S3SecretAccessKey string
	S3Region          string

	Port             int
	AppName          string
	TrustedProxyHops int
	OpenSignup       bool
	IPHashPepper     string

	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	EmailFrom    string

	MigrationLockKey int64
	LogLevel         string
	E2ETestHooks     bool
}

type problems struct{ list []string }

func (p *problems) add(field, message string) {
	p.list = append(p.list, fmt.Sprintf("%s: %s", field, message))
}

func (p *problems) require(env map[string]string, field string) (string, bool) {
	v := strings.TrimSpace(env[field])
	if v == "" {
		p.add(field, "is required")
		return "", false
	}
	return v, true
}

func isAbsoluteHTTPURL(v string) bool {
	u, err := url.Parse(v)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func isLoopbackURL(v string) bool {
	u, err := url.Parse(v)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (p *problems) intOr(env map[string]string, field string, def int, min int) int {
	v := strings.TrimSpace(env[field])
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		p.add(field, "must be an integer")
		return def
	}
	if n < min {
		p.add(field, fmt.Sprintf("must be at least %d", min))
		return def
	}
	return n
}

func (p *problems) boolOr(env map[string]string, field string, def bool) bool {
	switch strings.TrimSpace(env[field]) {
	case "":
		return def
	case "0":
		return false
	case "1":
		return true
	default:
		p.add(field, `must be "0" or "1"`)
		return def
	}
}

// Load parses and validates a plain string map (the shape a real environ
// and a test fixture share). It reports every invalid or missing field in
// one error.
func Load(env map[string]string) (*Config, error) {
	p := &problems{}
	cfg := &Config{}

	if v, ok := p.require(env, "DATABASE_URL"); ok {
		cfg.DatabaseURL = v
	}
	if v, ok := p.require(env, "APP_URL"); ok {
		if !isAbsoluteHTTPURL(v) {
			p.add("APP_URL", "must be an absolute http(s) URL")
		} else {
			cfg.AppURL = strings.TrimRight(v, "/")
		}
	}
	if v, ok := p.require(env, "AUTH_SECRET"); ok {
		if len(v) < 32 {
			p.add("AUTH_SECRET", "must be at least 32 bytes")
		} else {
			cfg.AuthSecret = v
		}
	}

	if driver, ok := p.require(env, "STORAGE_DRIVER"); ok {
		switch driver {
		case "fs":
			cfg.StorageDriver = driver
			if v, ok := p.require(env, "STORAGE_FS_PATH"); ok {
				cfg.StorageFSPath = v
			}
		case "s3":
			cfg.StorageDriver = driver
			if v, ok := p.require(env, "S3_BUCKET"); ok {
				cfg.S3Bucket = v
			}
			if v, ok := p.require(env, "S3_ENDPOINT"); ok {
				if !isAbsoluteHTTPURL(v) {
					p.add("S3_ENDPOINT", "must be an absolute http(s) URL")
				} else {
					cfg.S3Endpoint = v
				}
			}
			if v, ok := p.require(env, "S3_ACCESS_KEY_ID"); ok {
				cfg.S3AccessKeyID = v
			}
			if v, ok := p.require(env, "S3_SECRET_ACCESS_KEY"); ok {
				cfg.S3SecretAccessKey = v
			}
			cfg.S3Region = "auto"
			if v := strings.TrimSpace(env["S3_REGION"]); v != "" {
				cfg.S3Region = v
			}
		default:
			p.add("STORAGE_DRIVER", `must be "fs" or "s3"`)
		}
	}

	cfg.Port = p.intOr(env, "PORT", 3000, 1)
	cfg.AppName = "Snarvei"
	if v := strings.TrimSpace(env["APP_NAME"]); v != "" {
		cfg.AppName = v
	}
	cfg.TrustedProxyHops = p.intOr(env, "TRUSTED_PROXY_HOPS", 0, 0)
	cfg.OpenSignup = p.boolOr(env, "OPEN_SIGNUP", true)
	cfg.IPHashPepper = env["IP_HASH_PEPPER"]

	// SMTP: all five or none. Half a mail configuration is a misconfiguration,
	// not "email off".
	smtpFields := []string{"SMTP_HOST", "SMTP_PORT", "SMTP_USERNAME", "SMTP_PASSWORD", "EMAIL_FROM"}
	present := 0
	for _, f := range smtpFields {
		if strings.TrimSpace(env[f]) != "" {
			present++
		}
	}
	if present > 0 {
		for _, f := range smtpFields {
			if strings.TrimSpace(env[f]) == "" {
				p.add(f, "is required when any SMTP_* variable is set")
			}
		}
		cfg.SMTPHost = strings.TrimSpace(env["SMTP_HOST"])
		cfg.SMTPPort = p.intOr(env, "SMTP_PORT", 0, 1)
		cfg.SMTPUsername = env["SMTP_USERNAME"]
		cfg.SMTPPassword = env["SMTP_PASSWORD"]
		cfg.EmailFrom = strings.TrimSpace(env["EMAIL_FROM"])
	}

	cfg.MigrationLockKey = 1935762089
	if v := strings.TrimSpace(env["MIGRATION_LOCK_KEY"]); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			p.add("MIGRATION_LOCK_KEY", "must be an integer")
		} else {
			cfg.MigrationLockKey = n
		}
	}

	cfg.LogLevel = "info"
	if v := strings.TrimSpace(env["LOG_LEVEL"]); v != "" {
		switch v {
		case "debug", "info", "warn", "error":
			cfg.LogLevel = v
		default:
			p.add("LOG_LEVEL", `must be one of "debug", "info", "warn", "error"`)
		}
	}

	cfg.E2ETestHooks = p.boolOr(env, "E2E_TEST_HOOKS", false)
	if cfg.E2ETestHooks && cfg.AppURL != "" && !isLoopbackURL(cfg.AppURL) {
		p.add("E2E_TEST_HOOKS", "may only be enabled when APP_URL is a loopback origin")
	}

	if len(p.list) > 0 {
		return nil, fmt.Errorf("invalid configuration:\n  %s", strings.Join(p.list, "\n  "))
	}
	return cfg, nil
}

// FromOS loads configuration from the real environment.
func FromOS() (*Config, error) {
	env := make(map[string]string, len(os.Environ()))
	for _, kv := range os.Environ() {
		if k, v, ok := strings.Cut(kv, "="); ok {
			env[k] = v
		}
	}
	return Load(env)
}

// EmailEnabled reports whether transactional email is configured.
func (c *Config) EmailEnabled() bool { return c.SMTPHost != "" }

// Secure reports whether APP_URL is https, which decides cookie Secure flags.
func (c *Config) Secure() bool { return strings.HasPrefix(c.AppURL, "https://") }

// DisabledSubsystems names optional subsystems that are off, for the boot log.
func (c *Config) DisabledSubsystems() []string {
	var off []string
	if !c.EmailEnabled() {
		off = append(off, "email")
	}
	return off
}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `cd apps/server && go vet ./... && go test ./internal/config/ -v`
Expected: all 8 tests PASS.

- [ ] **Step 7: Commit**

```bash
git add .mise.toml apps/server
git commit -m "feat(server): Go module, toolchain pins and validated configuration"
```

---

### Task 4: Database package: migrations, advisory-locked runner, pool, test rig

**Files:**
- Create: `apps/server/internal/db/migrations.go`, `apps/server/internal/db/migrate.go`, `apps/server/internal/db/db.go`, `apps/server/internal/db/migrations/00001_init.sql`, `apps/server/internal/db/queries/.keep`, `apps/server/internal/db/migrate_test.go`, `apps/server/internal/testrig/rig.go`, `apps/server/sqlc.yaml`, `docker-compose.test.yml`

**Interfaces:**
- Produces:
  ```go
  package db
  const DefaultMigrationLockKey int64 = 1935762089
  func ApplyMigrations(ctx context.Context, databaseURL string, lockKey int64) error
  func LatestMigrationVersion() (int64, error)
  func New(ctx context.Context, databaseURL string) (*pgxpool.Pool, error)
  func IsUniqueViolation(err error) bool

  package testrig
  func DatabaseURL() string
  type Rig struct{ Pool *pgxpool.Pool }
  func Setup(t *testing.T) *Rig     // migrated + truncated database, pool closed on cleanup
  ```

- [ ] **Step 1: Write `docker-compose.test.yml` and start it**

```yaml
# The database the Go suite runs against:
#   docker compose -f docker-compose.test.yml up -d
#   cd apps/server && go test -p 1 ./...
name: snarvei-test
services:
  db:
    image: postgres:17-alpine
    environment:
      POSTGRES_USER: snarvei
      POSTGRES_PASSWORD: snarvei
      POSTGRES_DB: snarvei_test
    ports:
      - "127.0.0.1:55432:5432"
    tmpfs:
      - /var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U snarvei -d snarvei_test"]
      interval: 3s
      timeout: 3s
      retries: 20
```

Run: `docker compose -f docker-compose.test.yml up -d --wait`
Expected: the `db` service reports healthy.

- [ ] **Step 2: Write the migration `apps/server/internal/db/migrations/00001_init.sql`**

The Limen-shaped tables are Pjokk's `00001_init.sql` with its `00002_limen_align.sql` already applied, minus Pjokk-only columns, plus the two-factor plugin's table (`two_factors`) and user column (`two_factor_enabled`, read from the plugin's `constants.go`). The Snarvei tables follow spec section 4.

```sql
-- +goose Up

-- ===========================================================================
-- Auth tables (Limen-shaped). Column sets come from Pjokk's runtime-verified
-- migrations (00001_init + 00002_limen_align) and, for two_factors, from the
-- plugin's own schema definition. Limen does not migrate or validate the
-- schema itself; a mismatch here surfaces as a SQL error on first use.
-- ===========================================================================

CREATE TABLE "users" (
	"id" text PRIMARY KEY DEFAULT gen_random_uuid()::text,
	"public_id" text NOT NULL DEFAULT gen_random_uuid()::text,
	"first_name" text,
	"last_name" text,
	"email" text NOT NULL,
	"password" text,
	"email_verified_at" timestamptz,
	"two_factor_enabled" boolean NOT NULL DEFAULT false,
	"created_at" timestamptz NOT NULL DEFAULT now(),
	"updated_at" timestamptz NOT NULL DEFAULT now(),
	"deleted_at" timestamptz,
	-- Snarvei's own fields, supplied through Limen's additional-fields map.
	"name" text,
	"image" text,
	CONSTRAINT "users_email_unique" UNIQUE ("email"),
	CONSTRAINT "users_public_id_unique" UNIQUE ("public_id")
);

CREATE TABLE "organizations" (
	"id" text PRIMARY KEY DEFAULT gen_random_uuid()::text,
	"name" text NOT NULL,
	"slug" text NOT NULL,
	"logo" text,
	"metadata" text,
	"created_at" timestamptz NOT NULL DEFAULT now(),
	"updated_at" timestamptz NOT NULL DEFAULT now(),
	CONSTRAINT "organizations_slug_unique" UNIQUE ("slug")
);

CREATE TABLE "sessions" (
	"id" text PRIMARY KEY DEFAULT gen_random_uuid()::text,
	"user_id" text NOT NULL REFERENCES "users" ("id") ON DELETE CASCADE,
	"token" text NOT NULL,
	"created_at" timestamptz NOT NULL DEFAULT now(),
	"expires_at" timestamptz NOT NULL,
	"last_access" timestamptz,
	"metadata" text,
	"active_organization_id" text REFERENCES "organizations" ("id") ON DELETE SET NULL,
	CONSTRAINT "sessions_token_unique" UNIQUE ("token")
);
CREATE INDEX "sessions_user_idx" ON "sessions" ("user_id");
CREATE INDEX "idx_sessions_active_organization" ON "sessions" ("active_organization_id");

CREATE TABLE "accounts" (
	"id" text PRIMARY KEY DEFAULT gen_random_uuid()::text,
	"user_id" text NOT NULL REFERENCES "users" ("id") ON DELETE CASCADE,
	"provider" text NOT NULL,
	"provider_account_id" text NOT NULL,
	"access_token" text,
	"refresh_token" text,
	"id_token" text,
	"access_token_expires_at" timestamptz,
	"refresh_token_expires_at" timestamptz,
	"scope" text,
	"created_at" timestamptz NOT NULL DEFAULT now(),
	"updated_at" timestamptz NOT NULL DEFAULT now(),
	CONSTRAINT "accounts_provider_account_unique" UNIQUE ("provider", "provider_account_id")
);
CREATE INDEX "accounts_user_idx" ON "accounts" ("user_id");

CREATE TABLE "verifications" (
	"id" text PRIMARY KEY DEFAULT gen_random_uuid()::text,
	"subject" text NOT NULL,
	"value" text NOT NULL,
	"expires_at" timestamptz NOT NULL,
	"created_at" timestamptz NOT NULL DEFAULT now(),
	"updated_at" timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX "idx_verifications_subject" ON "verifications" ("subject");
CREATE UNIQUE INDEX "idx_verifications_value" ON "verifications" ("value");

-- Limen's own rate limiter table (distinct from Snarvei's rate_limit below).
CREATE TABLE "rate_limits" (
	"id" text PRIMARY KEY DEFAULT gen_random_uuid()::text,
	"key" text NOT NULL,
	"count" integer NOT NULL DEFAULT 0,
	"last_request_at" bigint NOT NULL DEFAULT 0,
	CONSTRAINT "rate_limits_key_unique" UNIQUE ("key")
);

CREATE TABLE "two_factors" (
	"id" text PRIMARY KEY DEFAULT gen_random_uuid()::text,
	"user_id" text NOT NULL REFERENCES "users" ("id") ON DELETE CASCADE,
	"secret" text,
	"backup_codes" text
);
CREATE UNIQUE INDEX "idx_two_factors_user_id" ON "two_factors" ("user_id");

CREATE TABLE "organization_members" (
	"id" text PRIMARY KEY DEFAULT gen_random_uuid()::text,
	"organization_id" text NOT NULL REFERENCES "organizations" ("id") ON DELETE CASCADE,
	"user_id" text NOT NULL REFERENCES "users" ("id") ON DELETE CASCADE,
	"created_at" timestamptz NOT NULL DEFAULT now(),
	"updated_at" timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX "organization_members_org_idx" ON "organization_members" ("organization_id");
CREATE INDEX "organization_members_user_idx" ON "organization_members" ("user_id");
CREATE UNIQUE INDEX "idx_organization_members_org_user" ON "organization_members" ("organization_id", "user_id");

CREATE TABLE "organization_member_roles" (
	"id" text PRIMARY KEY DEFAULT gen_random_uuid()::text,
	"member_id" text NOT NULL REFERENCES "organization_members" ("id") ON DELETE CASCADE,
	"organization_id" text NOT NULL REFERENCES "organizations" ("id") ON DELETE CASCADE,
	"role" text,
	"created_at" timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX "idx_organization_member_roles_member_role" ON "organization_member_roles" ("member_id", "role");
CREATE INDEX "organization_member_roles_org_idx" ON "organization_member_roles" ("organization_id");

CREATE TABLE "organization_invitations" (
	"id" text PRIMARY KEY DEFAULT gen_random_uuid()::text,
	"organization_id" text NOT NULL REFERENCES "organizations" ("id") ON DELETE CASCADE,
	"email" text NOT NULL,
	"roles" text,
	"status" text NOT NULL DEFAULT 'pending',
	"token" text NOT NULL,
	"expires_at" timestamptz,
	"inviter_id" text REFERENCES "users" ("id") ON DELETE CASCADE,
	"created_at" timestamptz NOT NULL DEFAULT now(),
	"updated_at" timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX "organization_invitations_org_idx" ON "organization_invitations" ("organization_id");
CREATE INDEX "organization_invitations_email_idx" ON "organization_invitations" ("email");
CREATE UNIQUE INDEX "idx_organization_invitations_token" ON "organization_invitations" ("token");

-- ===========================================================================
-- Snarvei tables (spec section 4)
-- ===========================================================================

CREATE TABLE "teams" (
	"id" text PRIMARY KEY,
	"organization_id" text NOT NULL REFERENCES "organizations" ("id") ON DELETE CASCADE,
	"name" text NOT NULL,
	"created_at" timestamptz NOT NULL DEFAULT now(),
	"updated_at" timestamptz NOT NULL DEFAULT now(),
	CONSTRAINT "teams_org_name_unique" UNIQUE ("organization_id", "name")
);
CREATE INDEX "teams_org_idx" ON "teams" ("organization_id");

CREATE TABLE "team_members" (
	"team_id" text NOT NULL REFERENCES "teams" ("id") ON DELETE CASCADE,
	"user_id" text NOT NULL REFERENCES "users" ("id") ON DELETE CASCADE,
	"created_at" timestamptz NOT NULL DEFAULT now(),
	CONSTRAINT "team_members_pk" PRIMARY KEY ("team_id", "user_id")
);
CREATE INDEX "team_members_user_idx" ON "team_members" ("user_id");

CREATE TABLE "invitation_teams" (
	"invitation_id" text PRIMARY KEY REFERENCES "organization_invitations" ("id") ON DELETE CASCADE,
	"team_id" text NOT NULL REFERENCES "teams" ("id") ON DELETE CASCADE
);

CREATE TABLE "email_change_requests" (
	"id" text PRIMARY KEY,
	"user_id" text NOT NULL REFERENCES "users" ("id") ON DELETE CASCADE,
	"new_email" text NOT NULL,
	"token_hash" text NOT NULL,
	"expires_at" timestamptz NOT NULL,
	"created_at" timestamptz NOT NULL DEFAULT now(),
	CONSTRAINT "email_change_requests_token_unique" UNIQUE ("token_hash")
);
CREATE INDEX "email_change_requests_user_idx" ON "email_change_requests" ("user_id");

CREATE TABLE "links" (
	"id" text PRIMARY KEY,
	"organization_id" text NOT NULL REFERENCES "organizations" ("id") ON DELETE CASCADE,
	"team_id" text NOT NULL REFERENCES "teams" ("id") ON DELETE CASCADE,
	"slug" text NOT NULL,
	"target_url" text NOT NULL,
	"redirect_status" smallint NOT NULL DEFAULT 302 CHECK ("redirect_status" IN (301, 302, 307)),
	"is_active" boolean NOT NULL DEFAULT true,
	"title" text,
	"description" text,
	-- Authorship is informational: links belong to the team, so deleting a
	-- user must never delete links.
	"created_by" text REFERENCES "users" ("id") ON DELETE SET NULL,
	"updated_by" text REFERENCES "users" ("id") ON DELETE SET NULL,
	"created_at" timestamptz NOT NULL DEFAULT now(),
	"updated_at" timestamptz NOT NULL DEFAULT now(),
	CONSTRAINT "links_slug_unique" UNIQUE ("slug")
);
CREATE INDEX "links_team_idx" ON "links" ("team_id");
CREATE INDEX "links_org_idx" ON "links" ("organization_id");

CREATE TABLE "link_target_history" (
	"id" text PRIMARY KEY,
	"link_id" text NOT NULL REFERENCES "links" ("id") ON DELETE CASCADE,
	"old_target_url" text,
	"new_target_url" text NOT NULL,
	"changed_by" text REFERENCES "users" ("id") ON DELETE SET NULL,
	"changed_at" timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX "link_target_history_link_idx" ON "link_target_history" ("link_id");

CREATE TABLE "click_events" (
	"id" text PRIMARY KEY,
	"link_id" text NOT NULL REFERENCES "links" ("id") ON DELETE CASCADE,
	"clicked_at" timestamptz NOT NULL DEFAULT now(),
	"ip_hash" text NOT NULL,
	"user_agent" text,
	"referer" text,
	"country" text,
	"host" text NOT NULL,
	"path" text NOT NULL,
	"query_string" text,
	"redirect_status_used" smallint NOT NULL
);
CREATE INDEX "click_events_link_idx" ON "click_events" ("link_id");
CREATE INDEX "click_events_clicked_at_idx" ON "click_events" ("clicked_at");
CREATE INDEX "click_events_link_clicked_at_idx" ON "click_events" ("link_id", "clicked_at");

-- Snarvei's own fixed-window rate limiter (redirects, invitation registration, ...).
CREATE TABLE "rate_limit" (
	"key" text PRIMARY KEY,
	"window_start" timestamptz NOT NULL,
	"count" integer NOT NULL DEFAULT 0
);

-- +goose Down

DROP TABLE "rate_limit";
DROP TABLE "click_events";
DROP TABLE "link_target_history";
DROP TABLE "links";
DROP TABLE "email_change_requests";
DROP TABLE "invitation_teams";
DROP TABLE "team_members";
DROP TABLE "teams";
DROP TABLE "organization_invitations";
DROP TABLE "organization_member_roles";
DROP TABLE "organization_members";
DROP TABLE "two_factors";
DROP TABLE "rate_limits";
DROP TABLE "verifications";
DROP TABLE "accounts";
DROP TABLE "sessions";
DROP TABLE "organizations";
DROP TABLE "users";
```

Also: `touch apps/server/internal/db/queries/.keep` and write `apps/server/sqlc.yaml` (used from phase 2 on; present now so `go generate` has a stable target):

```yaml
version: "2"
sql:
  - engine: "postgresql"
    queries: "internal/db/queries"
    schema: "internal/db/migrations"
    gen:
      go:
        package: "gen"
        out: "internal/db/gen"
        sql_package: "pgx/v5"
        emit_pointers_for_null_types: true
        # rate_limit (ours) and rate_limits (Limen's) would both singularise
        # to RateLimit; exact names keep them apart.
        emit_exact_table_names: true
```

- [ ] **Step 3: Write the failing migration test**

`apps/server/internal/db/migrate_test.go`:

```go
package db

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func testDatabaseURL() string {
	if url := os.Getenv("TEST_DATABASE_URL"); url != "" {
		return url
	}
	return "postgres://snarvei:snarvei@127.0.0.1:55432/snarvei_test"
}

func resetSchema(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), "DROP SCHEMA public CASCADE; CREATE SCHEMA public"); err != nil {
		t.Fatalf("reset schema: %v", err)
	}
}

func TestApplyMigrationsOnEmptyDatabase(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := New(ctx, testDatabaseURL())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer pool.Close()
	resetSchema(t, pool)

	if err := ApplyMigrations(ctx, testDatabaseURL(), DefaultMigrationLockKey); err != nil {
		t.Fatalf("ApplyMigrations: %v", err)
	}

	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_tables WHERE schemaname = 'public' AND tablename IN ('users','organizations','teams','links','click_events','rate_limit')`).Scan(&n); err != nil {
		t.Fatalf("count tables: %v", err)
	}
	if n != 6 {
		t.Fatalf("expected 6 known tables, found %d", n)
	}

	latest, err := LatestMigrationVersion()
	if err != nil {
		t.Fatalf("LatestMigrationVersion: %v", err)
	}
	var applied int64
	if err := pool.QueryRow(ctx, `SELECT max(version_id) FROM goose_db_version WHERE is_applied`).Scan(&applied); err != nil {
		t.Fatalf("read goose version: %v", err)
	}
	if applied != latest {
		t.Fatalf("applied %d, embedded latest %d", applied, latest)
	}

	// Idempotent: a second run finds nothing pending.
	if err := ApplyMigrations(ctx, testDatabaseURL(), DefaultMigrationLockKey); err != nil {
		t.Fatalf("second ApplyMigrations: %v", err)
	}
}

func TestApplyMigrationsHoldsTheAdvisoryLock(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := New(ctx, testDatabaseURL())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer pool.Close()

	// Hold the lock from an independent connection; ApplyMigrations must block.
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", DefaultMigrationLockKey); err != nil {
		t.Fatalf("lock: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- ApplyMigrations(ctx, testDatabaseURL(), DefaultMigrationLockKey) }()

	select {
	case err := <-done:
		t.Fatalf("ApplyMigrations returned (%v) while the lock was held", err)
	case <-time.After(2 * time.Second):
	}

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_unlock($1)", DefaultMigrationLockKey); err != nil {
		t.Fatalf("unlock: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("ApplyMigrations after unlock: %v", err)
	}
}

func TestIsUniqueViolation(t *testing.T) {
	ctx := context.Background()
	pool, err := New(ctx, testDatabaseURL())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer pool.Close()
	if err := ApplyMigrations(ctx, testDatabaseURL(), DefaultMigrationLockKey); err != nil {
		t.Fatalf("ApplyMigrations: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM organizations WHERE slug = 'dup'`); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organizations (name, slug) VALUES ('a', 'dup')`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO organizations (name, slug) VALUES ('b', 'dup')`)
	if !IsUniqueViolation(err) {
		t.Fatalf("expected a unique violation, got %v", err)
	}
	if IsUniqueViolation(context.Canceled) {
		t.Fatal("context.Canceled is not a unique violation")
	}
}
```

- [ ] **Step 4: Run the tests to verify they fail**

Run: `cd apps/server && go test ./internal/db/`
Expected: compile error, `undefined: New` (and others).

- [ ] **Step 5: Write `migrations.go`, `migrate.go`, `db.go`**

`apps/server/internal/db/migrations.go`:

```go
package db

import (
	"embed"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// migrationsFS embeds the goose migration files so the image needs no
// separate copy of the SQL.
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

// LatestMigrationVersion is the highest embedded migration version, derived
// from the filenames. testrig compares it against goose_db_version.
func LatestMigrationVersion() (int64, error) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return 0, fmt.Errorf("db: read embedded migrations: %w", err)
	}
	var latest int64
	for _, entry := range entries {
		version, _, ok := strings.Cut(entry.Name(), "_")
		if !ok {
			continue
		}
		n, err := strconv.ParseInt(version, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("db: migration %q has no numeric version prefix", entry.Name())
		}
		if n > latest {
			latest = n
		}
	}
	if latest == 0 {
		return 0, errors.New("db: no embedded migrations found")
	}
	return latest, nil
}
```

`apps/server/internal/db/migrate.go`:

```go
package db

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
	"github.com/pressly/goose/v3"
)

// DefaultMigrationLockKey is the advisory-lock key used unless
// MIGRATION_LOCK_KEY overrides it. Every process migrating the same database
// must use the same key, or they stop contending and the lock is pointless.
const DefaultMigrationLockKey int64 = 1935762089

// ApplyMigrations runs every pending goose migration under a session-level
// Postgres advisory lock, so several containers booting at once serialise
// instead of racing to apply the same DDL. It returns errors rather than
// exiting so both the default dispatch mode and `snarvei migrate` own their
// exit code.
//
// pg_advisory_lock is per physical connection, so the lock, the migration and
// the unlock must all run on the same connection: the pool here is pinned to
// exactly one.
func ApplyMigrations(ctx context.Context, databaseURL string, lockKey int64) error {
	sqlDB, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("db: open database: %w", err)
	}
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	if _, err := sqlDB.ExecContext(ctx, "SELECT pg_advisory_lock($1)", lockKey); err != nil {
		return fmt.Errorf("db: acquire advisory lock: %w", err)
	}
	defer func() { _, _ = sqlDB.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", lockKey) }()

	dir, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("db: resolve embedded migrations dir: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, dir)
	if err != nil {
		return fmt.Errorf("db: create goose provider: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("db: apply migrations: %w", err)
	}
	return nil
}
```

`apps/server/internal/db/db.go`:

```go
// Package db owns the connection pool and the embedded, advisory-locked
// goose migrations. sqlc-generated queries land in internal/db/gen from
// phase 2 on.
package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// New opens a normal multi-connection pgx pool and pings it once.
func New(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("db: open pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: ping: %w", err)
	}
	return pool, nil
}

// IsUniqueViolation reports whether err is SQLSTATE 23505. Detect by code,
// never by matching error text.
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `cd apps/server && go test -p 1 -count=1 ./internal/db/ -v`
Expected: 3 tests PASS. (`TestApplyMigrationsHoldsTheAdvisoryLock` takes about 2 s.)

- [ ] **Step 7: Write `apps/server/internal/testrig/rig.go`**

```go
// Package testrig gives every test package a real, migrated, empty Postgres.
// Setup probes goose_db_version on every call (not sync.Once: migrate_test
// drops the schema and test order is not guaranteed), truncates every public
// table except goose_db_version, and closes the pool on cleanup. Truncation is
// process-wide, so run the suite with `go test -p 1 ./...`.
package testrig

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/refsdal/snarvei/server/internal/db"
)

// DatabaseURL is TEST_DATABASE_URL or the docker-compose.test.yml default.
func DatabaseURL() string {
	if url := os.Getenv("TEST_DATABASE_URL"); url != "" {
		return url
	}
	return "postgres://snarvei:snarvei@127.0.0.1:55432/snarvei_test"
}

var migrateMu sync.Mutex

// Rig is a migrated, truncated database for one test.
type Rig struct {
	Pool *pgxpool.Pool
}

// Setup returns a Rig whose pool closes when the test ends.
func Setup(t *testing.T) *Rig {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	url := DatabaseURL()
	pool, err := db.New(ctx, url)
	if err != nil {
		t.Fatalf("testrig: open pool: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := ensureMigrated(ctx, url, pool); err != nil {
		t.Fatalf("testrig: migrate: %v", err)
	}
	if err := truncateAll(ctx, pool); err != nil {
		t.Fatalf("testrig: truncate: %v", err)
	}
	return &Rig{Pool: pool}
}

func ensureMigrated(ctx context.Context, url string, pool *pgxpool.Pool) error {
	migrateMu.Lock()
	defer migrateMu.Unlock()

	latest, err := db.LatestMigrationVersion()
	if err != nil {
		return err
	}
	var reg *string
	if err := pool.QueryRow(ctx, `SELECT to_regclass('public.goose_db_version')::text`).Scan(&reg); err != nil {
		return fmt.Errorf("probe schema: %w", err)
	}
	if reg != nil {
		var applied int64
		if err := pool.QueryRow(ctx, `SELECT COALESCE(MAX(version_id), 0) FROM goose_db_version WHERE is_applied`).Scan(&applied); err != nil {
			return fmt.Errorf("read applied version: %w", err)
		}
		if applied >= latest {
			return nil
		}
	}
	return db.ApplyMigrations(ctx, url, db.DefaultMigrationLockKey)
}

func truncateAll(ctx context.Context, pool *pgxpool.Pool) error {
	rows, err := pool.Query(ctx, `SELECT tablename FROM pg_tables WHERE schemaname = 'public' AND tablename <> 'goose_db_version'`)
	if err != nil {
		return fmt.Errorf("list tables: %w", err)
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return fmt.Errorf("scan table: %w", err)
		}
		tables = append(tables, `"`+name+`"`)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(tables) == 0 {
		return nil
	}
	_, err = pool.Exec(ctx, "TRUNCATE TABLE "+strings.Join(tables, ", ")+" RESTART IDENTITY CASCADE")
	return err
}
```

Run: `cd apps/server && go vet ./...`
Expected: no output.

- [ ] **Step 8: Commit**

```bash
git add apps/server docker-compose.test.yml
git commit -m "feat(server): embedded goose migrations, advisory-locked runner, pool and test rig"
```

---

### Task 5: Web package: embedded SPA, security headers, fallback

**Files:**
- Create: `apps/server/internal/web/web.go`, `apps/server/internal/web/web_test.go`, `apps/server/internal/web/dist/index.html`

**Interfaces:**
- Produces: `func Handler(api http.Handler) http.Handler`. Paths `/api`, `/api/*`, `/healthz`, `/readyz`, `/l/*`, `/openapi.json`, `/scalar`, `/images/*` go to `api` untouched. Everything else gets the security headers, then: `/robots.txt`, an embedded file, or `index.html`.

- [ ] **Step 1: Write the placeholder `apps/server/internal/web/dist/index.html`**

```html
<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <title>Snarvei</title>
  </head>
  <body>
    <!-- Placeholder so `go build`/`go test` work without a frontend build.
         scripts/spa-embed-overlay.sh replaces this directory with the Vite
         output before the release binaries are built. -->
    <p>SPA build lands here.</p>
  </body>
</html>
```

- [ ] **Step 2: Write the failing tests `apps/server/internal/web/web_test.go`**

```go
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
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `cd apps/server && go test ./internal/web/`
Expected: compile error, `undefined: Handler`.

- [ ] **Step 4: Write `apps/server/internal/web/web.go`**

```go
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

// CSP is the Content-Security-Policy on every non-API response. 'unsafe-inline'
// for styles is required by Emotion (MUI); data:/blob: images cover QR codes
// and profile-image previews.
const CSP = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'"

const robotsBody = "User-agent: *\nDisallow: /\n"

// serverOwnedPrefixes and serverOwnedExact are handed to the API handler
// without SPA headers. /l/, /openapi.json, /scalar and /images/ answer a JSON
// 404 until the phase that implements them lands.
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
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd apps/server && go test ./internal/web/ -v`
Expected: 4 tests PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/server/internal/web
git commit -m "feat(server): embedded SPA handler with security headers and fallback"
```

---

### Task 6: OpenAPI spec, codegen and the API package (`/healthz`, `/readyz`, `/api/config`)

**Files:**
- Create: `openapi/snarvei.yaml`, `apps/server/generate.go`, `apps/server/internal/api/gen/cfg-server.yaml`, `apps/server/internal/api/gen/cfg-types.yaml`, `apps/server/internal/api/gen/server.gen.go` (generated), `apps/server/internal/api/gen/types.gen.go` (generated), `apps/server/internal/api/snarvei.yaml` (generated copy), `apps/server/internal/api/respond/respond.go`, `apps/server/internal/api/api.go`, `apps/server/internal/api/system.go`, `apps/server/internal/api/api_test.go`, `apps/server/internal/api/spec_sync_test.go`, `apps/server/internal/api/generate_test.go`, `apps/server/internal/web/composed_test.go`

**Interfaces:**
- Consumes: `testrig.Setup`, `web.Handler`.
- Produces:
  ```go
  package api
  type Deps struct {
      Pool       *pgxpool.Pool
      AppName    string
      OpenSignup bool
      Version    string
  }
  func NewHandler(d Deps) http.Handler

  package respond
  type Envelope struct { Code string; Message string; Details map[string]any }
  func JSON(w http.ResponseWriter, status int, v any)
  func Error(w http.ResponseWriter, status int, code, message string)
  ```

- [ ] **Step 1: Write `openapi/snarvei.yaml`**

```yaml
openapi: 3.0.3
info:
  title: Snarvei API
  description: >-
    Snarvei is an organization-aware URL shortener. This file is the single
    source of truth for the server's HTTP surface (except /api/auth/*, which
    Limen serves): it drives runtime request validation, the generated Go
    strict server and the generated TypeScript client.
  version: "1.0.0"
servers:
  - url: /
paths:
  /healthz:
    get:
      operationId: healthz
      summary: Liveness probe. Touches nothing, not even the database pool.
      tags: [system]
      responses:
        "200":
          description: The process is alive.
          content:
            application/json:
              schema:
                type: object
                required: [ok, service, version]
                properties:
                  ok:
                    type: boolean
                    enum: [true]
                  service:
                    type: string
                    enum: [snarvei]
                  version:
                    type: string
  /readyz:
    get:
      operationId: readyz
      summary: Readiness probe. Runs SELECT 1 against the database pool.
      tags: [system]
      responses:
        "200":
          description: Ready.
          content:
            application/json:
              schema:
                type: object
                required: [ok]
                properties:
                  ok:
                    type: boolean
                    enum: [true]
        "503":
          description: The database is unreachable.
          content:
            application/json:
              schema:
                type: object
                required: [ok, error]
                properties:
                  ok:
                    type: boolean
                    enum: [false]
                  error:
                    type: string
  /api/config:
    get:
      operationId: getConfig
      summary: Public, unauthenticated deployment facts the SPA needs before sign-in.
      tags: [system]
      responses:
        "200":
          description: Deployment configuration.
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/PublicConfig"
components:
  schemas:
    PublicConfig:
      type: object
      required: [appName, openSignup]
      properties:
        appName:
          type: string
        openSignup:
          type: boolean
    Error:
      type: object
      required: [code, message]
      properties:
        code:
          type: string
        message:
          type: string
        details:
          type: object
          additionalProperties: true
```

- [ ] **Step 2: Write the codegen configs and `generate.go`**

`apps/server/internal/api/gen/cfg-types.yaml`:

```yaml
package: gen
output: internal/api/gen/types.gen.go
generate:
  models: true
output-options:
  skip-prune: true
```

`apps/server/internal/api/gen/cfg-server.yaml`:

```yaml
package: gen
output: internal/api/gen/server.gen.go
generate:
  std-http-server: true
  strict-server: true
output-options:
  skip-prune: true
```

`apps/server/generate.go`:

```go
// Command go generate regenerates the OpenAPI-derived code from
// openapi/snarvei.yaml (repo root). Run from apps/server with the tools from
// .mise.toml on PATH:
//
//	go generate ./...
//
// Generated code is committed; CI and the image never run codegen.
// internal/api/snarvei.yaml is a committed COPY of the root spec because
// go:embed cannot reach above the module root. Never hand-edit it.
package server

//go:generate cp ../../openapi/snarvei.yaml internal/api/snarvei.yaml
//go:generate oapi-codegen -config internal/api/gen/cfg-types.yaml ../../openapi/snarvei.yaml
//go:generate oapi-codegen -config internal/api/gen/cfg-server.yaml ../../openapi/snarvei.yaml
```

Run: `cd apps/server && mise exec -- go generate ./... && ls internal/api/gen internal/api/snarvei.yaml`
Expected: `cfg-server.yaml cfg-types.yaml server.gen.go types.gen.go` and the copied spec. `grep -n "GetConfig\|Healthz\|Readyz" internal/api/gen/server.gen.go | head` shows the three strict interface methods.

- [ ] **Step 3: Write `apps/server/internal/api/respond/respond.go`**

```go
// Package respond is the JSON error envelope every hand-written response
// uses: {"code": "...", "message": "...", "details"?: {...}}. Its own package
// so future middleware can use it without importing internal/api.
package respond

import (
	"encoding/json"
	"net/http"
)

// Envelope is the error body shape.
type Envelope struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// JSON encodes v with the given status.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Error writes the standard envelope.
func Error(w http.ResponseWriter, status int, code, message string) {
	JSON(w, status, Envelope{Code: code, Message: message})
}
```

- [ ] **Step 4: Write the failing tests**

`apps/server/internal/api/api_test.go`:

```go
package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/refsdal/snarvei/server/internal/api"
	"github.com/refsdal/snarvei/server/internal/testrig"
)

func handler(t *testing.T) (http.Handler, *testrig.Rig) {
	t.Helper()
	rig := testrig.Setup(t)
	return api.NewHandler(api.Deps{Pool: rig.Pool, AppName: "Snarvei Test", OpenSignup: false, Version: "test-sha"}), rig
}

func getJSON(t *testing.T, h http.Handler, path string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("GET %s Content-Type = %q (body %q)", path, ct, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("GET %s: not JSON: %v (%q)", path, err, rec.Body.String())
	}
	return rec, body
}

func TestHealthz(t *testing.T) {
	h := api.NewHandler(api.Deps{Pool: nil, AppName: "x", Version: "abc"}) // nil pool: healthz must not touch it
	rec, body := getJSON(t, h, "/healthz")
	if rec.Code != 200 || body["ok"] != true || body["service"] != "snarvei" || body["version"] != "abc" {
		t.Fatalf("healthz = %d %v", rec.Code, body)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("healthz Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
}

func TestReadyzTracksTheDatabase(t *testing.T) {
	h, rig := handler(t)
	rec, body := getJSON(t, h, "/readyz")
	if rec.Code != 200 || body["ok"] != true {
		t.Fatalf("readyz healthy = %d %v", rec.Code, body)
	}
	rig.Pool.Close()
	rec, body = getJSON(t, h, "/readyz")
	if rec.Code != 503 || body["ok"] != false || body["error"] == "" {
		t.Fatalf("readyz after close = %d %v", rec.Code, body)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("readyz Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
}

func TestConfigIsPublic(t *testing.T) {
	h, _ := handler(t)
	rec, body := getJSON(t, h, "/api/config")
	if rec.Code != 200 || body["appName"] != "Snarvei Test" || body["openSignup"] != false {
		t.Fatalf("config = %d %v", rec.Code, body)
	}
}

func TestUnknownAPIPathIsJSON404(t *testing.T) {
	h, _ := handler(t)
	for _, p := range []string{"/api/nope", "/api", "/l/abc", "/openapi.json", "/scalar", "/images/profile/x"} {
		rec, body := getJSON(t, h, p)
		if rec.Code != 404 || body["code"] != "NOT_FOUND" {
			t.Errorf("%s = %d %v, want 404 NOT_FOUND", p, rec.Code, body)
		}
	}
}

func TestWrongMethodIsRejectedBySpec(t *testing.T) {
	h, _ := handler(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/config", strings.NewReader("{}")))
	if rec.Code != 404 && rec.Code != 405 {
		t.Fatalf("POST /api/config = %d, want 404 or 405", rec.Code)
	}
	if !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("expected a JSON error body, got %q", rec.Body.String())
	}
}
```

`apps/server/internal/api/spec_sync_test.go`:

```go
package api_test

import (
	"bytes"
	"os"
	"testing"
)

// The embedded copy must equal the repo-root spec; go:embed happily embeds a
// stale copy otherwise.
func TestEmbeddedSpecMatchesRepoRoot(t *testing.T) {
	embedded, err := os.ReadFile("snarvei.yaml")
	if err != nil {
		t.Fatalf("read embedded copy: %v", err)
	}
	root, err := os.ReadFile("../../../../openapi/snarvei.yaml")
	if err != nil {
		t.Fatalf("read repo-root spec: %v", err)
	}
	if !bytes.Equal(embedded, root) {
		t.Fatal("internal/api/snarvei.yaml has drifted from openapi/snarvei.yaml: run `go generate ./...` from apps/server and commit")
	}
}
```

`apps/server/internal/api/generate_test.go` (drift guard for the generated Go; skips when oapi-codegen is not on PATH so `go test` works without mise, and CI installs it):

```go
package api_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Regenerates into a temp dir and diffs against the committed files.
func TestGeneratedCodeIsUpToDate(t *testing.T) {
	if _, err := exec.LookPath("oapi-codegen"); err != nil {
		t.Skip("oapi-codegen not on PATH (run `mise install`)")
	}
	tmp := t.TempDir()
	for _, c := range []struct{ cfg, out string }{
		{"gen/cfg-types.yaml", "gen/types.gen.go"},
		{"gen/cfg-server.yaml", "gen/server.gen.go"},
	} {
		cfgBytes, err := os.ReadFile(c.cfg)
		if err != nil {
			t.Fatal(err)
		}
		// Point the output at the temp dir by rewriting the config's output line.
		tmpCfg := filepath.Join(tmp, filepath.Base(c.cfg))
		tmpOut := filepath.Join(tmp, filepath.Base(c.out))
		rewritten := bytes.Replace(cfgBytes, []byte("output: internal/api/"+c.out), []byte("output: "+tmpOut), 1)
		if err := os.WriteFile(tmpCfg, rewritten, 0o644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("oapi-codegen", "-config", tmpCfg, "../../../../openapi/snarvei.yaml")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("oapi-codegen %s: %v\n%s", c.cfg, err, out)
		}
		want, _ := os.ReadFile(tmpOut)
		got, _ := os.ReadFile(c.out)
		if !bytes.Equal(want, got) {
			t.Fatalf("%s is stale: run `go generate ./...` from apps/server and commit", c.out)
		}
	}
}
```

`apps/server/internal/web/composed_test.go` (external test package so it can import both layers):

```go
package web_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/refsdal/snarvei/server/internal/api"
	"github.com/refsdal/snarvei/server/internal/testrig"
	"github.com/refsdal/snarvei/server/internal/web"
)

func composed(t *testing.T) http.Handler {
	t.Helper()
	rig := testrig.Setup(t)
	return web.Handler(api.NewHandler(api.Deps{Pool: rig.Pool, AppName: "Snarvei", OpenSignup: true, Version: "dev"}))
}

func TestProbesReachTheAPIThroughTheComposedStack(t *testing.T) {
	h := composed(t)
	for _, p := range []string{"/healthz", "/readyz", "/api/config"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
			t.Errorf("%s = %d %q", p, rec.Code, rec.Header().Get("Content-Type"))
		}
		if rec.Header().Get("Content-Security-Policy") != "" {
			t.Errorf("%s carries the SPA CSP", p)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app/anything", nil))
	if rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("SPA fallback = %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
}
```

- [ ] **Step 5: Run the tests to verify they fail**

Run: `cd apps/server && go test ./internal/api/ ./internal/web/`
Expected: compile errors, `undefined: api.NewHandler` / `undefined: api.Deps`.

- [ ] **Step 6: Write `apps/server/internal/api/api.go` and `system.go`**

`api.go`:

```go
// Package api owns every route in openapi/snarvei.yaml. NewHandler validates
// requests against the embedded spec (kin-openapi), dispatches to the
// generated strict server, and answers a JSON 404 for anything the spec does
// not know. It never reads the environment or constructs a dependency: cmd/
// snarvei hands it a Deps.
package api

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/jackc/pgx/v5/pgxpool"
	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"

	"github.com/refsdal/snarvei/server/internal/api/gen"
	"github.com/refsdal/snarvei/server/internal/api/respond"
)

// specYAML is the committed copy of openapi/snarvei.yaml (see generate.go).
//
//go:embed snarvei.yaml
var specYAML []byte

// Deps is everything the handlers need. Fields grow with each phase.
type Deps struct {
	Pool       *pgxpool.Pool
	AppName    string
	OpenSignup bool
	Version    string
}

func loadSpec() *openapi3.T {
	loader := openapi3.NewLoader()
	spec, err := loader.LoadFromData(specYAML)
	if err != nil {
		panic(fmt.Sprintf("api: parse embedded spec: %v", err))
	}
	if err := spec.Validate(loader.Context); err != nil {
		panic(fmt.Sprintf("api: embedded spec is invalid: %v", err))
	}
	return spec
}

// withSpecValidation rejects requests that do not match the spec. Unmatched
// routes fall through to next (the JSON 404); matched-but-invalid ones get
// 400 VALIDATION_FAILED.
func withSpecValidation(spec *openapi3.T, next http.Handler) http.Handler {
	validate := nethttpmiddleware.OapiRequestValidatorWithOptions(spec, &nethttpmiddleware.Options{
		DoNotValidateServers: true,
		ErrorHandlerWithOpts: func(_ context.Context, err error, w http.ResponseWriter, r *http.Request, opts nethttpmiddleware.ErrorHandlerOpts) {
			if opts.MatchedRoute == nil {
				next.ServeHTTP(w, r)
				return
			}
			respond.JSON(w, http.StatusBadRequest, respond.Envelope{
				Code: "VALIDATION_FAILED", Message: "Invalid request",
				Details: map[string]any{"reason": err.Error()},
			})
		},
	})
	return validate(next)
}

func handleNotFound(w http.ResponseWriter, _ *http.Request) {
	respond.Error(w, http.StatusNotFound, "NOT_FOUND", "Not found")
}

func requestErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	log.Printf("api: %s %s: invalid request: %v", r.Method, r.URL.Path, err)
	respond.Error(w, http.StatusBadRequest, "VALIDATION_FAILED", "Invalid request")
}

func responseErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	log.Printf("api: %s %s: %v", r.Method, r.URL.Path, err)
	respond.Error(w, http.StatusInternalServerError, "INTERNAL", "Internal error")
}

// noStore marks probe responses uncacheable.
func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

// NewHandler builds the handler for every server-owned path. web.Handler
// wraps it and is what cmd/snarvei serves.
func NewHandler(d Deps) http.Handler {
	spec := loadSpec()
	mux := http.NewServeMux()

	// Least specific: any server-owned path the spec does not know answers a
	// JSON 404 (web.Handler routes /l/, /images/, /openapi.json and /scalar
	// here too until their phases land).
	notFound := http.HandlerFunc(handleNotFound)
	mux.Handle("/", withSpecValidation(spec, notFound))

	strict := gen.NewStrictHandlerWithOptions(d, nil, gen.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  requestErrorHandler,
		ResponseErrorHandlerFunc: responseErrorHandler,
	})
	gen.HandlerWithOptions(strict, gen.StdHTTPServerOptions{
		BaseRouter: mux,
		Middlewares: []gen.MiddlewareFunc{
			func(next http.Handler) http.Handler { return withSpecValidation(spec, next) },
		},
	})

	return noStore(mux)
}
```

`system.go`:

```go
package api

import (
	"context"

	"github.com/refsdal/snarvei/server/internal/api/gen"
)

var _ gen.StrictServerInterface = Deps{}

// Healthz is pure liveness: it reads nothing, not even d.Pool.
func (d Deps) Healthz(_ context.Context, _ gen.HealthzRequestObject) (gen.HealthzResponseObject, error) {
	return gen.Healthz200JSONResponse{Ok: true, Service: "snarvei", Version: d.Version}, nil
}

// Readyz runs SELECT 1 and reports 503 on failure. The failure is a normal
// response, not an error, so it never hits the 500 path.
func (d Deps) Readyz(ctx context.Context, _ gen.ReadyzRequestObject) (gen.ReadyzResponseObject, error) {
	var one int
	if err := d.Pool.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil {
		return gen.Readyz503JSONResponse{Ok: false, Error: err.Error()}, nil
	}
	return gen.Readyz200JSONResponse{Ok: true}, nil
}

// GetConfig is public: the landing page reads it before any session exists.
func (d Deps) GetConfig(_ context.Context, _ gen.GetConfigRequestObject) (gen.GetConfigResponseObject, error) {
	return gen.GetConfig200JSONResponse{AppName: d.AppName, OpenSignup: d.OpenSignup}, nil
}
```

The exact generated type names (`gen.Healthz200JSONResponse`, `Ok` field typed as an enum) come from `server.gen.go`; open it and match the names exactly. If `Ok` is generated as an enum type (`gen.Healthz200JSONResponseBodyOkTrue`), use that constant instead of `true`.

- [ ] **Step 7: Run the tests to verify they pass**

Run: `cd apps/server && go vet ./... && go test -p 1 -count=1 ./... -v 2>&1 | tail -40`
Expected: every test in `config`, `db`, `web`, `api` passes; `TestGeneratedCodeIsUpToDate` passes (or skips outside mise).

If `TestWrongMethodIsRejectedBySpec` gets a `text/plain` 405 from the std mux, register the strict handler's routes so the spec validator answers first: the `withSpecValidation` wrapper on `/` already handles unmatched methods as "no matched route" and falls through to the JSON 404, which the test accepts.

- [ ] **Step 8: Commit**

```bash
git add openapi apps/server
git commit -m "feat(server): spec-first API package with /healthz, /readyz and /api/config"
```

---

### Task 7: `cmd/snarvei`: dispatch, serve, migrate, healthcheck

**Files:**
- Create: `apps/server/cmd/snarvei/main.go`, `apps/server/cmd/snarvei/main_test.go`

**Interfaces:**
- Consumes: `config.FromOS`, `db.ApplyMigrations`, `db.New`, `api.NewHandler`, `web.Handler`.
- Produces: the `snarvei` binary. `var version = "dev"` set by `-ldflags "-X main.version=..."`. Modes: none → migrate then serve; `server`; `migrate`/`migrations`; `healthcheck`; unknown → usage, exit 2.

- [ ] **Step 1: Write the failing dispatch tests `main_test.go`**

```go
package main

import "testing"

func TestParseArgs(t *testing.T) {
	cases := []struct {
		args []string
		want dispatchMode
	}{
		{nil, modeDefault},
		{[]string{""}, modeDefault},
		{[]string{"server"}, modeServer},
		{[]string{"migrate"}, modeMigrate},
		{[]string{"migrations"}, modeMigrate},
		{[]string{"healthcheck"}, modeHealthcheck},
		{[]string{"migrationz"}, modeUnknown},
	}
	for _, c := range cases {
		if got := parseArgs(c.args); got.mode != c.want {
			t.Errorf("parseArgs(%v) = %v, want %v", c.args, got.mode, c.want)
		}
	}
}

func TestUnknownModeExits2(t *testing.T) {
	if code := run([]string{"bogus"}); code != 2 {
		t.Fatalf("run(bogus) = %d, want 2", code)
	}
}

func TestHealthcheckAgainstNothingExits1(t *testing.T) {
	if code := healthcheckMode("1"); code != 1 {
		t.Fatalf("healthcheck on a closed port = %d, want 1", code)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd apps/server && go test ./cmd/snarvei/`
Expected: compile error, `undefined: parseArgs`.

- [ ] **Step 3: Write `main.go`**

```go
// Command snarvei is the container entrypoint and the composition root: the
// one place that reads the environment, opens the pool and hands
// collaborators to internal/api. It is also the dispatch table:
//
//	(none)                 migrate under an advisory lock, then serve
//	server                 HTTP only, never migrates (what replicas run)
//	migrate | migrations   apply migrations, exit 0/1
//	healthcheck            probe /healthz on this process, exit 0/1
//	anything else          usage, exit 2
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	// The distroless image has no zoneinfo; compile it in.
	_ "time/tzdata"

	"github.com/refsdal/snarvei/server/internal/api"
	"github.com/refsdal/snarvei/server/internal/config"
	"github.com/refsdal/snarvei/server/internal/db"
	"github.com/refsdal/snarvei/server/internal/web"
)

// version is injected with -ldflags "-X main.version=<tag or sha>".
var version = "dev"

const (
	shutdownTimeout   = 20 * time.Second
	readHeaderTimeout = 15 * time.Second
	idleTimeout       = 120 * time.Second
)

func main() { os.Exit(run(os.Args[1:])) }

type dispatchMode int

const (
	modeDefault dispatchMode = iota
	modeServer
	modeMigrate
	modeHealthcheck
	modeUnknown
)

type dispatch struct {
	mode dispatchMode
	raw  string
}

func parseArgs(args []string) dispatch {
	if len(args) == 0 || args[0] == "" {
		return dispatch{mode: modeDefault}
	}
	switch args[0] {
	case "server":
		return dispatch{mode: modeServer, raw: args[0]}
	case "migrate", "migrations":
		return dispatch{mode: modeMigrate, raw: args[0]}
	case "healthcheck":
		return dispatch{mode: modeHealthcheck, raw: args[0]}
	default:
		return dispatch{mode: modeUnknown, raw: args[0]}
	}
}

func run(args []string) int {
	d := parseArgs(args)
	switch d.mode {
	case modeHealthcheck:
		return healthcheckMode(portFromEnv())
	case modeUnknown:
		fmt.Fprintf(os.Stderr, "Unknown dispatch mode %q. Expected one of: server, migrate (or migrations), healthcheck, or no argument to migrate-then-serve.\n", d.raw)
		return 2
	case modeMigrate:
		return migrateMode()
	case modeServer:
		return serveMode(false)
	default:
		return serveMode(true)
	}
}

func portFromEnv() string {
	if p := os.Getenv("PORT"); p != "" {
		return p
	}
	return "3000"
}

// healthcheckMode constructs nothing: a liveness probe must not fail because
// DATABASE_URL is wrong. The image has no shell or curl, so the binary probes itself.
func healthcheckMode(port string) int {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/healthz")
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return 0
	}
	return 1
}

func migrateMode() int {
	cfg, err := config.FromOS()
	if err != nil {
		log.Printf("configuration error: %v", err)
		return 1
	}
	// Background, not signal-cancelled: aborting DDL halfway is worse than
	// being killed after the grace period.
	if err := db.ApplyMigrations(context.Background(), cfg.DatabaseURL, cfg.MigrationLockKey); err != nil {
		log.Printf("migration failed: %v", err)
		return 1
	}
	log.Print("migrations applied")
	return 0
}

func serveMode(migrate bool) int {
	cfg, err := config.FromOS()
	if err != nil {
		log.Printf("configuration error: %v", err)
		return 1
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(sig)

	if migrate {
		if err := db.ApplyMigrations(context.Background(), cfg.DatabaseURL, cfg.MigrationLockKey); err != nil {
			log.Printf("migration failed: %v", err)
			return 1
		}
	}

	pool, err := db.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Printf("startup failed: %v", err)
		return 1
	}
	defer pool.Close()

	deps := api.Deps{Pool: pool, AppName: cfg.AppName, OpenSignup: cfg.OpenSignup, Version: version}

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           web.Handler(api.NewHandler(deps)),
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       idleTimeout,
	}

	log.Printf("snarvei %s listening on http://0.0.0.0:%d", version, cfg.Port)
	log.Printf("  app url:  %s", cfg.AppURL)
	log.Printf("  storage:  %s", cfg.StorageDriver)
	if off := cfg.DisabledSubsystems(); len(off) > 0 {
		log.Printf("  disabled: %s", strings.Join(off, ", "))
	}

	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()

	select {
	case err := <-errc:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("server failed: %v", err)
			return 1
		}
		return 0
	case s := <-sig:
		log.Printf("%s received, shutting down", signalName(s))
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("shutdown: %v", err)
		}
		return 0
	}
}

func signalName(s os.Signal) string {
	switch s {
	case syscall.SIGTERM:
		return "SIGTERM"
	case syscall.SIGINT:
		return "SIGINT"
	default:
		return s.String()
	}
}
```

- [ ] **Step 4: Run the tests and a real boot**

Run: `cd apps/server && go vet ./... && go test ./cmd/snarvei/ -v`
Expected: 3 tests PASS.

Then boot against the test database:

```bash
cd apps/server
DATABASE_URL=postgres://snarvei:snarvei@127.0.0.1:55432/snarvei_test APP_URL=http://localhost:3000 \
  AUTH_SECRET=local-dev-secret-at-least-32-bytes-long STORAGE_DRIVER=fs STORAGE_FS_PATH=/tmp \
  go run ./cmd/snarvei &
sleep 2
curl -s localhost:3000/healthz; echo
curl -s localhost:3000/readyz; echo
curl -s localhost:3000/api/config; echo
curl -s -o /dev/null -w '%{http_code}\n' localhost:3000/api/nope
curl -s localhost:3000/ | head -3
kill %1
```

Expected: `{"ok":true,"service":"snarvei","version":"dev"}`, `{"ok":true}`, `{"appName":"Snarvei","openSignup":true}`, `404`, and the placeholder HTML. The boot log shows `disabled: email`.

- [ ] **Step 5: Commit**

```bash
git add apps/server/cmd
git commit -m "feat(server): snarvei entrypoint with migrate-then-serve, server, migrate and healthcheck modes"
```

---

### Task 8: Build scripts, Dockerfile, compose files, `.env.example`

**Files:**
- Create: `scripts/spa-embed-overlay.sh`, `scripts/restore-embed-overlay.sh`, `scripts/build-artifacts.sh`, `scripts/build-image.sh`, `Dockerfile`, `.dockerignore`, `docker/data-skel/.keep`, `docker-compose.yml`, `docker-compose.selfhost.yml`, `.env.example`

**Interfaces:**
- Consumes: `bun run build` (Task 2), `apps/server/cmd/snarvei` (Task 7).
- Produces: `dist/server/linux/{amd64,arm64}/snarvei`; image `snarvei:local`; the `BINARY_ROOT` build arg contract GoReleaser uses in phase 5.

- [ ] **Step 1: Write the overlay scripts**

`scripts/spa-embed-overlay.sh`:

```bash
#!/usr/bin/env bash
# Builds the SPA and overlays it into apps/server/internal/web/dist, where
# go:embed picks it up. Callers restore the overlay afterwards
# (scripts/restore-embed-overlay.sh) so the working tree stays clean.
set -euo pipefail
cd "$(dirname "$0")/.."

EMBED_DIR=apps/server/internal/web/dist

echo "==> SPA (vite)"
bun run build   # writes dist/client at the repo root

echo "==> embed overlay"
rm -rf "$EMBED_DIR"
mkdir -p "$EMBED_DIR"
cp -R dist/client/. "$EMBED_DIR/"
```

`scripts/restore-embed-overlay.sh`:

```bash
#!/usr/bin/env bash
# Drops the SPA overlay and puts the committed placeholder back. Idempotent;
# a no-op outside a git checkout.
set -euo pipefail
cd "$(dirname "$0")/.."

EMBED_DIR=apps/server/internal/web/dist
if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  git clean -qfd "$EMBED_DIR"
  git checkout -q -- "$EMBED_DIR"
fi
```

- [ ] **Step 2: Write `scripts/build-artifacts.sh` and `scripts/build-image.sh`**

`build-artifacts.sh`:

```bash
#!/usr/bin/env bash
# Builds everything the image COPYs, natively. Output:
#   dist/server/linux/amd64/snarvei
#   dist/server/linux/arm64/snarvei
# The SPA is embedded in both binaries; the overlay is restored on exit.
# Prerequisites: `mise install` and `bun install`.
set -euo pipefail
cd "$(dirname "$0")/.."

trap 'bash scripts/restore-embed-overlay.sh' EXIT
bash scripts/spa-embed-overlay.sh

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"

echo "==> server binaries ($VERSION)"
rm -rf dist/server
for arch in amd64 arm64; do
  mkdir -p "dist/server/linux/$arch"
  (cd apps/server && CGO_ENABLED=0 GOOS=linux GOARCH="$arch" \
    go build -trimpath -ldflags="-s -w -X main.version=$VERSION" \
    -o "../../dist/server/linux/$arch/snarvei" ./cmd/snarvei)
  echo "    dist/server/linux/$arch/snarvei"
done
ls -lh dist/server/linux/*/
```

`build-image.sh`:

```bash
#!/usr/bin/env bash
# Multi-arch image from the native artifacts. Usage:
#   bash scripts/build-image.sh                # snarvei:local, loaded for the host arch
#   PUSH=1 IMAGE=ghcr.io/refsdal/snarvei:dev bash scripts/build-image.sh
set -euo pipefail
cd "$(dirname "$0")/.."

bash scripts/build-artifacts.sh

IMAGE="${IMAGE:-snarvei:local}"
if [ "${PUSH:-0}" = "1" ]; then
  docker buildx build --platform linux/amd64,linux/arm64 -t "$IMAGE" --push .
else
  docker buildx build --load -t "$IMAGE" .
fi
```

Run: `chmod +x scripts/*.sh`

- [ ] **Step 3: Write `Dockerfile`, `.dockerignore`, `docker/data-skel/.keep`**

The digest below is the current `gcr.io/distroless/static-debian12:nonroot` manifest (resolved 2026-09-04 with `docker buildx imagetools inspect`); Dependabot keeps it fresh.

`Dockerfile`:

```dockerfile
# syntax=docker/dockerfile:1

# Snarvei: one image, one static Go binary, four modes selected by argv[1]
# (default migrate-then-serve, `server`, `migrate`, `healthcheck`). See
# apps/server/cmd/snarvei/main.go.
#
# NOTHING COMPILES IN HERE. Build natively first:
#   bash scripts/build-artifacts.sh   # -> dist/server/linux/{amd64,arm64}/snarvei
# This file only COPYs the binary matching TARGETPLATFORM, so a multi-arch
# buildx build is seconds of file copying with no QEMU.
#
# distroless "static" rather than scratch: same no-shell/no-libc surface, but
# with an up-to-date CA bundle, tzdata, /tmp and the nonroot user (uid 65532).
# Pinned by digest; Dependabot bumps it.
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab

ARG TARGETPLATFORM
# scripts/build-artifacts.sh layout by default; GoReleaser passes BINARY_ROOT=.
ARG BINARY_ROOT=dist/server

# The fs storage driver's default mountpoint, pre-created OWNED BY nonroot so
# Docker copies that ownership onto a fresh named volume.
COPY --chown=nonroot:nonroot docker/data-skel/ /data/

COPY ${BINARY_ROOT}/${TARGETPLATFORM}/snarvei /app/snarvei

ENV PORT=3000
EXPOSE 3000

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD ["/app/snarvei", "healthcheck"]

ENTRYPOINT ["/app/snarvei"]
```


`.dockerignore`:

```
# COPY-only image: allowlist exactly what the Dockerfile copies.
*
!dist/server
!docker/data-skel
```

`docker/data-skel/.keep`: an empty file (`mkdir -p docker/data-skel && touch docker/data-skel/.keep`).

- [ ] **Step 4: Write the compose files and `.env.example`**

`docker-compose.yml` (contributors, builds from `dist/server`):

```yaml
# Snarvei, whole stack, from source:
#   cp .env.example .env          # set AUTH_SECRET at minimum
#   bash scripts/build-artifacts.sh
#   docker compose up --build
name: snarvei
services:
  db:
    image: postgres:17-alpine
    restart: unless-stopped
    environment:
      POSTGRES_USER: snarvei
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:-snarvei}
      POSTGRES_DB: snarvei
    volumes:
      - db-data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U snarvei -d snarvei"]
      interval: 5s
      timeout: 5s
      retries: 20

  app:
    build: .
    image: snarvei:local
    restart: unless-stopped
    depends_on:
      db:
        condition: service_healthy
    ports:
      - "${PORT:-3000}:3000"
    volumes:
      - snarvei-data:/data
    environment: &app-env
      DATABASE_URL: postgres://snarvei:${POSTGRES_PASSWORD:-snarvei}@db:5432/snarvei
      APP_URL: ${APP_URL:-http://localhost:3000}
      AUTH_SECRET: "${AUTH_SECRET:?set AUTH_SECRET in .env (openssl rand -base64 32)}"
      STORAGE_DRIVER: fs
      STORAGE_FS_PATH: /data
      OPEN_SIGNUP: ${OPEN_SIGNUP:-1}
      TRUSTED_PROXY_HOPS: ${TRUSTED_PROXY_HOPS:-0}
      SMTP_HOST: ${SMTP_HOST:-}
      SMTP_PORT: ${SMTP_PORT:-}
      SMTP_USERNAME: ${SMTP_USERNAME:-}
      SMTP_PASSWORD: ${SMTP_PASSWORD:-}
      EMAIL_FROM: ${EMAIL_FROM:-}

  # One-off: `docker compose run --rm migrate`
  migrate:
    build: .
    image: snarvei:local
    profiles: ["tools"]
    depends_on:
      db:
        condition: service_healthy
    command: ["migrate"]
    environment: *app-env

volumes:
  db-data:
  snarvei-data:
```

`docker-compose.selfhost.yml` (operators, pulls the published image):

```yaml
# Snarvei, self-hosted, from the published image:
#   AUTH_SECRET=$(openssl rand -base64 32) docker compose -f docker-compose.selfhost.yml up -d
# Postgres for the data, a local volume for profile images (STORAGE_DRIVER=fs),
# and the app in default mode: it migrates itself under an advisory lock at
# startup, then serves. BACK UP BOTH VOLUMES.
name: snarvei
services:
  db:
    image: postgres:17-alpine
    restart: unless-stopped
    environment:
      POSTGRES_USER: snarvei
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:-snarvei}
      POSTGRES_DB: snarvei
    volumes:
      - db-data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U snarvei -d snarvei"]
      interval: 5s
      timeout: 5s
      retries: 20

  app:
    image: ghcr.io/refsdal/snarvei:${SNARVEI_TAG:-latest}
    restart: unless-stopped
    depends_on:
      db:
        condition: service_healthy
    ports:
      - "${PORT:-3000}:3000"
    volumes:
      - snarvei-data:/data
    environment:
      DATABASE_URL: postgres://snarvei:${POSTGRES_PASSWORD:-snarvei}@db:5432/snarvei
      # The public origin people type; https:// makes cookies Secure.
      APP_URL: ${APP_URL:-http://localhost:3000}
      AUTH_SECRET: "${AUTH_SECRET:?generate one with: openssl rand -base64 32}"
      STORAGE_DRIVER: fs
      STORAGE_FS_PATH: /data
      OPEN_SIGNUP: ${OPEN_SIGNUP:-1}
      # Cloudflare proxied DNS in front = 1; Cloudflare + your own reverse proxy = 2.
      TRUSTED_PROXY_HOPS: ${TRUSTED_PROXY_HOPS:-0}
      SMTP_HOST: ${SMTP_HOST:-}
      SMTP_PORT: ${SMTP_PORT:-}
      SMTP_USERNAME: ${SMTP_USERNAME:-}
      SMTP_PASSWORD: ${SMTP_PASSWORD:-}
      EMAIL_FROM: ${EMAIL_FROM:-}

volumes:
  db-data:
  snarvei-data:
```

`.env.example`:

```bash
# Snarvei configuration. Copy to .env and edit. Every variable is read by
# apps/server/internal/config, validated at startup with every problem
# reported at once. This file is the complete contract.

# ---- Required -------------------------------------------------------------

# Signs sessions and derives the IP-hash key. At least 32 bytes:
#   openssl rand -base64 32
# Changing it signs everybody out.
AUTH_SECRET=

# The public origin people type. https:// makes cookies Secure.
APP_URL=http://localhost:3000

# Postgres connection string. docker-compose.yml supplies its own.
# DATABASE_URL=postgres://snarvei:snarvei@db:5432/snarvei

# "fs" (a volume) or "s3" (any S3-compatible store) for profile images.
STORAGE_DRIVER=fs
# STORAGE_DRIVER=fs only. Must be writable by uid 65532; the image creates /data.
STORAGE_FS_PATH=/data
# STORAGE_DRIVER=s3 only: all four required, S3_REGION optional (default auto).
# S3_BUCKET=
# S3_ENDPOINT=https://s3.eu-north-1.amazonaws.com
# S3_ACCESS_KEY_ID=
# S3_SECRET_ACCESS_KEY=
# S3_REGION=auto

# ---- Optional -------------------------------------------------------------

# PORT=3000
# APP_NAME=Snarvei

# Proxies in front of the container. 0 ignores X-Forwarded-For entirely.
# Cloudflare proxied DNS = 1; Cloudflare + your own reverse proxy = 2.
# TRUSTED_PROXY_HOPS=0

# 1 (default) lets anyone register; 0 restricts accounts to invitations.
# OPEN_SIGNUP=1

# Dedicated key for hashing visitor IPs in click analytics; when unset a key
# is derived from AUTH_SECRET (rotating AUTH_SECRET then changes the hashes).
# IP_HASH_PEPPER=

# Transactional email (invitations, password reset, change-email). All five or
# none; absent means mail is dropped with a redacted log line.
# SMTP_HOST=smtp.example.com
# SMTP_PORT=587
# SMTP_USERNAME=
# SMTP_PASSWORD=
# EMAIL_FROM=Snarvei <no-reply@example.com>

# Postgres advisory-lock id for startup migrations. Never change on a live system.
# MIGRATION_LOCK_KEY=1935762089

# debug | info | warn | error
# LOG_LEVEL=info

# 1 enables test-only endpoints for the Playwright suite. Refused unless
# APP_URL is a loopback origin.
# E2E_TEST_HOOKS=0
```

- [ ] **Step 5: Build the artifacts and the image, then run it**

```bash
mise exec -- bash scripts/build-artifacts.sh
git status --short apps/server/internal/web   # must be clean: overlay restored
docker build -t snarvei:local .
docker network create snarvei-smoke || true
docker run -d --name smoke-pg --network snarvei-smoke -e POSTGRES_USER=snarvei -e POSTGRES_PASSWORD=snarvei -e POSTGRES_DB=snarvei postgres:17-alpine
for i in $(seq 1 30); do docker exec smoke-pg pg_isready -h 127.0.0.1 -U snarvei -d snarvei && break; sleep 1; done
docker run --rm --network snarvei-smoke -e DATABASE_URL=postgres://snarvei:snarvei@smoke-pg:5432/snarvei -e APP_URL=http://localhost:3000 -e AUTH_SECRET=smoke-secret-at-least-32-bytes-long-ok -e STORAGE_DRIVER=fs -e STORAGE_FS_PATH=/data snarvei:local migrate
docker run -d --name smoke-app --network snarvei-smoke -p 3000:3000 -e DATABASE_URL=postgres://snarvei:snarvei@smoke-pg:5432/snarvei -e APP_URL=http://localhost:3000 -e AUTH_SECRET=smoke-secret-at-least-32-bytes-long-ok -e STORAGE_DRIVER=fs -e STORAGE_FS_PATH=/data snarvei:local
sleep 3
curl -fsS localhost:3000/healthz; echo
curl -fsS localhost:3000/readyz; echo
curl -fsS localhost:3000/ | grep -o '<title>Snarvei</title>'
curl -fsS localhost:3000/ | grep -c 'assets/'      # the REAL SPA, not the placeholder
docker inspect --format '{{.State.Health.Status}}' smoke-app   # after ~15s: healthy
docker rm -f smoke-app smoke-pg; docker network rm snarvei-smoke
```

Expected: `migrations applied`, both probes `ok:true`, the title, at least one `assets/` reference, and `healthy`. `ls -lh dist/server/linux/amd64/snarvei` shows a binary of roughly 15 to 25 MB.

- [ ] **Step 6: Commit**

```bash
git add scripts Dockerfile .dockerignore docker docker-compose.yml docker-compose.selfhost.yml .env.example
git commit -m "feat(build): native artifacts, COPY-only distroless image, compose files and env contract"
```

---

### Task 9: CI workflows and Dependabot

**Files:**
- Create: `.github/workflows/test.yml`, `.github/workflows/ci.yml`, `.github/dependabot.yml`
- Delete: the old `.github/workflows/ci.yml` (replaced)

**Interfaces:**
- Consumes: `.mise.toml`, `bun run check|test`, `go test`, `scripts/build-artifacts.sh`, `Dockerfile`.
- Produces: the reusable `test.yml` that `release.yml` (phase 5) also calls; `ci.yml` pushes `ghcr.io/refsdal/snarvei:<next>-pr.<n>` previews.

- [ ] **Step 1: Write `.github/workflows/test.yml`**

Copy the action pins from `~/projects/refsdal/pjokk/.github/workflows/test.yml` (`actions/checkout`, `jdx/mise-action`, `actions/cache` with their `# vX.Y.Z` comments).

```yaml
name: Tests

# The single definition of "the suite is green". Reusable: ci.yml (PRs) and,
# from phase 5, release.yml (pushes to main) both call it.
on:
  workflow_call:

permissions:
  contents: read

jobs:
  tests:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:17-alpine
        env:
          POSTGRES_USER: snarvei
          POSTGRES_PASSWORD: snarvei
          POSTGRES_DB: snarvei_test
        ports:
          - 5432:5432
        options: >-
          --health-cmd "pg_isready -U snarvei -d snarvei_test"
          --health-interval 5s
          --health-timeout 5s
          --health-retries 20
    env:
      TEST_DATABASE_URL: postgres://snarvei:snarvei@127.0.0.1:5432/snarvei_test
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
      - uses: jdx/mise-action@c2a87611a18de5b3828c5652fe268e992400cb5c # v4.3.0
        with:
          install_args: "go bun go:github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen"
      - uses: actions/cache@55cc8345863c7cc4c66a329aec7e433d2d1c52a9 # v6.1.0
        with:
          path: |
            ~/go/pkg/mod
            ~/.cache/go-build
          key: ${{ runner.os }}-go-${{ hashFiles('apps/server/go.sum') }}
          restore-keys: ${{ runner.os }}-go-
      - name: Install
        run: bun install --frozen-lockfile
      - name: Lint + typecheck
        run: bun run check
      - name: Frontend tests
        run: bun run test
      - name: Go vet
        run: cd apps/server && go vet ./...
      - name: Go tests
        run: cd apps/server && go test -p 1 -count=1 ./...
```

`oapi-codegen` is installed so `TestGeneratedCodeIsUpToDate` runs rather than skips.

- [ ] **Step 2: Write `.github/workflows/ci.yml`**

```yaml
name: CI

# The PR gate: the full suite, then the image built once from native
# artifacts, smoke-tested, and pushed as a semver-prerelease preview.
on:
  pull_request:

permissions:
  contents: read

concurrency:
  group: ci-${{ github.head_ref }}
  cancel-in-progress: true

jobs:
  test:
    uses: ./.github/workflows/test.yml
    permissions:
      contents: read

  image:
    needs: test
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write
    env:
      REGISTRY: ghcr.io
      IMAGE_NAME: ${{ github.repository }}
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          fetch-depth: 0
      - uses: jdx/mise-action@c2a87611a18de5b3828c5652fe268e992400cb5c # v4.3.0
        with:
          install_args: "go bun aqua:caarlos0/svu"
      - uses: actions/cache@55cc8345863c7cc4c66a329aec7e433d2d1c52a9 # v6.1.0
        with:
          path: |
            ~/go/pkg/mod
            ~/.cache/go-build
          key: ${{ runner.os }}-go-${{ hashFiles('apps/server/go.sum') }}
          restore-keys: ${{ runner.os }}-go-
      - name: Install
        run: bun install --frozen-lockfile

      # What the next release WOULD be, so preview tags sort below it.
      - name: Compute version
        id: version
        run: |
          tag=$(svu next --v0)
          echo "version=${tag#v}" >> "$GITHUB_OUTPUT"

      - name: Build artifacts (SPA + both server binaries)
        run: VERSION=${{ steps.version.outputs.version }}-pr.${{ github.event.pull_request.number }} bash scripts/build-artifacts.sh

      - uses: docker/setup-buildx-action@37fe631027851001ddb9b187196cc803df7f5f0e # v4.3.0

      - name: Can this run push?
        id: gate
        run: |
          if [ "${{ github.event.pull_request.head.repo.full_name }}" != "${{ github.repository }}" ]; then
            echo "push=false" >> "$GITHUB_OUTPUT"
          else
            echo "push=true" >> "$GITHUB_OUTPUT"
          fi

      - name: Log in to the container registry
        if: steps.gate.outputs.push == 'true'
        uses: docker/login-action@dbcb813823bdd20940b903addbd779551569679f # v4.6.0
        with:
          registry: ${{ env.REGISTRY }}
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Build image
        run: docker build -t snarvei:ci .

      - name: Smoke-test the image
        run: |
          set -euo pipefail
          docker network create snarvei-ci
          docker run -d --name pg --network snarvei-ci \
            -e POSTGRES_USER=snarvei -e POSTGRES_PASSWORD=snarvei -e POSTGRES_DB=snarvei postgres:17-alpine
          for i in $(seq 1 30); do
            docker exec pg pg_isready -h 127.0.0.1 -U snarvei -d snarvei && break
            sleep 1
          done
          env_args=(
            -e DATABASE_URL=postgres://snarvei:snarvei@pg:5432/snarvei
            -e APP_URL=http://localhost:3000
            -e AUTH_SECRET=ci-smoke-test-secret-at-least-32-bytes-long
            -e STORAGE_DRIVER=fs
            -e STORAGE_FS_PATH=/data
          )
          # Migrations must apply from the image itself.
          docker run --rm --network snarvei-ci "${env_args[@]}" snarvei:ci migrate
          docker run -d --name app --network snarvei-ci -p 3000:3000 "${env_args[@]}" snarvei:ci
          for i in $(seq 1 30); do
            curl -fsS http://localhost:3000/healthz && break
            sleep 1
          done
          curl -fsS http://localhost:3000/readyz | grep -q '"ok":true'
          curl -fsS http://localhost:3000/api/config | grep -q '"appName":"Snarvei"'
          curl -fsS http://localhost:3000/ | grep -qi '<title>Snarvei'
          curl -fsS http://localhost:3000/ | grep -q 'assets/'
          curl -fsS http://localhost:3000/robots.txt | grep -q 'Disallow: /'
          test "$(curl -s -o /dev/null -w '%{http_code}' http://localhost:3000/api/nope)" = "404"
          # The healthcheck subcommand runs from the shell-less image.
          docker exec app /app/snarvei healthcheck

      - name: Push preview image
        if: steps.gate.outputs.push == 'true'
        run: |
          tag="${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}:${{ steps.version.outputs.version }}-pr.${{ github.event.pull_request.number }}"
          docker tag snarvei:ci "$tag"
          docker push "$tag"
          echo "Pushed $tag" >> "$GITHUB_STEP_SUMMARY"
```


- [ ] **Step 3: Write `.github/dependabot.yml`**

```yaml
version: 2
updates:
  - package-ecosystem: github-actions
    directory: /
    schedule:
      interval: weekly
    groups:
      actions:
        patterns: ["*"]
  - package-ecosystem: gomod
    directory: /apps/server
    schedule:
      interval: weekly
    groups:
      go-minor-patch:
        update-types: [minor, patch]
        patterns: ["*"]
  - package-ecosystem: npm
    directory: /
    schedule:
      interval: weekly
    groups:
      js-minor-patch:
        update-types: [minor, patch]
        patterns: ["*"]
  - package-ecosystem: docker
    directory: /
    schedule:
      interval: weekly
```

- [ ] **Step 4: Validate the workflow files locally**

Run: `bunx --package @action-validator/cli action-validator .github/workflows/test.yml .github/workflows/ci.yml` (or, if unavailable, `python3 -c "import yaml,sys; [yaml.safe_load(open(f)) for f in sys.argv[1:]]" .github/workflows/*.yml .github/dependabot.yml`).
Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add .github
git commit -m "ci: reusable test workflow, PR image smoke test and preview push, dependabot"
```

---

### Task 10: Interim docs, full verification, pull request

**Files:**
- Modify: `AGENTS.md` (prepend a banner), `README.md` (prepend a banner), `docs/runbook.md` (prepend a banner)

**Interfaces:** none.

- [ ] **Step 1: Prepend a migration banner to the three documents**

At the very top of `AGENTS.md`, `README.md` and `docs/runbook.md` insert:

```markdown
> **Migration in progress (2026-09).** Snarvei is moving from Cloudflare Workers to a
> Go server with an embedded SPA, shipped as a container. The stack, routes and
> operations described below are the OLD ones until phase 5 rewrites this file.
> The design is `docs/superpowers/specs/2026-09-04-go-backend-migration-design.md`;
> the current phase plan is under `docs/superpowers/plans/`. Backend: `apps/server`
> (Go); frontend: `apps/frontend`; build: `scripts/build-artifacts.sh`.
```

- [ ] **Step 2: Run the whole verification set**

```bash
bun install --frozen-lockfile
bun run check
bun run test
docker compose -f docker-compose.test.yml up -d --wait
(cd apps/server && mise exec -- go vet ./... && mise exec -- go test -p 1 -count=1 ./...)
mise exec -- bash scripts/build-artifacts.sh
git status --short   # only intended changes; apps/server/internal/web/dist clean
```

Expected: every command exits 0. Paste the tail of the Go test output into the PR description.

- [ ] **Step 3: Commit and open the pull request**

```bash
git add -A
git commit -m "docs: migration-in-progress banners pointing at the Go spec and plans"
git push -u origin HEAD
gh pr create --title "feat: Go server foundation with embedded SPA and container image (phase 1)" --body-file - <<'EOF'
Phase 1 of the Go migration (spec: docs/superpowers/specs/2026-09-04-go-backend-migration-design.md).

- Removes the Cloudflare Workers deployment, worker sources, Drizzle, Vitest, Playwright config; adds AGPL-3.0.
- Moves the SPA to apps/frontend on bun + biome, building to dist/client (still react-router; ported in phase 4).
- New Go module apps/server: validated config, embedded goose migrations under an advisory lock, pgx pool, test rig, embedded-SPA handler with security headers, spec-first API (/healthz, /readyz, /api/config, JSON 404), entrypoint with migrate-then-serve / server / migrate / healthcheck modes.
- COPY-only distroless image, native build scripts, compose files, .env.example.
- CI: reusable test workflow (Go against Postgres, biome, bun test), PR image smoke test and ghcr.io preview push, Dependabot.

Not in this PR: auth, links, redirect (phases 2 and 3), the TanStack Router port (phase 4), Playwright, GoReleaser and release.yml (phase 5).

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
```

- [ ] **Step 4: Watch CI**

Run: `gh pr checks --watch`
Expected: `Tests / tests` and `CI / image` both green, and the step summary names the pushed preview tag.

---

## Self-review

**Spec coverage (phase 1 items in section 11):** Workers removed (T1); LICENSE (T1); SPA on bun + biome to dist/client (T2); config (T3); pool + goose + 00001_init.sql (T4); web embed + placeholder (T5); /healthz, /readyz, /api/config with codegen (T6); dispatch + graceful shutdown (T7); Dockerfile, .dockerignore, data-skel, build scripts, compose, .mise.toml, .env.example (T3, T8); test.yml + ci.yml without e2e (T9); image boots, migrates and serves the old SPA (T8 step 5, T9 smoke test). Section 2's header set and cache rules (T5), section 8's variables and defaults (T3, T8), section 9's drift guards (T6).

**Deferred on purpose:** sqlc has no queries yet (`queries/.keep`, `sqlc.yaml` present); the `api-schema.d.ts` client and its drift guard arrive with phase 4; `/l/`, `/openapi.json`, `/scalar`, `/images/` answer the JSON 404 until phases 2 and 3.

**Type consistency:** `api.Deps{Pool, AppName, OpenSignup, Version}` is used identically in T6 tests, T6 composed test and T7. `db.ApplyMigrations(ctx, url, lockKey)` has three arguments everywhere (T4, T4 testrig, T7). `respond.Error(w, status, code, message)` has code before message in T6 (`api.go` and `respond.go`). `web.CSP` is exported and used by the T5 test. `config.Config` field names in T3 match their use in T7 (`MigrationLockKey`, `DisabledSubsystems`, `StorageDriver`).
