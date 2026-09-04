import type { OrganizationSummary } from "../types";

type OrganizationTarget = Pick<OrganizationSummary, "id" | "slug"> | null | undefined;

export const getOrganizationPathSegment = (organization: OrganizationTarget) =>
  organization?.slug ?? organization?.id ?? "";

export const buildOrganizationPath = (organization: OrganizationTarget, suffix = "dashboard") => {
  const org = getOrganizationPathSegment(organization);
  return `/app/${org}/${suffix}`;
};

export const buildLinksPath = (organization: OrganizationTarget, linkId?: string) => {
  const base = buildOrganizationPath(organization, "links");
  return linkId ? `${base}/${linkId}` : base;
};

export const settingsPath = "/app/settings";
