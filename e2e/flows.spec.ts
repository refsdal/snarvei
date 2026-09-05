import { expect, test } from "@playwright/test";
import { headers, lastMailTo, PASSWORD, signUpApi, signUpUi, unique, workspace } from "./support";

test("a member sees only their team's links, and an admin can add them to another team", async ({
  page,
  request,
  browser,
}) => {
  // Owner (API): org with teams Marketing (from workspace) and Sales; one link in each.
  const ws = await workspace(request);
  const sales = await (
    await request.post(`/api/organizations/${ws.orgId}/teams`, { headers, data: { name: "Sales" } })
  ).json();
  await request.post("/api/links", {
    headers,
    data: { teamId: ws.teamId, targetUrl: "https://example.com/marketing", title: "Marketing launch" },
  });
  await request.post("/api/links", {
    headers,
    data: { teamId: sales.id, targetUrl: "https://example.com/sales", title: "Sales deck" },
  });
  // Invite a member into Marketing only.
  const member = `member-${unique()}@example.com`;
  const inv = await (
    await request.post(`/api/organizations/${ws.orgId}/invitations`, {
      headers,
      data: { email: member, role: "member", teamId: ws.teamId },
    })
  ).json();
  // The member has an account already: sign up in the browser, then accept through the emailed link.
  await signUpUi(page, "Member", member);
  const mail = await lastMailTo(request, member);
  const link = mail.text.match(/\/app\/invitations\/[A-Za-z0-9-]+/)?.[0];
  expect(link).toBe(`/app/invitations/${inv.id}`);
  await page.goto(link!);
  await expect(page.getByTestId("invitation-organization")).toHaveText("Acme");
  await page.getByTestId("invitation-accept-button").click();
  await page.waitForURL(/\/app(\?|$|\/)/);
  await page.getByRole("button", { name: "Open workspace" }).click();
  await page.waitForURL(`**/app/${ws.slug}/dashboard`);
  await expect(page.getByTestId("dashboard-links-count")).toHaveText("1");
  await page.goto(`/app/${ws.slug}/links`);
  await expect(page.getByText("Marketing launch")).toBeVisible();
  await expect(page.getByText("Sales deck")).toHaveCount(0);
  await expect(page.getByTestId("links-team-filter")).toHaveCount(0); // one team: no filter

  // Owner (browser, second context) adds the member to Sales through the team dialog.
  const ownerCtx = await browser.newContext();
  const owner = await ownerCtx.newPage();
  await owner.goto("/");
  await owner.getByTestId("auth-email-input").fill(ws.ownerEmail);
  await owner.getByTestId("auth-password-input").fill(PASSWORD);
  await owner.getByTestId("sign-in-button").click();
  await owner.waitForURL(/\/app/);
  await owner.goto(`/app/${ws.slug}/organization`);
  await owner.getByTestId("manage-team-Sales").click();
  // MUI's Select puts the data-testid on the hidden native <input> used for
  // form submission; the clickable, visible element that opens the listbox
  // is the sibling div with role="combobox", found here by its label.
  await owner.getByRole("combobox", { name: "Add organization member" }).click();
  await owner.getByTestId(`add-team-member-option-${member}`).click();
  await owner.getByTestId("add-team-member-button").click();
  await expect(owner.getByTestId("team-members-list")).toContainText(member);
  await ownerCtx.close();

  // The member now sees both links.
  await page.reload();
  await expect(page.getByText("Sales deck")).toBeVisible();
  await expect(page.getByTestId("links-team-filter")).toBeVisible();
});

test("profile name update is reflected in the shell", async ({ page }) => {
  const email = `kari-${unique()}@example.com`;
  await signUpUi(page, "Kari", email);
  await page.goto("/app/settings");
  await page.getByTestId("settings-name-input").fill("Kari Nordmann");
  await page.getByRole("button", { name: /save/i }).first().click();
  await expect(page.getByRole("alert")).toContainText(/updated|saved/i);
  await page.reload();
  await expect(page.getByText("Kari Nordmann")).toBeVisible();
});

test("forgot-password flow through the mailbox", async ({ page, request }) => {
  const email = `reset-${unique()}@example.com`;
  await signUpApi(request, "Ola", email);
  await page.goto("/");
  await page.getByRole("button", { name: /forgot/i }).click();
  await page.getByTestId("forgot-password-email-input").fill(email);
  await page.getByTestId("forgot-password-button").click();
  const mail = await lastMailTo(request, email);
  const url = mail.text.match(/\/reset-password\?token=[^\s]+/)?.[0];
  expect(url).toBeTruthy();
  await page.goto(url!);
  await page.getByTestId("reset-password-input").fill("Newpass456!");
  await page.getByTestId("reset-password-confirm-input").fill("Newpass456!");
  await page.getByTestId("reset-password-button").click();
  await page.waitForURL(/\/\?reset=done/);
  await page.getByTestId("auth-email-input").fill(email);
  await page.getByTestId("auth-password-input").fill("Newpass456!");
  await page.getByTestId("sign-in-button").click();
  await page.waitForURL(/\/app/);
});

test("the Scalar page renders the API reference", async ({ page }) => {
  const res = await page.goto("/scalar");
  expect(res?.status()).toBe(200);
  await expect(page.locator("script#api-reference")).toBeAttached();
  // The bundle comes from cdn.jsdelivr.net (allowed by the page's own CSP);
  // give it time, then expect the spec's title to be rendered.
  await expect(page.getByText("Snarvei API").first()).toBeVisible({ timeout: 20_000 });
});
