import { expect, type APIRequestContext, type Page } from "@playwright/test";

export const PASSWORD = "Playwright123";
export const ORIGIN = process.env.E2E_BASE_URL ?? "http://127.0.0.1:3300";
export const headers = { origin: ORIGIN, "content-type": "application/json" };

export const unique = () => Math.random().toString(36).slice(2, 10);

// Limen's credential-password plugin throttles /signup/credential to 5
// requests per 10s per client IP; every request against the e2e stack shares
// its loopback IP, so retry through the throttle instead of assuming a fixed
// number of prior sign-ups.
export async function signUpApi(request: APIRequestContext, name: string, email: string): Promise<void> {
  let res = await request.post("/api/auth/signup/credential", { headers, data: { name, email, password: PASSWORD } });
  for (let attempt = 0; res.status() === 429 && attempt < 8; attempt++) {
    const retryAfter = Number(res.headers()["retry-after"]);
    await new Promise((resolve) =>
      setTimeout(resolve, (Number.isFinite(retryAfter) && retryAfter > 0 ? retryAfter : 2.5) * 1000),
    );
    res = await request.post("/api/auth/signup/credential", { headers, data: { name, email, password: PASSWORD } });
  }
  expect(res.status(), await res.text()).toBe(200);
}

// The "Create account" button stays disabled until name, email and password
// are all filled, so fill first, then retry the click through the throttle.
// 5 spec files now share this same per-IP throttle (up from 4 before the
// flows.spec.ts flows were added), so a burst at the start of the run can
// need more than a handful of retries to clear; 14 attempts at 2.5s apart
// (worst case ~35s) fits comfortably under playwright.config.ts's 60s
// per-test timeout.
export async function signUpUi(page: Page, name: string, email: string): Promise<void> {
  await page.goto("/");
  await page.getByTestId("auth-name-input").fill(name);
  await page.getByTestId("auth-email-input").fill(email);
  await page.getByTestId("auth-password-input").fill(PASSWORD);
  for (let attempt = 0; attempt < 14; attempt++) {
    await page.getByTestId("create-account-button").click();
    const outcome = await Promise.race([
      page.waitForURL(/\/app(\?|$|\/)/, { timeout: 5000 }).then(() => "ok" as const),
      page
        .getByText(/too many|rate/i)
        .first()
        .waitFor({ timeout: 5000 })
        .then(() => "throttled" as const),
    ]).catch(() => "unknown" as const);
    if (outcome === "ok") return;
    await page.waitForTimeout(2500);
  }
  throw new Error("sign-up kept being throttled");
}

export async function createOrganizationUi(page: Page, name: string, slug: string): Promise<void> {
  await page.getByRole("button", { name: "Create organization" }).click();
  await page.getByTestId("organization-name-input").fill(name);
  await page.getByTestId("organization-slug-input").fill(slug);
  await page.getByTestId("create-organization-button").click();
  await page.waitForURL(`**/app/${slug}/dashboard`);
}

export async function workspace(
  request: APIRequestContext,
): Promise<{ orgId: string; slug: string; teamId: string; ownerEmail: string }> {
  const ownerEmail = `owner-${unique()}@example.com`;
  await signUpApi(request, "Owner", ownerEmail);
  const slug = `acme-${unique()}`;
  const org = await (await request.post("/api/organizations", { headers, data: { name: "Acme", slug } })).json();
  await request.post(`/api/organizations/${org.id}/switch`, { headers });
  const team = await (
    await request.post(`/api/organizations/${org.id}/teams`, { headers, data: { name: "Marketing" } })
  ).json();
  return { orgId: org.id as string, slug, teamId: team.id as string, ownerEmail };
}

// The mailbox is a single process-wide, in-memory recording shared by every
// spec file running concurrently, and no spec clears it. Poll for the newest
// message addressed to `email` (never index 0) rather than assuming anything
// about what else landed in between.
export async function lastMailTo(
  request: APIRequestContext,
  email: string,
): Promise<{ to: string; subject: string; text: string }> {
  const deadline = Date.now() + 10_000;
  for (;;) {
    const res = await request.get("/api/_test/mail");
    const { messages } = (await res.json()) as { messages: { to: string; subject: string; text: string }[] };
    const mine = messages.find((m) => m.to === email); // newest first
    if (mine) return mine;
    if (Date.now() > deadline) throw new Error(`no mail for ${email}`);
    await new Promise((r) => setTimeout(r, 250));
  }
}
