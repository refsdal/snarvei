# Go Migration Phase 4: Frontend Port Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port `apps/frontend` from react-router + Better Auth + hand-written fetches to TanStack Router + TanStack Query + a generated openapi-fetch client + the limen-auth SDK, against the phase 2/3 Go API, with every screen carried over, passkeys removed, sign-up hidden when closed, and the invitation registration flow.

**Architecture:** One code-based TanStack router (`src/router.tsx`) with `beforeLoad` guards over a `['me']` query that is the single source of truth for "who is signed in" (`GET /api/me`, `null` on 401). Every server read is a `queryOptions` in `src/lib/data/`, every write a `useMutation` that invalidates the affected keys. `/api/auth/*` goes through the limen-auth client (credential-password + two-factor plugins); everything else through `openapi-fetch` typed by `src/lib/api-schema.d.ts`, generated from `openapi/snarvei.yaml`. Screens keep their MUI markup and `data-testid`s; only their data access changes. The old `WorkspaceProvider`, `App.tsx`, Better Auth and react-router are deleted in the last task, so every intermediate commit still type-checks.

**Tech Stack:** React 19, MUI 9 (+ icons, X Data Grid), Emotion, `react-qr-code`, `@tanstack/react-router` ^1.170, `@tanstack/react-query` ^5.102, `openapi-fetch` ^0.17, `openapi-typescript` 7.13.0 (via `bunx --package`), `limen-auth` ^0.1.1, bun, biome, Vite 8, Playwright (existing `e2e/`).

**Spec:** `docs/superpowers/specs/2026-09-04-go-backend-migration-design.md` (section 6 is the frontend design; section 2 lists every endpoint; section 3 the auth allowlist; section 9 testing; phase 4 in section 11). Read section 6 before Task 1.

## Global Constraints

