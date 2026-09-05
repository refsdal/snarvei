import { describe, expect, test } from "bun:test";
import { afterAuthPath, buildLinksPath, buildOrganizationPath, orgParams, settingsPath } from "../src/lib/routes";

describe("route helpers", () => {
  test("prefers the organization slug over its id", () => {
    expect(buildOrganizationPath({ id: "org_1", slug: "acme" })).toBe("/app/acme/dashboard");
    expect(buildOrganizationPath({ id: "org_1", slug: undefined })).toBe("/app/org_1/dashboard");
  });

  test("orgParams prefers the slug over the id", () => {
    expect(orgParams({ id: "org_1", slug: "acme" })).toEqual({ org: "acme" });
    expect(orgParams({ id: "org_1", slug: undefined })).toEqual({ org: "org_1" });
  });

  test("builds link list and detail paths", () => {
    expect(buildLinksPath({ id: "org_1", slug: "acme" })).toBe("/app/acme/links");
    expect(buildLinksPath({ id: "org_1", slug: "acme" }, "lnk_9")).toBe("/app/acme/links/lnk_9");
  });

  test("settings path is fixed", () => {
    expect(settingsPath).toBe("/app/settings");
  });
});

describe("afterAuthPath", () => {
  test("honours in-app destinations only", () => {
    expect(afterAuthPath("/app/invitations/abc")).toBe("/app/invitations/abc");
    expect(afterAuthPath("/app")).toBe("/app");
    expect(afterAuthPath("https://evil.example/app/x")).toBe("/app");
    expect(afterAuthPath("//evil.example")).toBe("/app");
    expect(afterAuthPath(null)).toBe("/app");
    expect(afterAuthPath("/settings")).toBe("/app");
  });

  test("keeps the query string on an in-app destination", () => {
    expect(afterAuthPath("/app/settings?emailToken=abc")).toBe("/app/settings?emailToken=abc");
  });
});
