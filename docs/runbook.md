> **Migration in progress (2026-09).** Snarvei is moving from Cloudflare Workers to a
> Go server with an embedded SPA, shipped as a container. The stack, routes and
> operations described below are the OLD ones until phase 5 rewrites this file.
> The design is `docs/superpowers/specs/2026-09-04-go-backend-migration-design.md`;
> the current phase plan is under `docs/superpowers/plans/`. Backend: `apps/server`
> (Go); frontend: `apps/frontend`; build: `scripts/build-artifacts.sh`.

# Snarvei operations runbook

Single-operator runbook for the dev and production Cloudflare Workers environments. Keep it short and accurate; update it in the same PR as the behaviour it describes.

## Environments

|        | Production                                           | Dev                                      |
| ------ | ---------------------------------------------------- | ---------------------------------------- |
| Worker | `snarvei`                                            | `snarvei-dev` (`--env dev`)              |
| URL    | `https://snarvei.ros-nett.com`                       | `https://snarvei-dev.ros-nett.com`       |
| D1     | `snarvei`                                            | `snarvei-dev`                            |
| R2     | `snarvei-profile-images`                             | `snarvei-dev-profile-images`             |
| Deploy | manual `Deploy Production` workflow (ref + approval) | automatic after a green CI run on `main` |

Both are defined in `wrangler.jsonc` (top level = production, `env.dev` = dev). The Worker is bound to its custom domain only (`workers_dev` and `preview_urls` are off).

## Secrets and variables

Set with `pnpm exec wrangler secret put <NAME>` (add `--env dev` for dev). The Worker refuses to serve (HTTP 500 `Server misconfigured`, log event `env.invalid`) when required ones are missing.

| Name                              | Required                               | Purpose                                                                                                                                                                                 |
| --------------------------------- | -------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `AUTH_SECRET`                     | yes (>= 32 chars)                      | Better Auth session signing; default pepper for IP hashing                                                                                                                              |
| `IP_HASH_PEPPER`                  | recommended                            | Dedicated pepper for visitor IP hashing (rotating `AUTH_SECRET` then keeps analytics stable)                                                                                            |
| `EMAIL` binding + `EMAIL_FROM`    | yes for invitations/verification/reset | Transactional email via Cloudflare Email Service (`send_email` binding in `wrangler.jsonc`, `EMAIL_FROM` var); without them messages are dropped with an `email.not_configured` warning |
| `APP_URL`, `APP_NAME`, `NODE_ENV` | vars in `wrangler.jsonc`               | Public origin, display name, production guards                                                                                                                                          |
| `APP_VERSION`                     | optional var                           | Overrides the build-time git SHA reported by `/api/health`                                                                                                                              |

Rotation: Better Auth accepts a `secrets` array for overlapping rotations (see its docs); rotating `AUTH_SECRET` signs everyone out. Rotate the Cloudflare API token in GitHub Actions secrets (`CLOUDFLARE_API_TOKEN`).

## Deploy and verify

1. Merge to `main` → CI (`Validate`) → `Deploy Dev` runs only on success (workflow_run).
2. Production: run the `Deploy Production` workflow with the ref to deploy; it refuses refs without a successful `Validate` check, lists pending D1 migrations, applies them, then deploys.
3. Verify: `curl -s https://<host>/api/health` → `{"ok":true,...,"version":"<git sha>","checks":{"database":"ok"}}`; open `/scalar`; sign in and follow a short link (`/l/<slug>` → 302 with `Cache-Control: no-store`).

## Rollback and recovery

- Worker: `pnpm exec wrangler deployments list [--env dev]` → `pnpm exec wrangler rollback <version-id> [--env dev]`. Code rollback does **not** roll back migrations — migrations are additive/backward compatible by policy (see `AGENTS.md` → Database Migrations).
- Database: D1 Time Travel — `pnpm exec wrangler d1 time-travel info DB --remote [--env dev]`, then `... restore DB --remote --timestamp <iso>` (30-day window; writes after the bookmark are lost). Take a snapshot before risky production migrations: `pnpm exec wrangler d1 export DB --remote --output backup.sql`.
- R2 (profile images): no versioning; objects are only deleted when a user replaces/removes their image. Orphans are harmless.

## Common failures

| Symptom                                  | Log event / check                               | Action                                                                                                                                                                                                        |
| ---------------------------------------- | ----------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Every request 500 `Server misconfigured` | `env.invalid`                                   | A required secret/var is missing → set it, redeploy not needed                                                                                                                                                |
| `/api/health` 503                        | `health.degraded` (`checks.database`)           | D1 unreachable/broken → Cloudflare status, D1 dashboard, Time Travel if corrupted                                                                                                                             |
| Invitations / reset emails never arrive  | `email.not_configured` / `email.send_failed`    | Onboard the `EMAIL_FROM` domain under Compute → Email Service → Email Sending (Workers Paid); `E_SENDER_NOT_VERIFIED` = domain not onboarded / wrong `EMAIL_FROM`; daily/rate limit codes are Cloudflare-side |
| Users get 429                            | `Retry-After` header                            | Expected under abuse; limits are in `src/worker/lib/auth.ts` (Better Auth) and `wrangler.jsonc` (`RATE_LIMIT` binding)                                                                                        |
| Redirect works but no analytics          | `click.record_failed`                           | D1 write failure/budget → check D1 metrics & limits                                                                                                                                                           |
| Unexpected 500 on an API route           | `request.error` (has `rayId`, `userId`, `path`) | Inspect the stack in Workers Logs; correlate with `version`                                                                                                                                                   |

## Where to look

- Workers Logs (observability is enabled in `wrangler.jsonc`) — filter by `event`.
- Cloudflare dashboard: Workers & Pages → snarvei / snarvei-dev (requests, errors, CPU), D1 (reads/writes, storage), R2.
- GitHub Actions for deploy history; `/api/health` `version` tells you which commit is live.

## Local development quickstart

```
pnpm install
cp .dev.vars.example .dev.vars      # set AUTH_SECRET (>= 32 chars), APP_URL=http://localhost:5173
pnpm db:migrate:local
pnpm dev                            # Vite + Workers runtime on http://localhost:5173
pnpm test                           # unit + integration (applies real migrations to an isolated D1)
pnpm test:e2e                       # Playwright (needs: pnpm exec playwright install --with-deps chromium)
```