- Branch `feat/go-migration-phase-4` is stacked on `feat/go-migration-phase-3` (PR #81, itself stacked on #80). The PR targets `feat/go-migration-phase-3` until that merges, then `main`.
- Routes (spec section 6, verbatim): `/` landing (sign in, sign up hidden when `GET /api/config` reports `openSignup=false`, forgot password, two-factor step; redirects to `/app` when signed in), `/reset-password?token=`, `/app` organization picker (session), `/app/invitations/{id}` (public: accept/reject when signed in, register when signed out and `hasAccount=false`), `/app/settings` (session), `/app/{org}/dashboard`, `/app/{org}/links`, `/app/{org}/links/{linkId}`, `/app/{org}/organization` (session + org). `{org}` is the organization slug (today's `buildOrganizationPath`: slug, falling back to id). The org layout resolves the slug against the organizations query and calls `POST /api/organizations/{orgId}/switch` when it differs from `me.session.activeOrganizationId`.
- Query keys (spec section 6, verbatim): `['me']`, `['organizations']`, `['teams', orgId]`, `['links', orgId, filters]`, `['link', id]`, `['history', id]`, `['analytics', id, days]`, `['invitations', orgId]`, `['members', orgId]`, `['sessions']`. This plan adds `['config']`, `['invitation', id]` (public invitation view) and `['teamMembers', teamId]`. Mutations invalidate the affected keys. No persistence layer, no PWA.
- Session truth is the `['me']` query (`GET /api/me`; a 401 resolves to `null`, never an error). Guards are `beforeLoad` hooks that `ensureQueryData` it. After every auth-changing call (sign in, two-factor verify, sign up, sign out, invitation register, two-factor finalize/disable, password change, account deletion) the code calls `queryClient.resetQueries()` or invalidates `['me']` as this plan states per call. The limen-auth session store is not used for rendering (its payload has no user id and no active organization); `crossTabSync: false, refetchOnWindowFocus: false` keep it silent.
- limen-auth wire facts: the SDK snake-cases request keys and merges `additionalFields` into the top-level body, so `signUp.credential({ email, password, additionalFields: { name } })` posts `{email,password,name}` and `password.reset({ token, newPassword })` posts `{token,new_password}`. A sign-in for a user with two-factor enabled answers `200 {"two_factor_required": true}` plus a challenge cookie; the SDK then calls the two-factor plugin's `onTwoFactorRedirect`; `twoFactor.verify({ code, method: "totp" })` completes the login (the server accepts a backup code in the same field). Allowed `/api/auth/*` routes (server allowlist): `GET /me`, `POST /signout`, `POST /signin/credential`, `POST /signup/credential` (only when `OPEN_SIGNUP`), `POST /passwords/change|request-reset|reset`, `POST /two-factor/initiate-setup|finalize-setup|disable|verify`, `GET /two-factor/totp/uri`, `GET|PUT /two-factor/backup-codes`. Everything else (sessions, organizations, invitations) is Snarvei's own `/api/*`.
- API error envelope: `{ code, message, details? }`; the client throws `ApiError { status, code, message, details }`. Codes seen by the UI: `VALIDATION_FAILED`, `UNAUTHENTICATED`, `FORBIDDEN`, `NOT_FOUND`, `SLUG_TAKEN`, `LAST_OWNER`, `RATE_LIMITED`, `INTERNAL`. limen-auth throws `LimenError { status, code, message }`.
- Generated client: `bun run gen:client` runs `bunx --package openapi-typescript@7.13.0 openapi-typescript openapi/snarvei.yaml -o apps/frontend/src/lib/api-schema.d.ts`; the output is committed and CI fails when it drifts. `biome.json` already excludes that file.
- TanStack navigation is typed: `navigate({ to: "/app/$org/links", params: { org: orgParams(organization).org } })` and `<Link to="/app/$org/dashboard" params={...}>`; add `export const orgParams = (organization: { id: string; slug?: string | null }) => ({ org: organization.slug ?? organization.id })` to `lib/routes.ts` (Task 3) and use it everywhere a screen navigates within an organization. The string helpers `buildOrganizationPath`/`buildLinksPath` stay for `href`s and tests.
- Keep every `data-testid` of a screen you port (the smoke suite and phase 5 flows key on them). Keep the landing copy "Short links you can trust long after they are shared." and `data-testid="sign-in-button"` (`e2e/smoke.spec.ts` asserts both).
- Removed UI: the passkeys section (`routes/settings/components/passkeys-section.tsx`) and every `passkey`/`signIn.passkey` call. Everything else on every screen carries over with the same MUI components.
- `bun run check` (biome + `tsc --noEmit` for the frontend and `e2e/`) and `bun run test` (bun tests under `apps/frontend/test`) must pass at every commit; `bun run build` must produce `dist/client`. Playwright: `E2E_REBUILD=1 mise run e2e` from the repo root (rebuilds the image; needs Docker; do not prune images or volumes).
- Conventional Commits with the two trailers `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>` and `Claude-Session: https://claude.ai/code/session_01UdGgRFBUoiwkd9PLH7zUJE`.
- Run commands from the repo root; `bun` and `go` through `mise exec --` when not on PATH; the dev loop is `bun run dev` (Vite 5173, proxies `/api`, `/l`, `/healthz`, `/readyz`, `/openapi.json`, `/scalar`, `/images`, `/robots.txt` to 3000) plus `bun run dev:server` against `docker compose -f docker-compose.test.yml up -d --wait` (Postgres on `127.0.0.1:55432`, db `snarvei_test`: `DATABASE_URL=postgres://snarvei:snarvei@localhost:55432/snarvei_test?sslmode=disable`, `APP_URL=http://localhost:5173`, `AUTH_SECRET=<32+ bytes>`, `OPEN_SIGNUP=1` — copy the rest from `.env.example`; there is no `EMAIL_DRIVER` config key, mail is a no-op unless all five `SMTP_*`/`EMAIL_FROM` vars are set). NOTE: with Vite on 5173 the browser's Origin is `http://localhost:5173`, so `APP_URL` must be exactly that for Limen to accept non-GET auth requests.

---

## File Structure

```
apps/frontend/
  package.json                       deps swapped (Task 1 adds, Task 7 removes)
  src/main.tsx                       QueryClientProvider + ThemeProvider + RouterProvider (Task 3)
  src/router.tsx                     every route, guards, org resolution (Task 3; Tasks 4-6 add children)
  src/theme.ts                       the MUI theme moved out of App.tsx (Task 3)
  src/lib/api-schema.d.ts            generated (Task 1)
  src/lib/api.ts                     openapi-fetch client, ApiError, unwrap (Task 1)
  src/lib/query.ts                   QueryClient, resetCache (Task 1)
  src/lib/auth-client.ts             limen-auth client + thin wrappers (Task 1)
  src/lib/routes.ts                  unchanged path helpers
  src/lib/data/keys.ts               query keys (Task 2)
  src/lib/data/types.ts              aliases over components["schemas"] (Task 2)
  src/lib/data/queries.ts            queryOptions + hooks (Task 2)
  src/lib/data/mutations.ts          useMutation hooks (Task 2)
  src/lib/data/index.ts              barrel (Task 2)
  src/components/app-shell.tsx       drawer on TanStack Link/useNavigate (Task 3)
  src/components/route-error.tsx     error component (Task 3)
  src/components/page-fallback.tsx   pending component (Task 3)
  src/components/message-context.tsx global snackbar/alert state (Task 3)
  src/routes/landing/page.tsx        ported (Task 3)
  src/routes/reset-password/page.tsx ported (Task 3)
  src/routes/organization-selection/page.tsx ported (Task 3)
  src/routes/dashboard/page.tsx      ported (Task 4)
  src/routes/links/page.tsx          ported (Task 4)
  src/routes/link-details/**         ported (Task 4)
  src/components/dialogs.tsx         link/team/invite dialogs on mutations (Task 4/5)
  src/routes/organization/page.tsx   ported (Task 5)
  src/components/team-members-dialog.tsx ported (Task 5)
  src/routes/invitation/page.tsx     ported + registration (Task 5)
  src/routes/settings/**             ported, passkeys removed (Task 6)
  test/api.test.ts, test/keys.test.ts, test/routes.test.ts   bun tests
  DELETED in Task 7: src/App.tsx, src/hooks/*, src/lib/api-types.ts, src/types.ts (replaced by lib/data/types.ts), routes/settings/components/passkeys-section.tsx
e2e/app.spec.ts                      browser flows (Task 7)
.github/workflows/test.yml           gen:client drift step (Task 1)
package.json                         gen:client script (Task 1)
AGENTS.md, README.md                 dev-loop and phase notes (Task 7)
```

---

### Task 1: Dependencies, generated client, HTTP and auth clients

**Files:**
- Modify: `package.json` (root: `gen:client` script), `apps/frontend/package.json` (add deps), `bun.lock`, `.github/workflows/test.yml`
- Create: `apps/frontend/src/lib/api-schema.d.ts` (generated), `apps/frontend/src/lib/api.ts`, `apps/frontend/src/lib/query.ts`, `apps/frontend/test/api.test.ts`
- Replace: `apps/frontend/src/lib/auth-client.ts` (the Better Auth client is replaced wholesale; `src/hooks/use-workspace.tsx` and the settings hooks still import the OLD names from it, so this task keeps a temporary compatibility export — see Step 5)

**Interfaces:**
- Produces:
  ```ts
  // lib/api.ts
  export const client: ReturnType<typeof createClient<paths>>;   // baseUrl "", credentials "include"
  export class ApiError extends Error { status: number; code: string; details?: Record<string, unknown> }
  export function unwrap<T>(result: OpenApiFetchResult | Response | Promise<OpenApiFetchResult | Response>): Promise<T>;
  export function errorMessage(err: unknown, fallback: string): string;   // ApiError/LimenError/Error → message
  // lib/query.ts
  export const queryClient: QueryClient;
  export async function resetCache(): Promise<void>;                     // queryClient.clear()
  // lib/auth-client.ts
  export const authClient;                                                // limen-auth react client
  export async function signInWithPassword(email: string, password: string): Promise<{ twoFactorRequired: boolean }>;
  export async function verifyTwoFactor(code: string): Promise<void>;
  export async function signUpWithPassword(name: string, email: string, password: string): Promise<void>;
  export async function signOut(): Promise<void>;
  export const password: { requestReset(email: string): Promise<void>; reset(token: string, newPassword: string): Promise<void>; change(currentPassword: string, newPassword: string): Promise<void> };
  export const twoFactor: { initiateSetup(password: string): Promise<{ uri: string }>; finalizeSetup(code: string): Promise<void>; disable(password: string): Promise<void>; getTotpUri(): Promise<{ uri: string }>; getBackupCodes(): Promise<string[]>; regenerateBackupCodes(): Promise<string[]> };
  ```

- [ ] **Step 1: Add dependencies and the generator script**

From the repo root:

```bash
cd apps/frontend && bun add @tanstack/react-router@^1.170.32 @tanstack/react-query@^5.102.1 openapi-fetch@^0.17.0 limen-auth@^0.1.1 && cd ../..
```

Do NOT remove `better-auth`, `@better-auth/passkey` or `react-router-dom` yet (Task 7). In the root `package.json` `scripts`, add:

```json
"gen:client": "bunx --package openapi-typescript@7.13.0 openapi-typescript openapi/snarvei.yaml -o apps/frontend/src/lib/api-schema.d.ts",
```

Run `bun run gen:client`; commit the generated `apps/frontend/src/lib/api-schema.d.ts` (biome already ignores it). Open it and confirm `paths["/api/me"]["get"]` and `components["schemas"]["Link"]` exist.

In `.github/workflows/test.yml`, after the `bun install --frozen-lockfile` step, add:

```yaml
      - name: Generated API client is current
        run: |
          bun run gen:client
          git diff --exit-code -- apps/frontend/src/lib/api-schema.d.ts || { echo "run 'bun run gen:client' and commit apps/frontend/src/lib/api-schema.d.ts"; exit 1; }
```

- [ ] **Step 2: Write the failing test for `unwrap`/`ApiError`**

`apps/frontend/test/api.test.ts`:

```ts
import { describe, expect, test } from "bun:test";
import { ApiError, errorMessage, unwrap } from "../src/lib/api";

const json = (status: number, body: unknown) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

describe("unwrap", () => {
  test("returns data from an openapi-fetch result", async () => {
    const out = await unwrap<{ ok: boolean }>({ data: { ok: true }, response: json(200, { ok: true }) });
    expect(out).toEqual({ ok: true });
  });

  test("throws ApiError with the envelope's code and message", async () => {
    const err = await unwrap({
      error: { code: "SLUG_TAKEN", message: "That slug is already taken" },
      response: json(409, {}),
    }).catch((e) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err.status).toBe(409);
    expect(err.code).toBe("SLUG_TAKEN");
    expect(err.message).toBe("That slug is already taken");
  });

  test("accepts a raw Response and parses the envelope", async () => {
    const err = await unwrap(json(400, { code: "VALIDATION_FAILED", message: "bad", details: { fields: { a: "x" } } })).catch(
      (e) => e,
    );
    expect(err).toBeInstanceOf(ApiError);
    expect(err.details).toEqual({ fields: { a: "x" } });
    const ok = await unwrap<{ n: number }>(json(200, { n: 1 }));
    expect(ok).toEqual({ n: 1 });
  });

  test("a 204 resolves to undefined", async () => {
    expect(await unwrap({ response: new Response(null, { status: 204 }) })).toBeUndefined();
  });

  test("a non-JSON failure becomes a generic ApiError", async () => {
    const err = await unwrap(new Response("boom", { status: 502 })).catch((e) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err.status).toBe(502);
    expect(err.code).toBe("INTERNAL");
  });
});

describe("errorMessage", () => {
  test("prefers the error's message and falls back", () => {
    expect(errorMessage(new ApiError(404, "NOT_FOUND", "Link not found"), "x")).toBe("Link not found");
    expect(errorMessage("nope", "fallback")).toBe("fallback");
  });
});
```

Run: `cd apps/frontend && bun test test/api.test.ts` → FAIL (module `../src/lib/api` not found).

- [ ] **Step 3: Write `lib/api.ts` and `lib/query.ts`**

`apps/frontend/src/lib/api.ts`:

```ts
import createClient from "openapi-fetch";
import type { paths } from "./api-schema";

// "" = same origin: the Go server serves the SPA and the API together, and
// in development Vite proxies the server-owned paths to :3000.
export const client = createClient<paths>({ baseUrl: "", credentials: "include" });

export class ApiError extends Error {
  status: number;
  code: string;
  details?: Record<string, unknown>;
  constructor(status: number, code: string, message: string, details?: Record<string, unknown>) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.details = details;
  }
}

interface OpenApiFetchResult {
  data?: unknown;
  error?: unknown;
  response: Response;
}

type Envelope = { code?: string; message?: string; details?: Record<string, unknown> };

function toApiError(status: number, body: unknown): ApiError {
  const env = (body ?? {}) as Envelope;
  return new ApiError(status, env.code ?? "INTERNAL", env.message ?? `Request failed (${status})`, env.details);
}

// openapi-fetch resolves { data, error, response } instead of throwing; unwrap
// turns that (or a raw Response, for the multipart routes that bypass the
// typed client) into "return data or throw ApiError".
export async function unwrap<T = unknown>(
  result: OpenApiFetchResult | Response | Promise<OpenApiFetchResult | Response>,
): Promise<T> {
  const r = await result;
  if (r instanceof Response) {
    if (r.status === 204) return undefined as T;
    let body: unknown = null;
    try {
      body = await r.json();
    } catch {
      body = null;
    }
    if (!r.ok) throw toApiError(r.status, body);
    return body as T;
  }
  if (r.error !== undefined || !r.response.ok) throw toApiError(r.response.status, r.error);
  if (r.response.status === 204) return undefined as T;
  return r.data as T;
}

export function errorMessage(err: unknown, fallback: string): string {
  if (err instanceof Error && err.message) return err.message;
  return fallback;
}
```

`apps/frontend/src/lib/query.ts`:

```ts
import { QueryClient } from "@tanstack/react-query";
import { ApiError } from "./api";

// 4xx answers are final (a 401 means "not signed in", a 404 "gone"); only
// network and 5xx failures are worth one retry.
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 15_000,
      retry: (count, err) => count < 1 && !(err instanceof ApiError && err.status < 500),
      refetchOnWindowFocus: false,
    },
  },
});

// Called whenever the identity behind the cache changes (sign in/out, account
// deletion): nothing of the previous user may survive.
export async function resetCache(): Promise<void> {
  queryClient.clear();
}
```

Run: `cd apps/frontend && bun test test/api.test.ts` → 6 pass.

- [ ] **Step 4: Replace `lib/auth-client.ts`**

```ts
import { createAuthClient } from "limen-auth/react";
import { credentialPasswordPlugin, twoFactorPlugin } from "limen-auth/plugins";
import { resetCache } from "./query";

// Only what the server's allowlist mounts (apps/server/internal/auth/routes.go):
// credential sign-in/sign-up, sign-out, password change/reset, two-factor.
// Sessions, organizations and invitations are Snarvei's own /api routes and
// go through lib/api.ts.
//
// The SDK's session store is deliberately unused for rendering: Limen's
// payload has no user id and no active organization, so GET /api/me (the
// ['me'] query) is the single truth. crossTabSync/refetchOnWindowFocus are
// off so the store never polls /api/auth/me on its own.
let challengePending = false;

export const authClient = createAuthClient({
  baseURL: "",
  basePath: "/api/auth",
  crossTabSync: false,
  refetchOnWindowFocus: false,
  plugins: [
    credentialPasswordPlugin(),
    twoFactorPlugin({
      onTwoFactorRedirect: () => {
        challengePending = true;
      },
    }),
  ],
});

/**
 * Email + password. Resolves { twoFactorRequired: true } when the server
 * answered with a two-factor challenge (the challenge cookie is set; call
 * verifyTwoFactor next). Throws LimenError on bad credentials.
 */
export async function signInWithPassword(email: string, password: string): Promise<{ twoFactorRequired: boolean }> {
  challengePending = false;
  try {
    await authClient.signIn.credential({ credential: email, password });
  } catch (err) {
    // The challenge response has no session; if the SDK trips over that
    // after firing onTwoFactorRedirect, the challenge still stands.
    if (!challengePending) throw err;
  }
  const required = challengePending;
  challengePending = false;
  if (!required) await resetCache();
  return { twoFactorRequired: required };
}

/** TOTP or backup code (the server recognises backup codes by shape). */
export async function verifyTwoFactor(code: string): Promise<void> {
  await authClient.twoFactor.verify({ code: code.trim(), method: "totp" });
  await resetCache();
}

export async function signUpWithPassword(name: string, email: string, password: string): Promise<void> {
  await authClient.signUp.credential({ email, password, additionalFields: { name: name.trim() } });
  await resetCache();
}

export async function signOut(): Promise<void> {
  try {
    await authClient.signout();
  } finally {
    await resetCache();
  }
}

export const password = {
  async requestReset(email: string): Promise<void> {
    await authClient.password.requestReset({ email });
  },
  async reset(token: string, newPassword: string): Promise<void> {
    await authClient.password.reset({ token, newPassword });
  },
  async change(currentPassword: string, newPassword: string): Promise<void> {
    await authClient.password.change({ currentPassword, newPassword, revokeOtherSessions: true });
  },
};

export const twoFactor = {
  async initiateSetup(password: string): Promise<{ uri: string }> {
    return authClient.twoFactor.initiateSetup({ password });
  },
  async finalizeSetup(code: string): Promise<void> {
    await authClient.twoFactor.finalizeSetup({ code: code.trim() });
  },
  async disable(password: string): Promise<void> {
    await authClient.twoFactor.disable({ password });
  },
  async getTotpUri(): Promise<{ uri: string }> {
    return authClient.twoFactor.getTotpUri();
  },
  async getBackupCodes(): Promise<string[]> {
    return authClient.twoFactor.getBackupCodes();
  },
  async regenerateBackupCodes(): Promise<string[]> {
    return authClient.twoFactor.regenerateBackupCodes();
  },
};
```

If a generated method name differs from the SDK's `as:` aliases quoted in the constraints (`signIn.credential`, `signUp.credential`, `password.requestReset`, `password.reset`, `password.change`, `twoFactor.initiateSetup`, `twoFactor.finalizeSetup`, `twoFactor.disable`, `twoFactor.verify`, `twoFactor.getTotpUri`, `twoFactor.getBackupCodes`, `twoFactor.regenerateBackupCodes`, `signout`), open `node_modules/limen-auth/dist/plugins/*/index.d.mts` and use what it declares; report the difference.

- [ ] **Step 5: Keep the old imports compiling until Task 7**

`src/hooks/use-workspace.tsx` and `src/routes/settings/hooks/use-settings-state.ts` import `authClient` from this module expecting the Better Auth shape. Rename the Better Auth client file instead of deleting it: `git mv apps/frontend/src/lib/auth-client.ts apps/frontend/src/lib/legacy-auth-client.ts` BEFORE writing the new file in Step 4, and change those two imports (and `routes/landing/page.tsx`, `routes/invitation/page.tsx`, `routes/reset-password/page.tsx`, `routes/settings/components/*.tsx` — grep `lib/auth-client`) to `legacy-auth-client`. Task 7 deletes `legacy-auth-client.ts` together with the old screens' dependencies. Run `bun run check` from the repo root: clean.

- [ ] **Step 6: Verify the auth client against the server (manual, recorded in the report)**

Start the compose Postgres and the Go server with `APP_URL=http://localhost:5173 OPEN_SIGNUP=1` (see Global Constraints), run `bun run dev`, open `http://localhost:5173`, and in the browser console:

```js
const m = await import("/src/lib/auth-client.ts");
await m.signUpWithPassword("Kari", `kari${Date.now()}@example.com`, "Testpass123");
await (await fetch("/api/me")).json();   // → { user: { name: "Kari", ... }, session: {...} }
await m.signOut();
(await fetch("/api/me")).status;         // → 401
```

Paste the three results into the report. Stop the servers.

- [ ] **Step 7: Commit**

```bash
bun run check && bun run test
git add package.json bun.lock apps/frontend .github/workflows/test.yml
git commit -m "feat(frontend): generated openapi-fetch client, TanStack Query client and limen-auth wrappers"
```

---

### Task 2: Data layer: keys, types, queries and mutations

**Files:**
- Create: `apps/frontend/src/lib/data/keys.ts`, `types.ts`, `queries.ts`, `mutations.ts`, `index.ts`, `apps/frontend/test/keys.test.ts`

**Interfaces:**
- Consumes: `client`, `unwrap`, `ApiError` (Task 1), `queryClient` (Task 1).
- Produces (exact names; Tasks 3–6 import from `../../lib/data`):
  ```ts
  export const keys: { me; config; organizations; sessions; teams(orgId); teamMembers(teamId); members(orgId); invitations(orgId); invitation(id); links(orgId, filters); link(id); history(id, page); analytics(id, days) };
  export type { Me, User, SessionInfo, PublicConfig, Organization, Member, Invitation, PublicInvitation, Team, TeamMember, Link, LinkPage, HistoryItem, HistoryPage, Analytics, SessionSummary, RedirectStatus, LinkFilters };
  export const meQueryOptions, configQueryOptions, organizationsQueryOptions, sessionsQueryOptions;  // () => queryOptions
  export function teamsQueryOptions(orgId), teamMembersQueryOptions(teamId), membersQueryOptions(orgId), invitationsQueryOptions(orgId), invitationQueryOptions(id), linksQueryOptions(orgId, filters), linkQueryOptions(id), historyQueryOptions(id, page), analyticsQueryOptions(id, days);
  export function useMe(), useConfig(), useOrganizations(), useSessions(), useTeams(orgId), useTeamMembers(teamId), useMembers(orgId), useInvitations(orgId), usePublicInvitation(id), useLinks(orgId, filters), useLink(id), useLinkHistory(id, page), useLinkAnalytics(id, days);
  export function useCreateOrganization(), useSwitchOrganization(), useCreateTeam(orgId), useAddTeamMember(teamId), useRemoveTeamMember(teamId), useCreateInvitation(orgId), useCancelInvitation(orgId), useAcceptInvitation(), useRejectInvitation(), useRegisterWithInvitation(), useCreateLink(orgId), useUpdateLink(orgId), useDeleteLink(orgId), useUpdateMe(), useUploadProfileImage(), useDeleteProfileImage(), useRequestEmailChange(), useConfirmEmailChange(), useRevokeSession(), useRevokeOtherSessions(), useDeleteMe();
  ```

- [ ] **Step 1: Write the failing keys test**

`apps/frontend/test/keys.test.ts`:

```ts
import { describe, expect, test } from "bun:test";
import { keys } from "../src/lib/data/keys";

describe("query keys", () => {
  test("match the spec's shapes", () => {
    expect(keys.me).toEqual(["me"]);
    expect(keys.organizations).toEqual(["organizations"]);
    expect(keys.sessions).toEqual(["sessions"]);
    expect(keys.teams("o1")).toEqual(["teams", "o1"]);
    expect(keys.members("o1")).toEqual(["members", "o1"]);
    expect(keys.invitations("o1")).toEqual(["invitations", "o1"]);
    expect(keys.links("o1", { page: 2, pageSize: 100, teamId: "t1" })).toEqual(["links", "o1", { page: 2, pageSize: 100, teamId: "t1" }]);
    expect(keys.link("l1")).toEqual(["link", "l1"]);
    expect(keys.history("l1", 1)).toEqual(["history", "l1", 1]);
    expect(keys.analytics("l1", 30)).toEqual(["analytics", "l1", 30]);
  });

  test("a link key is a prefix of its history and analytics keys' link scope", () => {
    // invalidating ["links", orgId] must catch every filter variant
    expect(keys.links("o1", { page: 1, pageSize: 100 }).slice(0, 2)).toEqual(["links", "o1"]);
  });
});
```

Run: `cd apps/frontend && bun test test/keys.test.ts` → FAIL.

- [ ] **Step 2: Write `keys.ts` and `types.ts`**

```ts
// lib/data/keys.ts
export type LinkFilters = { teamId?: string; page: number; pageSize: number };

export const keys = {
  me: ["me"] as const,
  config: ["config"] as const,
  organizations: ["organizations"] as const,
  sessions: ["sessions"] as const,
  teams: (orgId: string) => ["teams", orgId] as const,
  teamMembers: (teamId: string) => ["teamMembers", teamId] as const,
  members: (orgId: string) => ["members", orgId] as const,
  invitations: (orgId: string) => ["invitations", orgId] as const,
  invitation: (id: string) => ["invitation", id] as const,
  links: (orgId: string, filters: LinkFilters) => ["links", orgId, filters] as const,
  link: (id: string) => ["link", id] as const,
  history: (id: string, page: number) => ["history", id, page] as const,
  analytics: (id: string, days: number) => ["analytics", id, days] as const,
};
```

```ts
// lib/data/types.ts
import type { components } from "../api-schema";

type S = components["schemas"];
export type Me = S["Me"];
export type User = S["User"];
export type SessionInfo = S["SessionInfo"];
export type PublicConfig = S["PublicConfig"];
export type Organization = S["Organization"];
export type Member = S["Member"];
export type Invitation = S["Invitation"];
export type PublicInvitation = S["PublicInvitation"];
export type Team = S["Team"];
export type TeamMember = S["TeamMember"];
export type Link = S["Link"];
export type LinkPage = S["LinkPage"];
export type HistoryItem = S["HistoryItem"];
export type HistoryPage = S["HistoryPage"];
export type Analytics = S["Analytics"];
export type SessionSummary = S["SessionSummary"];
export type RedirectStatus = 301 | 302 | 307;
export type { LinkFilters } from "./keys";
```

- [ ] **Step 3: Write `queries.ts`**

```ts
import { queryOptions, useQuery } from "@tanstack/react-query";
import { ApiError, client, unwrap } from "../api";
import { keys, type LinkFilters } from "./keys";
import type {
  Analytics, HistoryPage, Invitation, Link, LinkPage, Me, Member, Organization, PublicConfig, PublicInvitation,
  SessionSummary, Team, TeamMember,
} from "./types";

// "Not signed in" is a value, not an error: guards and the landing page
// branch on null without tripping React Query's error state.
export const meQueryOptions = () =>
  queryOptions({
    queryKey: keys.me,
    queryFn: async (): Promise<Me | null> => {
      try {
        return await unwrap<Me>(client.GET("/api/me"));
      } catch (err) {
        if (err instanceof ApiError && err.status === 401) return null;
        throw err;
      }
    },
    staleTime: 30_000,
  });

export const configQueryOptions = () =>
  queryOptions({ queryKey: keys.config, queryFn: () => unwrap<PublicConfig>(client.GET("/api/config")), staleTime: Number.POSITIVE_INFINITY });

export const organizationsQueryOptions = () =>
  queryOptions({ queryKey: keys.organizations, queryFn: () => unwrap<Organization[]>(client.GET("/api/organizations")) });

export const sessionsQueryOptions = () =>
  queryOptions({ queryKey: keys.sessions, queryFn: () => unwrap<SessionSummary[]>(client.GET("/api/me/sessions")) });

export const teamsQueryOptions = (orgId: string) =>
  queryOptions({
    queryKey: keys.teams(orgId),
    queryFn: () => unwrap<Team[]>(client.GET("/api/organizations/{orgId}/teams", { params: { path: { orgId } } })),
  });

export const teamMembersQueryOptions = (teamId: string) =>
  queryOptions({
    queryKey: keys.teamMembers(teamId),
    queryFn: () => unwrap<TeamMember[]>(client.GET("/api/teams/{teamId}/members", { params: { path: { teamId } } })),
  });

export const membersQueryOptions = (orgId: string) =>
  queryOptions({
    queryKey: keys.members(orgId),
    queryFn: () => unwrap<Member[]>(client.GET("/api/organizations/{orgId}/members", { params: { path: { orgId } } })),
  });

export const invitationsQueryOptions = (orgId: string) =>
  queryOptions({
    queryKey: keys.invitations(orgId),
    queryFn: () => unwrap<Invitation[]>(client.GET("/api/organizations/{orgId}/invitations", { params: { path: { orgId } } })),
  });

export const invitationQueryOptions = (id: string) =>
  queryOptions({
    queryKey: keys.invitation(id),
    queryFn: () => unwrap<PublicInvitation>(client.GET("/api/invitations/{invitationId}", { params: { path: { invitationId: id } } })),
    retry: false,
  });

export const linksQueryOptions = (orgId: string, filters: LinkFilters) =>
  queryOptions({
    queryKey: keys.links(orgId, filters),
    queryFn: () =>
      unwrap<LinkPage>(
        client.GET("/api/links", { params: { query: { organizationId: orgId, teamId: filters.teamId, page: filters.page, pageSize: filters.pageSize } } }),
      ),
  });

export const linkQueryOptions = (id: string) =>
  queryOptions({ queryKey: keys.link(id), queryFn: () => unwrap<Link>(client.GET("/api/links/{linkId}", { params: { path: { linkId: id } } })) });

export const historyQueryOptions = (id: string, page: number) =>
  queryOptions({
    queryKey: keys.history(id, page),
    queryFn: () => unwrap<HistoryPage>(client.GET("/api/links/{linkId}/history", { params: { path: { linkId: id }, query: { page, pageSize: 100 } } })),
  });

export const analyticsQueryOptions = (id: string, days: number) =>
  queryOptions({
    queryKey: keys.analytics(id, days),
    queryFn: () => unwrap<Analytics>(client.GET("/api/links/{linkId}/analytics", { params: { path: { linkId: id }, query: { days } } })),
  });

export const useMe = () => useQuery(meQueryOptions());
export const useConfig = () => useQuery(configQueryOptions());
export const useOrganizations = () => useQuery(organizationsQueryOptions());
export const useSessions = () => useQuery(sessionsQueryOptions());
export const useTeams = (orgId: string) => useQuery(teamsQueryOptions(orgId));
export const useTeamMembers = (teamId: string) => useQuery(teamMembersQueryOptions(teamId));
export const useMembers = (orgId: string) => useQuery(membersQueryOptions(orgId));
export const useInvitations = (orgId: string) => useQuery(invitationsQueryOptions(orgId));
export const usePublicInvitation = (id: string) => useQuery(invitationQueryOptions(id));
export const useLinks = (orgId: string, filters: LinkFilters) => useQuery(linksQueryOptions(orgId, filters));
export const useLink = (id: string) => useQuery(linkQueryOptions(id));
export const useLinkHistory = (id: string, page = 1) => useQuery(historyQueryOptions(id, page));
export const useLinkAnalytics = (id: string, days = 30) => useQuery(analyticsQueryOptions(id, days));
```

The exact path-parameter names (`orgId`, `teamId`, `linkId`, `invitationId`, `sessionId`) come from `openapi/snarvei.yaml`; `api-schema.d.ts` will fail to type-check if one is wrong — fix the name, not the type.

- [ ] **Step 4: Write `mutations.ts`**

Every hook is `useMutation` with `mutationFn` and `onSuccess` invalidation. Full list (body types from `api-schema.d.ts`'s `requestBody` for the operation; write them as `paths["/api/links"]["post"]["requestBody"]["content"]["application/json"]` aliases at the top of the file):

```ts
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { client, unwrap } from "../api";
import { resetCache } from "../query";
import { keys } from "./keys";
import type { Invitation, Link, Me, Organization, Team } from "./types";
import type { paths } from "../api-schema";

type Body<P extends keyof paths, M extends keyof paths[P]> = paths[P][M] extends { requestBody: { content: { "application/json": infer B } } } ? B : never;

export function useCreateOrganization() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Body<"/api/organizations", "post">) => unwrap<Organization>(client.POST("/api/organizations", { body })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.organizations }),
  });
}

export function useSwitchOrganization() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (orgId: string) => unwrap<void>(client.POST("/api/organizations/{orgId}/switch", { params: { path: { orgId } } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.me }),
  });
}

export function useCreateTeam(orgId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Body<"/api/organizations/{orgId}/teams", "post">) =>
      unwrap<Team>(client.POST("/api/organizations/{orgId}/teams", { params: { path: { orgId } }, body })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.teams(orgId) }),
  });
}

export function useAddTeamMember(teamId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (userId: string) => unwrap<void>(client.POST("/api/teams/{teamId}/members", { params: { path: { teamId } }, body: { userId } })),
    onSuccess: () => Promise.all([qc.invalidateQueries({ queryKey: keys.teamMembers(teamId) }), qc.invalidateQueries({ queryKey: ["teams"] })]),
  });
}

export function useRemoveTeamMember(teamId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (userId: string) => unwrap<void>(client.DELETE("/api/teams/{teamId}/members/{userId}", { params: { path: { teamId, userId } } })),
    onSuccess: () => Promise.all([qc.invalidateQueries({ queryKey: keys.teamMembers(teamId) }), qc.invalidateQueries({ queryKey: ["teams"] })]),
  });
}

export function useCreateInvitation(orgId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Body<"/api/organizations/{orgId}/invitations", "post">) =>
      unwrap<Invitation>(client.POST("/api/organizations/{orgId}/invitations", { params: { path: { orgId } }, body })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.invitations(orgId) }),
  });
}

export function useCancelInvitation(orgId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (invitationId: string) =>
      unwrap<void>(client.DELETE("/api/organizations/{orgId}/invitations/{invitationId}", { params: { path: { orgId, invitationId } } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.invitations(orgId) }),
  });
}

export function useAcceptInvitation() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (invitationId: string) => unwrap<unknown>(client.POST("/api/invitations/{invitationId}/accept", { params: { path: { invitationId } } })),
    onSuccess: () => Promise.all([qc.invalidateQueries({ queryKey: keys.organizations }), qc.invalidateQueries({ queryKey: keys.me })]),
  });
}

export function useRejectInvitation() {
  return useMutation({
    mutationFn: (invitationId: string) => unwrap<unknown>(client.POST("/api/invitations/{invitationId}/reject", { params: { path: { invitationId } } })),
  });
}

export function useRegisterWithInvitation() {
  return useMutation({
    mutationFn: ({ invitationId, ...body }: { invitationId: string } & Body<"/api/invitations/{invitationId}/register", "post">) =>
      unwrap<Me>(client.POST("/api/invitations/{invitationId}/register", { params: { path: { invitationId } }, body })),
    onSuccess: () => resetCache(),
  });
}

export function useCreateLink(orgId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Body<"/api/links", "post">) => unwrap<Link>(client.POST("/api/links", { body })),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["links", orgId] }),
  });
}

export function useUpdateLink(orgId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ linkId, ...body }: { linkId: string } & Body<"/api/links/{linkId}", "patch">) =>
      unwrap<Link>(client.PATCH("/api/links/{linkId}", { params: { path: { linkId } }, body })),
    onSuccess: (link) =>
      Promise.all([
        qc.invalidateQueries({ queryKey: ["links", orgId] }),
        qc.invalidateQueries({ queryKey: keys.link(link.id) }),
        qc.invalidateQueries({ queryKey: ["history", link.id] }),
      ]),
  });
}

export function useDeleteLink(orgId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (linkId: string) => unwrap<void>(client.DELETE("/api/links/{linkId}", { params: { path: { linkId } } })),
    onSuccess: (_, linkId) => Promise.all([qc.invalidateQueries({ queryKey: ["links", orgId] }), qc.removeQueries({ queryKey: keys.link(linkId) })]),
  });
}

export function useUpdateMe() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Body<"/api/me", "patch">) => unwrap<Me>(client.PATCH("/api/me", { body })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.me }),
  });
}

// The profile-image routes are hand-mounted outside the OpenAPI document
// (apps/server/internal/api/images.go): multipart field "file", ≤ 2 MiB,
// png/jpeg/webp, answering { imageUrl: string | null }. They bypass the typed
// client and go through unwrap's raw-Response path.
export function useUploadProfileImage() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (file: File) => {
      const form = new FormData();
      form.append("file", file);
      return unwrap<{ imageUrl: string | null }>(fetch("/api/me/profile-image", { method: "POST", body: form, credentials: "include" }));
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.me }),
  });
}

export function useDeleteProfileImage() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => unwrap<{ imageUrl: null }>(fetch("/api/me/profile-image", { method: "DELETE", credentials: "include" })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.me }),
  });
}

export function useRequestEmailChange() {
  return useMutation({ mutationFn: (body: Body<"/api/me/email", "post">) => unwrap<void>(client.POST("/api/me/email", { body })) });
}

export function useConfirmEmailChange() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Body<"/api/me/email/confirm", "post">) => unwrap<Me>(client.POST("/api/me/email/confirm", { body })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.me }),
  });
}

export function useRevokeSession() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (sessionId: string) => unwrap<void>(client.DELETE("/api/me/sessions/{sessionId}", { params: { path: { sessionId } } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.sessions }),
  });
}

export function useRevokeOtherSessions() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => unwrap<void>(client.DELETE("/api/me/sessions")),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.sessions }),
  });
}

export function useDeleteMe() {
  return useMutation({
    mutationFn: (body: Body<"/api/me", "delete">) => unwrap<void>(client.DELETE("/api/me", { body })),
    onSuccess: () => resetCache(),
  });
}
```

Body shapes: `POST /api/me/email` is `{ newEmail, password }`, `POST /api/me/email/confirm` is `{ token }`, `DELETE /api/me` is `{ password }`, `PATCH /api/me` is `{ name }`, `POST /api/invitations/{invitationId}/register` is `{ name, password }` (answers `Me`, 201). `POST /api/organizations/{orgId}/switch` answers 204.

- [ ] **Step 5: Barrel, check, commit**

`lib/data/index.ts`: `export * from "./keys"; export * from "./types"; export * from "./queries"; export * from "./mutations";`

```bash
bun run check && bun run test
git add apps/frontend
git commit -m "feat(frontend): TanStack Query data layer over the generated client"
```

---

### Task 3: Router, shells, guards, landing, organization picker, reset page

**Files:**
- Create: `apps/frontend/src/router.tsx`, `apps/frontend/src/theme.ts`, `apps/frontend/src/components/route-error.tsx`, `apps/frontend/src/components/page-fallback.tsx`, `apps/frontend/src/components/message-context.tsx`
- Modify: `apps/frontend/src/main.tsx`, `apps/frontend/src/components/app-shell.tsx`, `apps/frontend/src/routes/landing/page.tsx`, `apps/frontend/src/routes/organization-selection/page.tsx`, `apps/frontend/src/routes/reset-password/page.tsx`, `apps/frontend/src/components/dialogs.tsx` (only `CreateOrganizationDialog`'s `onSubmit` contract stays as is; no change needed unless it imports `useWorkspace`), `apps/frontend/test/routes.test.ts` (add a test for `afterAuthPath`)
- Note: `src/App.tsx` stays on disk (unused) until Task 7; the old screens still compile against `legacy-auth-client` and `use-workspace`.

**Interfaces:**
- Consumes: Task 1 (`queryClient`, `resetCache`, auth wrappers, `errorMessage`), Task 2 (`meQueryOptions`, `organizationsQueryOptions`, `configQueryOptions`, `useMe`, `useOrganizations`, `useConfig`, `useCreateOrganization`, `useSwitchOrganization`, `client`, `unwrap`).
- Produces (Tasks 4–6 add child routes to these):
  ```ts
  // router.tsx
  export const rootRoute, indexRoute, resetPasswordRoute, invitationRoute, appRoute, appIndexRoute, settingsRoute, orgRoute;
  export const router;                       // createRouter({ routeTree, context: { queryClient } })
  export type RouterContext = { queryClient: QueryClient };
  // orgRoute.beforeLoad returns { organization: Organization } — children read it with `orgRoute.useRouteContext()` (or `useRouteContext({ from: orgRoute.id })`)
  // lib/routes.ts
  export function afterAuthPath(next: string | null | undefined): string;   // "/app/..." → next, else "/app"
  // components/message-context.tsx
  export function MessageProvider({ children }); export function useMessage(): { message: AppMessage | null; setMessage(m: AppMessage | null): void };
  export type AppMessage = { severity: "success" | "error" | "info"; text: string };
  ```

- [ ] **Step 1: `afterAuthPath` test and helper**

Append to `apps/frontend/test/routes.test.ts`:

```ts
import { afterAuthPath } from "../src/lib/routes";

describe("afterAuthPath", () => {
  test("honours in-app destinations only", () => {
    expect(afterAuthPath("/app/invitations/abc")).toBe("/app/invitations/abc");
    expect(afterAuthPath("/app")).toBe("/app");
    expect(afterAuthPath("https://evil.example/app/x")).toBe("/app");
    expect(afterAuthPath("//evil.example")).toBe("/app");
    expect(afterAuthPath(null)).toBe("/app");
    expect(afterAuthPath("/settings")).toBe("/app");
  });
});
```

Add to `src/lib/routes.ts`:

```ts
/** Where to go after sign-in/sign-up: a `next` that is an in-app path, else the picker. */
export const afterAuthPath = (next: string | null | undefined): string =>
  next === "/app" || (typeof next === "string" && next.startsWith("/app/")) ? next : "/app";
```

Run `cd apps/frontend && bun test` → all pass.

- [ ] **Step 2: Theme, fallback, error and message components**

`src/theme.ts`: move the `createTheme({...})` block from `App.tsx` verbatim and `export const theme`.

`src/components/page-fallback.tsx`:

```tsx
import { Box, CircularProgress } from "@mui/material";

export const PageFallback = ({ fullScreen = false }: { fullScreen?: boolean }) => (
  <Box sx={{ minHeight: fullScreen ? "100vh" : 240, display: "grid", placeItems: "center" }}>
    <CircularProgress />
  </Box>
);
```

`src/components/route-error.tsx`: the `RouteError` component from `App.tsx`, rewritten for TanStack: `export function RouteError({ error }: { error: unknown })` (TanStack passes `{ error, reset }` to `errorComponent`); title `"Something went wrong"`, detail `error instanceof Error ? error.message : null`; keep the Reload and "Go to workspace" (`href="/app"`) buttons. Also `export function NotFound()` rendering the same layout with title `"Page not found"` and only the workspace button.

`src/components/message-context.tsx`: a context holding `{ message, setMessage }` (state in `MessageProvider`), the `AppMessage` type, and `useMessage()` that throws if used outside the provider. The app shell renders the `<Alert>` from it exactly as today (`message ? <Alert severity onClose={() => setMessage(null)}>` above the `Outlet`).

- [ ] **Step 3: `router.tsx`**

```tsx
import type { QueryClient } from "@tanstack/react-query";
import {
  createRootRouteWithContext,
  createRoute,
  createRouter,
  lazyRouteComponent,
  Outlet,
  redirect,
} from "@tanstack/react-router";
import { AppShell } from "./components/app-shell";
import { PageFallback } from "./components/page-fallback";
import { NotFound, RouteError } from "./components/route-error";
import { client, unwrap } from "./lib/api";
import { meQueryOptions, organizationsQueryOptions } from "./lib/data";
import { LandingPage } from "./routes/landing/page";

export type RouterContext = { queryClient: QueryClient };

export const rootRoute = createRootRouteWithContext<RouterContext>()({
  component: () => <Outlet />,
  errorComponent: RouteError,
  notFoundComponent: NotFound,
});

type LandingSearch = { next?: string; forgot?: string; reset?: string };
const str = (v: unknown) => (typeof v === "string" ? v : undefined);

// "/" — signed-in visitors go straight to the app; the landing page keeps its
// ?next / ?forgot=1 / ?reset=done search params.
export const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  validateSearch: (s: Record<string, unknown>): LandingSearch => ({ next: str(s.next), forgot: str(s.forgot), reset: str(s.reset) }),
  beforeLoad: async ({ context, search }) => {
    const me = await context.queryClient.ensureQueryData(meQueryOptions());
    if (me) throw redirect({ to: afterAuthPath(search.next), replace: true });
  },
  component: LandingPage,
});

export const resetPasswordRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/reset-password",
  validateSearch: (s: Record<string, unknown>): { token?: string; error?: string } => ({ token: str(s.token), error: str(s.error) }),
  component: lazyRouteComponent(() => import("./routes/reset-password/page"), "ResetPasswordPage"),
});

// Public on purpose (spec §6): an invitee without an account registers here.
export const invitationRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/app/invitations/$invitationId",
  component: lazyRouteComponent(() => import("./routes/invitation/page"), "InvitationPage"),
});

// Everything else under /app needs a session. The destination survives the
// round trip through the landing page as ?next=.
export const appRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/app",
  beforeLoad: async ({ context, location }) => {
    const me = await context.queryClient.ensureQueryData(meQueryOptions());
    if (!me) {
      throw redirect({ to: "/", search: location.pathname.startsWith("/app/") ? { next: location.href } : {}, replace: true });
    }
    return { me };
  },
  component: AppShell,
  pendingComponent: () => <PageFallback fullScreen />,
});

export const appIndexRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/",
  component: lazyRouteComponent(() => import("./routes/organization-selection/page"), "OrganizationSelectionPage"),
});

export const settingsRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/settings",
  validateSearch: (s: Record<string, unknown>): { emailToken?: string } => ({ emailToken: str(s.emailToken) }),
  component: lazyRouteComponent(() => import("./routes/settings/page"), "SettingsPage"),
});

// /app/$org: resolve the slug (or id) and make it the active organization
// server-side, so every /api call that reads the session's active org agrees
// with the URL. Unknown slug → picker.
export const orgRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/$org",
  beforeLoad: async ({ context, params }) => {
    const organizations = await context.queryClient.ensureQueryData(organizationsQueryOptions());
    const organization = organizations.find((o) => o.slug === params.org || o.id === params.org);
    if (!organization) throw redirect({ to: "/app", replace: true });
    if (context.me.session.activeOrganizationId !== organization.id) {
      await unwrap<void>(client.POST("/api/organizations/{orgId}/switch", { params: { path: { orgId: organization.id } } }));
      await context.queryClient.invalidateQueries({ queryKey: meQueryOptions().queryKey });
    }
    return { organization };
  },
  component: () => <Outlet />,
  pendingComponent: PageFallback,
});

// Children of orgRoute are added by Tasks 4 and 5:
//   /$org/dashboard, /$org/links, /$org/links/$linkId, /$org/organization

const routeTree = rootRoute.addChildren([
  indexRoute,
  resetPasswordRoute,
  invitationRoute,
  appRoute.addChildren([appIndexRoute, settingsRoute, orgRoute.addChildren([])]),
]);

export const router = createRouter({
  routeTree,
  context: { queryClient: undefined as unknown as QueryClient }, // provided in main.tsx
  defaultPendingComponent: PageFallback,
  defaultPreload: "intent",
});

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
```

Import `afterAuthPath` from `./lib/routes`. The `settingsRoute` and `invitationRoute` components point at pages that still use the OLD hooks at this task's commit; that is fine for type-checking (they are lazy modules) and they are ported in Tasks 5 and 6. Do not mount the old `App.tsx` anywhere.

`src/main.tsx`:

```tsx
import { CssBaseline, ThemeProvider } from "@mui/material";
import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { MessageProvider } from "./components/message-context";
import { queryClient } from "./lib/query";
import { router } from "./router";
import { theme } from "./theme";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <ThemeProvider theme={theme}>
        <CssBaseline />
        <MessageProvider>
          <RouterProvider router={router} context={{ queryClient }} />
        </MessageProvider>
      </ThemeProvider>
    </QueryClientProvider>
  </StrictMode>,
);
```

- [ ] **Step 4: Port `components/app-shell.tsx`**

Replace the react-router imports with `Link`, `Outlet`, `useLocation`, `useNavigate`, `useParams` from `@tanstack/react-router` (`useParams({ strict: false })` gives `{ org?: string }`). Replace `useWorkspace()` with: `const { data: me } = useMe(); const { data: organizations = [] } = useOrganizations(); const { message, setMessage } = useMessage(); const switchOrganization = useSwitchOrganization(); const navigate = useNavigate();`. `activeOrganizationId = me?.session.activeOrganizationId ?? null`; `session.user` → `me.user`. Remove the `useEffect` that switched organizations (the org route's `beforeLoad` does it now). The organization `Select` navigates with `navigate({ to: buildOrganizationPath(nextOrganization) })` (the route resolves and switches). `ListItemButton component={Link} to={item.to}` works with TanStack's `Link` (pass `to` as a string). Sign-out: `void signOut().then(() => navigate({ to: "/" }))` using `signOut` from `lib/auth-client`. Keep the drawer markup, nav items and copy.

- [ ] **Step 5: Port the landing page**

Replace `useWorkspace`/`authClient` with: `useConfig()` (hide the "create account"/sign-up toggle and `auth-name-input` when `config?.openSignup === false`; while config loads treat as hidden), `signInWithPassword`, `signUpWithPassword`, `verifyTwoFactor`, `password.requestReset` from `lib/auth-client`, `useMessage()`, `indexRoute.useSearch()` for `next`/`forgot`/`reset`, `useNavigate()`; after a successful sign-in/sign-up/two-factor verify: `await queryClient.invalidateQueries()` is already done by `resetCache()` inside the wrappers, then `navigate({ to: afterAuthPath(next), replace: true })`. Errors: `setMessage({ severity: "error", text: errorMessage(err, "Sign-in failed.") })`. The two-factor step keeps both `totp` and `backup` method buttons and `two-factor-code-input`; both submit through `verifyTwoFactor(code)`. Keep every `data-testid` (`auth-email-input`, `auth-name-input`, `auth-password-input`, `sign-in-button`, `create-account-button`, `forgot-password-email-input`, `forgot-password-button`, `two-factor-code-input`). Replace the three chips `"Cloudflare Workers"`, `"Better Auth organizations + teams"`, `"OpenAPI + Scalar"` with `"Self-hosted, one container"`, `"Organizations + teams"`, `"OpenAPI + Scalar"` and the sentence "…through a single Cloudflare-native control plane." with "…from one small self-hosted service."; keep the headline. Remove the `if (session)` redirect (the route does it) and the `sessionPending` spinner.

- [ ] **Step 6: Port the organization picker and the reset page**

`routes/organization-selection/page.tsx`: `useOrganizations()` (spinner while `isPending`), `useMe()` for `activeOrganizationId`, `useCreateOrganization()`; "Open workspace" → `navigate({ to: buildOrganizationPath(organization) })`; the create dialog's `onSubmit` → `createOrganization.mutateAsync(values)` then `navigate({ to: buildOrganizationPath(created) })`, `SLUG_TAKEN`/validation errors → `setMessage`. Drop the `if (!session)` redirect. `submitting` → `createOrganization.isPending`.

`routes/reset-password/page.tsx`: `resetPasswordRoute.useSearch()` for `token`/`error`; `password.reset(token, newPassword)`; on success `navigate({ to: "/", search: { reset: "done" }, replace: true })`; the "Back to sign in" link becomes TanStack `Link to="/"`. Keep `reset-password-input`, `reset-password-confirm-input`, `reset-password-button` and the comment about the emailed link (Limen's mail links to `/reset-password?token=…`).

- [ ] **Step 7: Verify in the browser, commit**

`bun run check && bun run test`. Then with the dev servers up (Task 1 Step 6 settings): open `/`, sign up, land on `/app`, create an organization, land on `/app/<slug>/dashboard` (renders the app shell with an empty outlet — the dashboard route arrives in Task 4, so the URL shows the shell and no content; that is expected at this commit). Sign out from the drawer → `/`. Record the steps in the report.

```bash
git add apps/frontend
git commit -m "feat(frontend): TanStack Router with session and organization guards; landing, picker and reset pages on the new clients"
```

---

### Task 4: Dashboard, links table and link details on the data layer

**Files:**
- Modify: `apps/frontend/src/router.tsx` (add `dashboardRoute`, `linksRoute`, `linkDetailsRoute` under `orgRoute`), `apps/frontend/src/routes/dashboard/page.tsx`, `apps/frontend/src/routes/links/page.tsx`, `apps/frontend/src/routes/link-details/page.tsx`, `apps/frontend/src/routes/link-details/components/link-history-card.tsx`, `apps/frontend/src/routes/link-details/components/link-analytics-card.tsx`, `apps/frontend/src/components/dialogs.tsx` (`CreateLinkDialog`, `EditLinkDialog`, `CreateOrganizationDialog` stay presentational; remove any `useWorkspace` import), `apps/frontend/src/components/copy-button.tsx` (unchanged unless it imports react-router)

**Interfaces:**
- Consumes: `orgRoute` context `{ organization }`, `useLinks`, `useLink`, `useLinkHistory`, `useLinkAnalytics`, `useTeams`, `useMembers`, `useInvitations`, `useCreateLink`, `useUpdateLink`, `useDeleteLink`, `useMessage`, `buildLinksPath`, `buildOrganizationPath`.
- Produces: routes `/app/$org/dashboard`, `/app/$org/links`, `/app/$org/links/$linkId`.

- [ ] **Step 1: Routes**

In `router.tsx` add, and put them in `orgRoute.addChildren([...])`:

```tsx
export const dashboardRoute = createRoute({
  getParentRoute: () => orgRoute,
  path: "/dashboard",
  component: lazyRouteComponent(() => import("./routes/dashboard/page"), "DashboardPage"),
});
export const linksRoute = createRoute({
  getParentRoute: () => orgRoute,
  path: "/links",
  validateSearch: (s: Record<string, unknown>): { teamId?: string; page?: number } => ({
    teamId: str(s.teamId),
    page: typeof s.page === "number" && s.page >= 1 ? s.page : undefined,
  }),
  component: lazyRouteComponent(() => import("./routes/links/page"), "LinksPage"),
});
export const linkDetailsRoute = createRoute({
  getParentRoute: () => orgRoute,
  path: "/links/$linkId",
  component: lazyRouteComponent(() => import("./routes/link-details/page"), "LinkDetailsPage"),
});
```

- [ ] **Step 2: Dashboard**

`const { organization } = orgRoute.useRouteContext();` then `useLinks(organization.id, { page: 1, pageSize: 100 })`, `useTeams(organization.id)`, `useMembers(organization.id)`, `useInvitations(organization.id)`. Stat cards: links → `links.data?.total ?? 0`, teams → `teams.data?.length ?? 0`, members → `members.data?.length ?? 0`, pending invitations → `invitations.data?.filter(i => i.status === "pending").length ?? 0`; `recentLinks = links.data?.items.slice(0, 5) ?? []`; `loadingLinks` → `links.isPending`. Navigation via `useNavigate()` from TanStack with `navigate({ to: buildLinksPath(organization) })`. Keep the `dashboard-*-count` test ids and the rest of the markup.

- [ ] **Step 3: Links table**

`const { organization } = orgRoute.useRouteContext(); const { teamId, page = 1 } = linksRoute.useSearch();` `useTeams(organization.id)`, `useLinks(organization.id, { teamId, page, pageSize: 100 })`, `useCreateLink(organization.id)`. Team filter select (if the page has one) writes `teamId` to the search params with `navigate({ to: ".", search: (prev) => ({ ...prev, teamId }) })`; DataGrid `paginationMode="server"`, `rowCount={data.total}`, `paginationModel={{ page: page - 1, pageSize: 100 }}`, `onPaginationModelChange={(m) => navigate({ to: ".", search: (prev) => ({ ...prev, page: m.page + 1 }) })}`. `appOrigin` → `window.location.origin`. Create dialog `onSubmit` → `createLink.mutateAsync({ teamId, targetUrl, redirectStatus, title, description, slug: slug || undefined })`; on `ApiError` with code `SLUG_TAKEN` show its message via `setMessage` and keep the dialog open (return `false`); on success navigate to `buildLinksPath(organization, link.id)`. `activeTeamId` default → `teams.data?.[0]?.id ?? null`. Keep `create-link-*` test ids.

- [ ] **Step 4: Link details, history, analytics**

`const { linkId } = linkDetailsRoute.useParams(); const { organization } = orgRoute.useRouteContext();` `useLink(linkId)` (404 → `NotFound` layout), `useUpdateLink(organization.id)`, `useDeleteLink(organization.id)`; delete → `navigate({ to: buildLinksPath(organization) })`. Remove `setSelectedLinkId`/`getLinkById`/`useEffect`. `LinkHistoryCard` takes `linkId` and calls `useLinkHistory(linkId)` (`data.items`; show `data.total`); `LinkAnalyticsCard` takes `linkId`, keeps its `days` selector state (7/30/90 — keep whatever the current component offers) and calls `useLinkAnalytics(linkId, days)`; keep `analytics-total-clicks` and `analytics-unique-visitors`. Edit dialog → `updateLink.mutateAsync({ linkId, ...values })` where blank title/description are sent as `""` (the server clears on blank). Keep `selected-link-*`, `save-link-button`, `delete-link-button`.

- [ ] **Step 5: Verify, commit**

`bun run check && bun run test`; in the browser: create a link, open it, retarget it, see two history rows, open `/l/<slug>` in a new tab, reload the details page and see 1 click. Delete it.

```bash
git add apps/frontend
git commit -m "feat(frontend): dashboard, links table and link details on TanStack Query"
```

---

### Task 5: Organization page, team members dialog, invitation page with registration

**Files:**
- Modify: `apps/frontend/src/router.tsx` (add `organizationRoute` under `orgRoute`), `apps/frontend/src/routes/organization/page.tsx`, `apps/frontend/src/components/team-members-dialog.tsx`, `apps/frontend/src/components/dialogs.tsx` (`CreateTeamDialog`, `InviteMemberDialog` stay presentational), `apps/frontend/src/routes/invitation/page.tsx`

**Interfaces:**
- Consumes: `orgRoute` context, `useMembers`, `useTeams`, `useTeamMembers`, `useInvitations`, `useCreateTeam`, `useAddTeamMember`, `useRemoveTeamMember`, `useCreateInvitation`, `useCancelInvitation`, `usePublicInvitation`, `useAcceptInvitation`, `useRejectInvitation`, `useRegisterWithInvitation`, `useMe`, `useOrganizations`, `useMessage`, `invitationRoute`, `afterAuthPath`.
- Produces: route `/app/$org/organization`; the invitation page at `/app/invitations/$invitationId`.

- [ ] **Step 1: Route**

```tsx
export const organizationRoute = createRoute({
  getParentRoute: () => orgRoute,
  path: "/organization",
  component: lazyRouteComponent(() => import("./routes/organization/page"), "OrganizationPage"),
});
```

Add it to `orgRoute.addChildren([...])`.

- [ ] **Step 2: Organization page**

`const { organization } = orgRoute.useRouteContext();` `useMembers(organization.id)`, `useTeams(organization.id)`, `useInvitations(organization.id)`, `useCreateTeam(organization.id)`, `useCreateInvitation(organization.id)`, `useCancelInvitation(organization.id)`. The members grid columns read the flat `Member` (`row.name`, `row.email`, `row.role`) instead of `row.user.*`. `roleLabel` moves from `src/types.ts` into this page (or `lib/data/roles.ts` if the invitation page needs it too — it does; create `src/lib/roles.ts` with `roleLabel(role: string): string` and the `InvitationRole = "member" | "admin"` type; owner is never assignable through the UI). Owner/admin gating: `const canManage = organization.role === "owner" || organization.role === "admin"`; hide `open-invite-member-button`, `open-create-team-button` and the cancel buttons when `!canManage` (the server enforces it anyway). Invite dialog `onSubmit` → `createInvitation.mutateAsync({ email, role, teamId: teamId || undefined })`; create team → `createTeam.mutateAsync({ name })`; cancel → `cancelInvitation.mutateAsync(invitation.id)`. `teamName(teamId)` → `invitation.teamName` (the API returns it). Keep every `data-testid` (`open-invite-member-button`, `invite-email-input`, `invite-team-select`, `invite-team-option-<name>`, `send-invitation-button`, `open-create-team-button`, `team-name-input`, `create-team-button`, `manage-team-<name>`, `invitation-<email>`, `cancel-invitation-<email>`).

- [ ] **Step 3: Team members dialog**

Props become `{ team: Team | null; organizationId: string; members: Member[]; onClose }`; inside: `useTeamMembers(team.id)` (enabled when `team` is set: pass `""` and `enabled: Boolean(team)` via `useQuery({ ...teamMembersQueryOptions(team?.id ?? ""), enabled: Boolean(team) })`), `useAddTeamMember(team.id)`, `useRemoveTeamMember(team.id)`. The "add" select lists org members not yet in the team (`members.filter(m => !teamMembers.some(t => t.userId === m.userId))`, option test id `add-team-member-option-<email>`, `add-team-member-select`, `add-team-member-button`, list `team-members-list`, remove `remove-team-member-<userId>`). Drop `onChanged` (invalidation replaces it).

- [ ] **Step 4: Invitation page**

```
const { invitationId } = invitationRoute.useParams();
const invitation = usePublicInvitation(invitationId);       // PublicInvitation | 404 → "not found" alert
const me = useMe();                                           // null → signed out
const organizations = useOrganizations() (enabled: Boolean(me.data))
```

States:
- loading → spinner; `NOT_FOUND`/error → `Alert` "This invitation could not be found. It may have expired or been cancelled." (`invitation-card` wrapper stays).
- `invitation.status !== "pending"` → alert "This invitation is no longer valid." (status may be `accepted`, `rejected`, `cancelled`, `expired`).
- signed in → card with `invitation-organization` (org name), inviter, role (`roleLabel`), team name, `invitation-accept-button` → `acceptInvitation.mutateAsync(id)` then `setMessage({ severity: "success", text: \`You joined ${organizationName}.\` })` and `navigate({ to: "/app" })` (the picker lists the new org; `me` is invalidated by the mutation); a `403 FORBIDDEN` (email mismatch) shows the server's message. `invitation-decline-button` → reject then `navigate({ to: "/app" })`.
- signed out and `hasAccount === true` → text "Sign in with the invited address to continue" and a `Link to="/" search={{ next: \`/app/invitations/${invitationId}\` }}` button "Sign in".
- signed out and `hasAccount === false` → registration form: `auth-name-input`, `auth-password-input` (min 8), `create-account-button` → `registerWithInvitation.mutateAsync({ invitationId, name, password })` → `navigate({ to: "/app" })`. `409 EMAIL_TAKEN` → message "An account with that email exists; sign in instead." plus the sign-in link. `410 INVITATION_INVALID` → the "no longer valid" alert.

- [ ] **Step 5: Verify, commit**

`bun run check && bun run test`. Browser: as an owner create a team, invite a fresh address with the team, copy the invitation link from the server log or `GET /api/_test/mail` (run the dev server with `E2E_TEST_HOOKS=1`), open it in a private window, register, land on the picker, open the org, see the team on the organization page.

```bash
git add apps/frontend
git commit -m "feat(frontend): organization, team members and invitation screens on the API, with invitee registration"
```

---

### Task 6: Settings on the API and limen-auth, passkeys removed

**Files:**
- Modify: `apps/frontend/src/routes/settings/page.tsx`, `apps/frontend/src/routes/settings/hooks/use-settings-state.ts`, `apps/frontend/src/routes/settings/components/{profile,email,password,sessions,two-factor}-section.tsx`, `apps/frontend/src/routes/settings/components/types.ts`, `apps/frontend/src/routes/settings/components/index.ts`
- Delete: `apps/frontend/src/routes/settings/components/passkeys-section.tsx`
- Create: `apps/frontend/src/routes/settings/components/danger-section.tsx` (account deletion; the spec's `DELETE /api/me` has no screen today — add it here as a small card: password field + "Delete my account" button with a confirm dialog)

**Interfaces:**
- Consumes: `useMe`, `useSessions`, `useUpdateMe`, `useUploadProfileImage`, `useDeleteProfileImage`, `useRequestEmailChange`, `useConfirmEmailChange`, `useRevokeSession`, `useRevokeOtherSessions`, `useDeleteMe`, `password`, `twoFactor`, `signOut` (auth-client), `settingsRoute.useSearch()` (`emailToken`), `useMessage`.

- [ ] **Step 1: Shared props**

`components/types.ts`:

```ts
import type { AppMessage } from "../../../components/message-context";
import type { Me } from "../../../lib/data";

export type ActionRunner = (action: string, work: () => Promise<void>) => Promise<void>;
export type SharedSectionProps = {
  me: Me;
  busyAction: string | null;
  setMessage: (message: AppMessage) => void;
  runAction: ActionRunner;
};
```

`use-settings-state.ts` keeps `busyAction`, `runAction` (sets busy, runs, catches → `setMessage({ severity: "error", text: errorMessage(err, "Something went wrong.") })`, clears busy), `profileName` synced from `me.user.name`, `newEmail`, `currentPassword`, `newPassword`, `twoFactorPassword`, `twoFactorCode`, `totpUri`, `backupCodes`. Remove sessions/passkeys state (sessions come from `useSessions()`).

- [ ] **Step 2: Sections**

- **Profile**: `useUpdateMe()` for the name (`settings-name-input`), `useUploadProfileImage()` on the file input (`file` field is inside the hook), `useDeleteProfileImage()`; avatar `src={me.user.image ?? undefined}`. Success messages as today.
- **Email**: `useRequestEmailChange()` with `{ newEmail, password }` — add a `settings-email-password-input` password field to the card (the API requires the current password); on `202` show "Check the new email address to confirm the change." On mount, if `settingsRoute.useSearch().emailToken` is set, call `useConfirmEmailChange().mutateAsync({ token })` once (guard with a ref), show "Email address updated." or the error, then `navigate({ to: "/app/settings", search: {}, replace: true })`. The confirmation mail from the server links to `/app/settings?emailToken=…` (see `internal/email` templates; if the query key differs there, use the server's name and note it).
- **Password**: `password.change(currentPassword, newPassword)`; on success message "Password updated. Other sessions were signed out." and `queryClient.invalidateQueries({ queryKey: keys.sessions })`. Keep `settings-current-password-input`, `settings-new-password-input`.
- **Sessions**: `useSessions()`; rows show `userAgent`, `createdAt`, `lastAccess`, a "Current" chip when `current`; revoke one → `useRevokeSession()`; "Sign out other sessions" → `useRevokeOtherSessions()`.
- **Two-factor**: enabled state from `me.user.twoFactorEnabled`. Enable flow: password → `twoFactor.initiateSetup(password)` → `setTotpUri(uri)` → render `react-qr-code` + the secret URI → code (`settings-2fa-code-input`) → `twoFactor.finalizeSetup(code)` → `backupCodes = await twoFactor.getBackupCodes()` shown once → `queryClient.invalidateQueries({ queryKey: keys.me })`. When enabled: "Show backup codes" (`getBackupCodes`), "Regenerate backup codes" (`regenerateBackupCodes`), "Disable" with password (`twoFactor.disable`) then invalidate `me`. Limen answers 404 on `totp/uri` and `backup-codes` before `initiate-setup` has run — only offer those buttons when enabled.
- **Danger**: password field + button; `useDeleteMe().mutateAsync({ password })`; `409 LAST_OWNER` → show the server message ("transfer ownership or delete the organization first"); success → `navigate({ to: "/" })`.
- `page.tsx`: `const me = useMe().data!` (the route guarantees a session), render the sections without `PasskeysSection`; the sign-out button → `signOut().then(() => navigate({ to: "/" }))`. Update the subtitle to "Manage your profile, security settings and active logins."

- [ ] **Step 3: Verify, commit**

`bun run check && bun run test`. Browser: change name, upload and remove a photo, change password (then sign in again in another tab and see two sessions; revoke the other), enable two-factor with an authenticator app, sign out, sign in → two-factor step → backup code works; disable two-factor.

```bash
git add apps/frontend
git commit -m "feat(frontend): settings on the API and limen-auth; passkeys removed; account deletion"
```

---

### Task 7: Remove the old stack, browser e2e flows, docs

**Files:**
- Delete: `apps/frontend/src/App.tsx`, `apps/frontend/src/hooks/use-workspace.tsx`, `apps/frontend/src/hooks/use-workspace-context.ts`, `apps/frontend/src/hooks/workspace-context.ts`, `apps/frontend/src/lib/legacy-auth-client.ts`, `apps/frontend/src/lib/api-types.ts`, `apps/frontend/src/types.ts` (move anything still imported — `SelectedLinkFormValues`, `roleLabel` if not yet moved — next to its only consumer)
- Modify: `apps/frontend/package.json` (remove `better-auth`, `@better-auth/passkey`, `react-router-dom`), `bun.lock`, `e2e/smoke.spec.ts` (only if a selector changed), `AGENTS.md`, `README.md`, `.github/dependabot.yml` (nothing to change unless it lists removed packages)
- Create: `e2e/app.spec.ts`

- [ ] **Step 1: Delete and uninstall**

```bash
git rm apps/frontend/src/App.tsx apps/frontend/src/hooks/use-workspace.tsx apps/frontend/src/hooks/use-workspace-context.ts apps/frontend/src/hooks/workspace-context.ts apps/frontend/src/lib/legacy-auth-client.ts apps/frontend/src/lib/api-types.ts
cd apps/frontend && bun remove better-auth @better-auth/passkey react-router-dom && cd ../..
grep -rn "react-router-dom\|better-auth\|use-workspace\|api-types\|legacy-auth" apps/frontend/src   # must print nothing
bun run check && bun run test && bun run build
```

`src/types.ts`: delete it once no file imports it (grep); `SelectedLinkFormValues` goes to `components/dialogs.tsx` and is exported from there.

- [ ] **Step 2: Browser flows `e2e/app.spec.ts`**

```ts
import { expect, test, type Page } from "@playwright/test";

const unique = () => Math.random().toString(36).slice(2, 10);
const PASSWORD = "Playwright123";

async function signUp(page: Page, name: string, email: string) {
  await page.goto("/");
  await page.getByTestId("create-account-button").click();        // toggles the sign-up form
  await page.getByTestId("auth-name-input").fill(name);
  await page.getByTestId("auth-email-input").fill(email);
  await page.getByTestId("auth-password-input").fill(PASSWORD);
  // Limen throttles sign-up 5/10 s per IP; the form shows the 429 message, so retry through it.
  for (let attempt = 0; attempt < 8; attempt++) {
    await page.getByTestId("create-account-button").click();
    const outcome = await Promise.race([
      page.waitForURL(/\/app(\?|$|\/)/, { timeout: 5000 }).then(() => "ok" as const),
      page.getByText(/too many|rate/i).first().waitFor({ timeout: 5000 }).then(() => "throttled" as const),
    ]).catch(() => "unknown" as const);
    if (outcome === "ok") return;
    await page.waitForTimeout(2500);
  }
  throw new Error("sign-up kept being throttled");
}

async function createOrganization(page: Page, name: string, slug: string) {
  await page.getByRole("button", { name: "Create organization" }).click();
  await page.getByTestId("organization-name-input").fill(name);
  await page.getByTestId("organization-slug-input").fill(slug);
  await page.getByTestId("create-organization-button").click();
  await page.waitForURL(`**/app/${slug}/dashboard`);
}

test("sign up, create an organization and a team, create a link, follow it, see the click", async ({ page, context }) => {
  const slug = `acme-${unique()}`;
  await signUp(page, "Kari", `kari-${unique()}@example.com`);
  await createOrganization(page, "Acme", slug);
  await expect(page.getByTestId("dashboard-links-count")).toHaveText("0");

  await page.goto(`/app/${slug}/organization`);
  await page.getByTestId("open-create-team-button").click();
  await page.getByTestId("team-name-input").fill("Marketing");
  await page.getByTestId("create-team-button").click();
  await expect(page.getByTestId("manage-team-Marketing")).toBeVisible();

  await page.goto(`/app/${slug}/links`);
  await page.getByRole("button", { name: /create link/i }).first().click();
  await page.getByTestId("create-link-target-input").fill("https://example.com/launch");
  await page.getByTestId("create-link-title-input").fill("Launch");
  await page.getByTestId("create-link-button").click();
  await page.waitForURL(/\/links\/[^/]+$/);
  const shortUrl = await page.getByText(/\/l\/[A-Za-z2-9]{8}/).first().textContent();
  const shortPath = shortUrl?.match(/\/l\/[A-Za-z2-9]{8}/)?.[0];
  expect(shortPath).toBeTruthy();

  const visitor = await context.newPage();
  const hop = await visitor.request.get(shortPath!, { maxRedirects: 0 });
  expect(hop.status()).toBe(302);
  expect(hop.headers().location).toBe("https://example.com/launch");
  await visitor.close();

  await page.reload();
  await expect(page.getByTestId("analytics-total-clicks")).toHaveText("1", { timeout: 10_000 });

  await page.getByRole("button", { name: /edit/i }).first().click();
  await page.getByTestId("selected-link-target-input").fill("https://example.com/v2");
  await page.getByTestId("save-link-button").click();
  await expect(page.getByText("https://example.com/v2")).toBeVisible();
});

test("wrong password shows an error; sign out returns to the landing page", async ({ page }) => {
  const email = `ola-${unique()}@example.com`;
  await signUp(page, "Ola", email);
  await page.getByRole("button", { name: "Sign out" }).click();
  await page.waitForURL("/");
  await page.getByTestId("auth-email-input").fill(email);
  await page.getByTestId("auth-password-input").fill("wrong-password");
  await page.getByTestId("sign-in-button").click();
  await expect(page.getByRole("alert")).toBeVisible();
  await expect(page).toHaveURL("/");
});

test("an invitee registers through the emailed link and lands in the organization", async ({ page, browser, request }) => {
  const slug = `acme-${unique()}`;
  await signUp(page, "Owner", `owner-${unique()}@example.com`);
  await createOrganization(page, "Acme", slug);
  await request.delete("/api/_test/mail");
  const invitee = `new-${unique()}@example.com`;
  await page.goto(`/app/${slug}/organization`);
  await page.getByTestId("open-invite-member-button").click();
  await page.getByTestId("invite-email-input").fill(invitee);
  await page.getByTestId("send-invitation-button").click();
  await expect(page.getByTestId(`invitation-${invitee}`)).toBeVisible();

  const mail = await (await request.get("/api/_test/mail")).json();
  const link = (mail.messages[0].text as string).match(/\/app\/invitations\/[A-Za-z0-9-]+/)?.[0];
  expect(link).toBeTruthy();

  const guest = await browser.newContext();
  const gp = await guest.newPage();
  await gp.goto(link!);
  await expect(gp.getByTestId("invitation-organization")).toHaveText("Acme");
  await gp.getByTestId("auth-name-input").fill("New Person");
  await gp.getByTestId("auth-password-input").fill(PASSWORD);
  await gp.getByTestId("create-account-button").click();
  await gp.waitForURL(/\/app(\?|$|\/)/);
  await gp.getByRole("button", { name: "Open workspace" }).click();
  await gp.waitForURL(`**/app/${slug}/dashboard`);
  await guest.close();
});
```

Adjust selectors to the ported markup where the plan's guesses (`getByRole("button", { name: ... })`) do not match; never weaken an assertion. The e2e stack already sets `OPEN_SIGNUP=1` and `E2E_TEST_HOOKS=1`.

- [ ] **Step 3: Docs, run everything, commit**

`AGENTS.md` banner: append "Phase 4 (frontend on TanStack Router/Query, generated client, limen-auth; passkeys removed) is implemented; `bun run gen:client` regenerates `apps/frontend/src/lib/api-schema.d.ts` after spec changes." `README.md`: the development section describes `bun run dev` + `bun run dev:server` with the env from Global Constraints and `bun run gen:client`.

```bash
bun run check && bun run test && bun run build
E2E_REBUILD=1 mise run e2e 2>&1 | tail -15     # expect 18 passed (7 smoke + 4 auth-api + 4 links-api + 3 app)
git add -A apps/frontend e2e AGENTS.md README.md package.json bun.lock
git commit -m "feat(frontend): drop react-router and Better Auth; browser e2e flows for sign-up, links and invitations"
```

Do not push; the controller runs the whole-branch review and opens the PR (base `feat/go-migration-phase-3` until #81 merges).

---

## Self-review

**Spec coverage (section 6 and phase 4 in section 11):** TanStack Router with the exact route table and guards (T3, T4, T5); `{org}` slug resolution with server-side switch (T3 `orgRoute`); query keys and invalidation (T2); `GET /api/config` hiding sign-up (T3); limen-auth for sign-in/up/out, two-factor, password change/reset (T1, T3, T6); openapi-fetch for everything else (T1, T2); passkeys removed (T6, T7); invitation registration flow (T5); dev loop unchanged (Global Constraints, T7 docs); generated client committed with a CI drift check (T1); react-router-dom removed (T7). Section 9's frontend tests: bun tests (T1–T3) and Playwright browser flows (T7).

**Deviations decided here:** (1) session truth is the `['me']` query rather than limen-auth's session store (its payload lacks the user id and active organization); (2) `['config']`, `['invitation', id]` and `['teamMembers', teamId]` keys added; (3) the landing chips and one sentence lose their Cloudflare/Better Auth wording; (4) a small "Delete account" card is added to settings because the API exists and no screen used it; (5) email change now asks for the current password (the API requires it); (6) `/app/select-organization` (old post-auth target) becomes `/app`.

**Placeholder scan:** none; every code step carries its code, every port step names the hooks, the route, the test ids and the behaviour.

**Type consistency:** `unwrap`/`ApiError`/`errorMessage` (T1) used in T2–T6; `keys` and the hook names in T2's interface block match every use in T3–T6; `orgRoute.useRouteContext()` returns `{ organization }` as defined in T3 and consumed in T4/T5; `afterAuthPath` (T3) used by `indexRoute` and the invitation page; `signInWithPassword` returns `{ twoFactorRequired }` as the landing page expects; `useUploadProfileImage` posts field `file` and both image hooks read `{ imageUrl }` per `images.go`.
