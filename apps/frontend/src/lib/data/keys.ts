export type LinkFilters = { teamId?: string; page: number; pageSize: number };

export const keys = {
  me: ["me"] as const,
  config: ["config"] as const,
  organizations: ["organizations"] as const,
  sessions: ["sessions"] as const,
  teams: (orgId: string) => ["teams", orgId] as const,
  teamMembers: (teamId: string) => ["teamMembers", teamId] as const,
  members: (orgId: string) => ["members", orgId] as const,
  invitations: (orgId: string) => ["invitations", orgId] as const,
  invitation: (id: string) => ["invitation", id] as const,
  links: (orgId: string, filters: LinkFilters) => ["links", orgId, filters] as const,
  link: (id: string) => ["link", id] as const,
  history: (id: string, page: number) => ["history", id, page] as const,
  analytics: (id: string, days: number) => ["analytics", id, days] as const,
};
