/**
 * API DTO types for the React app. These are a hand copy of the zod/OpenAPI
 * schemas that used to live in the Cloudflare Worker's `openapi/schemas.ts`,
 * deleted when Snarvei moved off Workers to a Go server. They are kept here,
 * with no runtime imports, until the generated OpenAPI client replaces this
 * file in a later phase.
 */

export type LinkDto = {
  id: string;
  organizationId: string;
  teamId: string;
  teamName?: string | null;
  slug: string;
  targetUrl: string;
  redirectStatus: 301 | 302 | 307;
  isActive: boolean;
  title: string | null;
  description: string | null;
  createdBy: string | null;
  updatedBy: string | null;
  createdAt: string;
  updatedAt: string;
};

export type HistoryItemDto = {
  id: string;
  oldTargetUrl: string | null;
  newTargetUrl: string;
  changedBy: string | null;
  changedAt: string;
};

export type AnalyticsSummaryDto = {
  totalClicks: number;
  uniqueVisitorApproximation: number;
  topCountries: { country: string | null; clicks: number }[];
  topReferrers: { referer: string | null; clicks: number }[];
  clicksByDay: { day: string; clicks: number }[];
  /** The time window the numbers were computed for (ISO timestamps). */
  range: { from: string; to: string };
};

export type TeamMemberDto = {
  id: string;
  userId: string;
  name: string | null;
  email: string | null;
  createdAt: string | null;
};
