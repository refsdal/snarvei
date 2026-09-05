import { expect, test } from "@playwright/test";
import { createOrganizationUi, lastMailTo, PASSWORD, signUpUi, unique } from "./support";

test("sign up, create an organization and a team, create a link, follow it, see the click", async ({
  page,
  context,
}) => {
  const slug = `acme-${unique()}`;
  await signUpUi(page, "Kari", `kari-${unique()}@example.com`);
  await createOrganizationUi(page, "Acme", slug);
  await expect(page.getByTestId("dashboard-links-count")).toHaveText("0");

  await page.goto(`/app/${slug}/organization`);
  await page.getByTestId("open-create-team-button").click();
  await page.getByTestId("team-name-input").fill("Marketing");
  await page.getByTestId("create-team-button").click();
  await expect(page.getByTestId("manage-team-Marketing")).toBeVisible();

  await page.goto(`/app/${slug}/links`);
  await page
    .getByRole("button", { name: /create link/i })
    .first()
    .click();
  await page.getByTestId("create-link-target-input").fill("https://example.com/launch");
  await page.getByTestId("create-link-title-input").fill("Launch");
  await page.getByTestId("create-link-button").click();
  await page.waitForURL(/\/links\/[^/]+$/);
  const shortUrl = await page
    .getByText(/\/l\/[A-Za-z2-9]{8}/)
    .first()
    .textContent();
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
  await expect(page.getByText("https://example.com/v2").first()).toBeVisible();
});

test("wrong password shows an error; sign out returns to the landing page", async ({ page }) => {
  const email = `ola-${unique()}@example.com`;
  await signUpUi(page, "Ola", email);
  await page.getByRole("button", { name: "Sign out" }).click();
  await page.waitForURL("/");
  await page.getByTestId("auth-email-input").fill(email);
  await page.getByTestId("auth-password-input").fill("wrong-password");
  await page.getByTestId("sign-in-button").click();
  await expect(page.getByRole("alert")).toBeVisible();
  await expect(page).toHaveURL("/");
});

test("an invitee registers through the emailed link and lands in the organization", async ({
  page,
  browser,
  request,
}) => {
  const slug = `acme-${unique()}`;
  await signUpUi(page, "Owner", `owner-${unique()}@example.com`);
  await createOrganizationUi(page, "Acme", slug);
  await page.goto(`/app/${slug}/organization`);

  const invitee = `new-${unique()}@example.com`;
  await page.getByTestId("open-invite-member-button").click();
  await page.getByTestId("invite-email-input").fill(invitee);
  await page.getByTestId("send-invitation-button").click();
  await expect(page.getByTestId(`invitation-${invitee}`)).toBeVisible();
  const mail = await lastMailTo(request, invitee);
  const link = mail.text.match(/\/app\/invitations\/[A-Za-z0-9-]+/)?.[0];
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

test("account deletion: a sole owner is blocked; a user with no organization deletes and lands on the landing page", async ({
  page,
}) => {
  // (a) A sole owner of an organization must transfer ownership first — the
  // server refuses with LAST_OWNER, and the account must not be deleted.
  const ownerEmail = `owner-${unique()}@example.com`;
  await signUpUi(page, "Solo Owner", ownerEmail);
  await createOrganizationUi(page, "Acme", `acme-${unique()}`);
  await page.goto("/app/settings");
  await page.getByTestId("settings-delete-password-input").fill(PASSWORD);
  await page.getByTestId("settings-delete-account-button").click();
  await page.getByTestId("settings-delete-account-confirm-button").click();
  await expect(page.getByRole("alert").filter({ hasText: /owner/i })).toBeVisible();
  // Still signed in: no redirect away from settings, and the page still works.
  await expect(page).toHaveURL(/\/app\/settings$/);
  await expect(page.getByTestId("settings-delete-account-button")).toBeVisible();

  // Sign out before the second scenario: `signUpUi` navigates to "/", and a
  // still-authenticated owner would bounce straight back into the app. The
  // settings page itself also has a "Sign out from this device" button, so
  // match the drawer's exactly.
  await page.getByRole("button", { name: "Sign out", exact: true }).click();
  await page.waitForURL("/");

  // (b) A fresh user with no organization can delete straight away and lands
  // cleanly on the landing page (this used to crash into the root error
  // boundary — see final-review.md Important 1).
  const freshEmail = `fresh-${unique()}@example.com`;
  await signUpUi(page, "Fresh User", freshEmail);
  await page.goto("/app/settings");
  await page.getByTestId("settings-delete-password-input").fill(PASSWORD);
  await page.getByTestId("settings-delete-account-button").click();
  await page.getByTestId("settings-delete-account-confirm-button").click();
  await page.waitForURL("/");
  await expect(page.getByTestId("sign-in-button")).toBeVisible();

  // The deleted credentials no longer work.
  await page.getByTestId("auth-email-input").fill(freshEmail);
  await page.getByTestId("auth-password-input").fill(PASSWORD);
  await page.getByTestId("sign-in-button").click();
  await expect(page.getByRole("alert")).toBeVisible();
  await expect(page).toHaveURL("/");
});
