import { expect, test } from "@playwright/test";

const CSP =
  "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'";

test("landing page renders the product messaging and a sign-in button", async ({ page }) => {
  const response = await page.goto("/");
  expect(response?.status()).toBe(200);
  await expect(page.getByText("Short links you can trust long after they are shared.")).toBeVisible();
  await expect(page.getByTestId("sign-in-button")).toBeVisible();
});

test("SPA deep links are served by the fallback, not a 404", async ({ page }) => {
  for (const path of ["/app", "/app/settings", "/reset-password", "/app/acme/links/abc"]) {
    const response = await page.goto(path);
    expect(response?.status(), path).toBe(200);
    expect(response?.headers()["content-type"], path).toContain("text/html");
    await expect(page.locator("#root")).toBeAttached();
  }
});

test("SPA responses carry the security headers; probes do not", async ({ request }) => {
  const spa = await request.get("/");
  expect(spa.headers()["content-security-policy"]).toBe(CSP);
  expect(spa.headers()["x-frame-options"]).toBe("DENY");
  expect(spa.headers()["x-content-type-options"]).toBe("nosniff");
  expect(spa.headers()["cache-control"]).toBe("no-cache");

  const probe = await request.get("/healthz");
  expect(probe.headers()["content-security-policy"]).toBeUndefined();
  expect(probe.headers()["cache-control"]).toBe("no-store");
});

test("hashed assets are immutable", async ({ request }) => {
  const html = await (await request.get("/")).text();
  const asset = html.match(/\/assets\/[^"']+\.js/)?.[0];
  expect(asset, "index.html references a hashed bundle").toBeTruthy();
  const response = await request.get(asset as string);
  expect(response.status()).toBe(200);
  expect(response.headers()["cache-control"]).toBe("public, max-age=31536000, immutable");
});

test("probes and public config answer JSON", async ({ request }) => {
  const healthz = await request.get("/healthz");
  expect(healthz.status()).toBe(200);
  expect(await healthz.json()).toMatchObject({ ok: true, service: "snarvei" });
  expect((await healthz.json()).version).toEqual(expect.any(String));

  const readyz = await request.get("/readyz");
  expect(readyz.status()).toBe(200);
  expect(await readyz.json()).toEqual({ ok: true });

  const config = await request.get("/api/config");
  expect(config.status()).toBe(200);
  expect(await config.json()).toEqual({ appName: "Snarvei", openSignup: true });
});

test("unknown server-owned paths answer a JSON 404", async ({ request }) => {
  // /l/{slug} and /openapi.json are now real, documented routes (phase 3)
  // with their own status/content-type on an unknown slug or a real spec
  // fetch, so they no longer belong in this "unknown path" list; they are
  // covered by links-api.spec.ts and the CI smoke curls instead.
  for (const path of ["/api/nope", "/images/profile/x.png"]) {
    const response = await request.get(path);
    expect(response.status(), path).toBe(404);
    expect(response.headers()["content-type"], path).toContain("application/json");
    expect(await response.json(), path).toMatchObject({ code: "NOT_FOUND" });
  }
});

test("robots.txt disallows crawling", async ({ request }) => {
  const response = await request.get("/robots.txt");
  expect(response.status()).toBe(200);
  expect(await response.text()).toBe("User-agent: *\nDisallow: /\n");
});
