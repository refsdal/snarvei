import { expect, test, type APIRequestContext } from "@playwright/test";

const unique = () => Math.random().toString(36).slice(2, 10);
const PASSWORD = "Playwright123";
const ORIGIN = process.env.E2E_BASE_URL ?? "http://127.0.0.1:3300";
const headers = { origin: ORIGIN, "content-type": "application/json" };

// Limen's credential-password plugin throttles /signup/credential to 5
// requests per 10s per client IP; every request in this file shares the
// e2e stack's loopback IP, so retry through the throttle instead of assuming
// a fixed number of prior sign-ups.
async function signUp(request: APIRequestContext, name: string, email: string) {
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

async function workspace(request: APIRequestContext) {
  await signUp(request, "Owner", `owner-${unique()}@example.com`);
  const org = await (
    await request.post("/api/organizations", { headers, data: { name: "Acme", slug: `acme-${unique()}` } })
  ).json();
  await request.post(`/api/organizations/${org.id}/switch`, { headers });
  const team = await (
    await request.post(`/api/organizations/${org.id}/teams`, { headers, data: { name: "Marketing" } })
  ).json();
  return { orgId: org.id as string, teamId: team.id as string };
}

test("create a link, follow it, retarget it, see history and analytics, deactivate, delete", async ({
  request,
  playwright,
}) => {
  const { orgId, teamId } = await workspace(request);
  const created = await request.post("/api/links", {
    headers,
    data: { teamId, targetUrl: "https://example.com/v1", title: "Launch" },
  });
  expect(created.status(), await created.text()).toBe(201);
  const link = await created.json();
  expect(link.slug).toMatch(/^[A-Za-z2-9]{8}$/);

  const visitor = await playwright.request.newContext({ baseURL: ORIGIN, maxRedirects: 0 });
  const hop = await visitor.get(`/l/${link.slug}?utm_source=news&secret=1`, {
    headers: { referer: "https://ref.example/page?x=1" },
  });
  expect(hop.status()).toBe(302);
  expect(hop.headers()["location"]).toBe("https://example.com/v1");
  expect(hop.headers()["cache-control"]).toBe("no-store");

  await expect
    .poll(async () => (await (await request.get(`/api/links/${link.id}/analytics`)).json()).totalClicks, {
      timeout: 5000,
    })
    .toBe(1);
  const analytics = await (await request.get(`/api/links/${link.id}/analytics`)).json();
  expect(analytics.topReferrers[0]).toEqual({ referer: "https://ref.example/page", clicks: 1 });

  const retarget = await request.patch(`/api/links/${link.id}`, {
    headers,
    data: { targetUrl: "https://example.com/v2", redirectStatus: 307 },
  });
  expect(retarget.status()).toBe(200);
  const hop2 = await visitor.get(`/l/${link.slug}`);
  expect(hop2.status()).toBe(307);
  expect(hop2.headers()["location"]).toBe("https://example.com/v2");
  const history = await (await request.get(`/api/links/${link.id}/history`)).json();
  expect(history.total).toBe(2);
  expect(history.items[0]).toMatchObject({
    oldTargetUrl: "https://example.com/v1",
    newTargetUrl: "https://example.com/v2",
  });

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
  const created = await request.post("/api/links", {
    headers,
    data: { teamId, targetUrl: "https://example.com", slug: `  ${slug.toUpperCase()} ` },
  });
  expect(created.status(), await created.text()).toBe(201);
  expect((await created.json()).slug).toBe(slug);
  const dup = await request.post("/api/links", { headers, data: { teamId, targetUrl: "https://example.com", slug } });
  expect(dup.status()).toBe(409);
  expect((await dup.json()).code).toBe("SLUG_TAKEN");

  const other = await playwright.request.newContext({ baseURL: ORIGIN });
  const theirs = await workspace(other);
  const cross = await other.post("/api/links", {
    headers,
    data: { teamId: theirs.teamId, targetUrl: "https://example.com", slug },
  });
  expect(cross.status()).toBe(409);
  await other.dispose();

  for (const bad of [
    { targetUrl: "javascript:alert(1)" },
    { targetUrl: "https://example.com", slug: "Hello World!" },
    { targetUrl: "https://example.com", redirectStatus: 308 },
  ]) {
    const res = await request.post("/api/links", { headers, data: { teamId, ...bad } });
    expect(res.status(), JSON.stringify(bad)).toBe(400);
  }
});

test("a member outside the team cannot see or edit its links", async ({ request, playwright }) => {
  const { orgId, teamId } = await workspace(request);
  const link = await (
    await request.post("/api/links", { headers, data: { teamId, targetUrl: "https://example.com" } })
  ).json();
  await request.delete("/api/_test/mail");
  const email = `member-${unique()}@example.com`;
  const inv = await (
    await request.post(`/api/organizations/${orgId}/invitations`, { headers, data: { email, role: "member" } })
  ).json();
  const member = await playwright.request.newContext({ baseURL: ORIGIN });
  expect(
    (
      await member.post(`/api/invitations/${inv.id}/register`, {
        headers,
        data: { name: "Member", password: PASSWORD },
      })
    ).status(),
  ).toBe(201);
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
