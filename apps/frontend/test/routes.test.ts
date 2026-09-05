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
