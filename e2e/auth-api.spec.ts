import { expect, test, type APIRequestContext } from "@playwright/test";

const unique = () => Math.random().toString(36).slice(2, 10);
const PASSWORD = "Playwright123";
const ORIGIN = process.env.E2E_BASE_URL ?? "http://127.0.0.1:3300";
const headers = { origin: ORIGIN, "content-type": "application/json" };

async function signUp(request: APIRequestContext, name: string, email: string) {
  const res = await request.post("/api/auth/signup/credential", { headers, data: { name, email, password: PASSWORD } });
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

  const bad = await request.post("/api/auth/signin/credential", {
    headers,
    data: { credential: email, password: "wrong" },
  });
  expect(bad.status()).toBe(401);

  const patched = await request.patch("/api/me", { headers, data: { name: "Kari Nordmann" } });
  expect((await patched.json()).user.name).toBe("Kari Nordmann");
});

test("organization, team, invitation with team, registration through the invitation", async ({
  request,
  playwright,
}) => {
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
  const inv = await request.post(`/api/organizations/${orgId}/invitations`, {
    headers,
    data: { email: invitee, role: "member", teamId },
  });
  expect(inv.status(), await inv.text()).toBe(201);
  const invitationId = (await inv.json()).id;

  const mail = await (await request.get("/api/_test/mail")).json();
  expect(mail.messages[0].to).toBe(invitee);
  expect(mail.messages[0].text).toContain(`/app/invitations/${invitationId}`);

  // A second, anonymous context plays the invitee.
  const guest = await playwright.request.newContext({ baseURL: ORIGIN });
  const pub = await guest.get(`/api/invitations/${invitationId}`);
  expect(await pub.json()).toMatchObject({
    organizationName: "Acme",
    teamName: "Marketing",
    role: "member",
    hasAccount: false,
  });
  expect(await pub.text()).not.toContain(invitee);

  const reg = await guest.post(`/api/invitations/${invitationId}/register`, {
    headers,
    data: { name: "New Person", password: PASSWORD },
  });
  expect(reg.status(), await reg.text()).toBe(201);
  expect(reg.headers()["set-cookie"]).toContain("snarvei_session=");
  const regBody = await reg.json();
  expect(regBody.session.activeOrganizationId).toBe(orgId);

  const teams = await guest.get(`/api/organizations/${orgId}/teams`);
  expect((await teams.json()).map((t: { id: string }) => t.id)).toEqual([teamId]);
  const members = await guest.get(`/api/teams/${teamId}/members`);
  expect((await members.json()).map((m: { email: string }) => m.email)).toEqual([invitee]);

  // A member cannot invite or create teams.
  expect((await guest.post(`/api/organizations/${orgId}/teams`, { headers, data: { name: "Nope" } })).status()).toBe(
    403,
  );
  expect(
    (
      await guest.post(`/api/organizations/${orgId}/invitations`, {
        headers,
        data: { email: "x@example.com", role: "member" },
      })
    ).status(),
  ).toBe(403);
  await guest.dispose();
});

test("existing account accepts an invitation; strangers are refused", async ({ request, playwright }) => {
  const owner = `owner-${unique()}@example.com`;
  await signUp(request, "Owner", owner);
  const orgId = (
    await (
      await request.post("/api/organizations", { headers, data: { name: "Beta", slug: `beta-${unique()}` } })
    ).json()
  ).id;
  await request.post(`/api/organizations/${orgId}/switch`, { headers });

  const existing = `existing-${unique()}@example.com`;
  const invitee = await playwright.request.newContext({ baseURL: ORIGIN });
  await signUp(invitee, "Existing", existing);
  const stranger = await playwright.request.newContext({ baseURL: ORIGIN });
  await signUp(stranger, "Stranger", `stranger-${unique()}@example.com`);

  const inv = await request.post(`/api/organizations/${orgId}/invitations`, {
    headers,
    data: { email: existing, role: "admin" },
  });
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
  // Limen's credential-password plugin throttles /signup/credential to 5
  // requests per 10s per client IP (all requests in this file share the
  // e2e stack's loopback IP). The three tests above already spent that
  // budget (5 sign-ups); wait out the window so this test's own sign-up
  // does not get a 429 instead of the 200 it actually returns once clear.
  await new Promise((resolve) => setTimeout(resolve, 10_500));
  const email = `reset-${unique()}@example.com`;
  await signUp(request, "Reset Me", email);
  await request.delete("/api/_test/mail");
  const anon = await playwright.request.newContext({ baseURL: ORIGIN });
  expect((await anon.post("/api/auth/passwords/request-reset", { headers, data: { email } })).status()).toBe(200);
  const mail = await (await anon.get("/api/_test/mail")).json();
  const token = /token=([A-Za-z0-9._-]+)/.exec(mail.messages[0].text)?.[1];
  expect(token).toBeTruthy();
  expect(
    (await anon.post("/api/auth/passwords/reset", { headers, data: { token, new_password: "Changed456" } })).status(),
  ).toBe(200);
  expect((await request.get("/api/me")).status()).toBe(401); // old session revoked
  const signin = await anon.post("/api/auth/signin/credential", {
    headers,
    data: { credential: email, password: "Changed456" },
  });
  expect(signin.status()).toBe(200);
  await anon.dispose();
});
