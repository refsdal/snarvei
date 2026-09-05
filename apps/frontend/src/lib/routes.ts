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

/** `{ org }` route params for a typed `navigate({ to: "/app/$org/...", params })`. */
export const orgParams = (organization: OrganizationTarget) => ({ org: organization?.slug ?? organization?.id ?? "" });

/** Where to go after sign-in/sign-up: a `next` that is an in-app path, else the picker. */
export const afterAuthPath = (next: string | null | undefined): string =>
  next === "/app" || (typeof next === "string" && next.startsWith("/app/")) ? next : "/app";
