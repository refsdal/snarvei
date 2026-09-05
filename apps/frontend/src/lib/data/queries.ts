import { queryOptions, useQuery } from "@tanstack/react-query";
import { ApiError, client, unwrap } from "../api";
import { keys, type LinkFilters } from "./keys";
import type {
  Analytics,
  HistoryPage,
  Invitation,
  Link,
  LinkPage,
  Me,
  Member,
  Organization,
  PublicConfig,
  PublicInvitation,
  SessionSummary,
  Team,
  TeamMember,
} from "./types";

// "Not signed in" is a value, not an error: guards and the landing page
// branch on null without tripping React Query's error state.
export const meQueryOptions = () =>
  queryOptions({
    queryKey: keys.me,
    queryFn: async (): Promise<Me | null> => {
      try {
        return await unwrap<Me>(client.GET("/api/me"));
      } catch (err) {
        if (err instanceof ApiError && err.status === 401) return null;
        throw err;
      }
    },
    staleTime: 30_000,
  });

export const configQueryOptions = () =>
  queryOptions({
    queryKey: keys.config,
    queryFn: () => unwrap<PublicConfig>(client.GET("/api/config")),
    staleTime: Number.POSITIVE_INFINITY,
  });

export const organizationsQueryOptions = () =>
  queryOptions({
    queryKey: keys.organizations,
    queryFn: () => unwrap<Organization[]>(client.GET("/api/organizations")),
  });

export const sessionsQueryOptions = () =>
  queryOptions({ queryKey: keys.sessions, queryFn: () => unwrap<SessionSummary[]>(client.GET("/api/me/sessions")) });

export const teamsQueryOptions = (orgId: string) =>
  queryOptions({
    queryKey: keys.teams(orgId),
    queryFn: () => unwrap<Team[]>(client.GET("/api/organizations/{orgId}/teams", { params: { path: { orgId } } })),
  });

export const teamMembersQueryOptions = (teamId: string) =>
  queryOptions({
    queryKey: keys.teamMembers(teamId),
    queryFn: () => unwrap<TeamMember[]>(client.GET("/api/teams/{teamId}/members", { params: { path: { teamId } } })),
  });

export const membersQueryOptions = (orgId: string) =>
  queryOptions({
    queryKey: keys.members(orgId),
    queryFn: () => unwrap<Member[]>(client.GET("/api/organizations/{orgId}/members", { params: { path: { orgId } } })),
  });

export const invitationsQueryOptions = (orgId: string) =>
  queryOptions({
    queryKey: keys.invitations(orgId),
    queryFn: () =>
      unwrap<Invitation[]>(client.GET("/api/organizations/{orgId}/invitations", { params: { path: { orgId } } })),
  });

export const invitationQueryOptions = (id: string) =>
  queryOptions({
    queryKey: keys.invitation(id),
    queryFn: () =>
      unwrap<PublicInvitation>(
        client.GET("/api/invitations/{invitationId}", { params: { path: { invitationId: id } } }),
      ),
    retry: false,
  });

export const linksQueryOptions = (orgId: string, filters: LinkFilters) =>
  queryOptions({
    queryKey: keys.links(orgId, filters),
    queryFn: () =>
      unwrap<LinkPage>(
        client.GET("/api/links", {
          params: {
            query: { organizationId: orgId, teamId: filters.teamId, page: filters.page, pageSize: filters.pageSize },
          },
        }),
      ),
  });

export const linkQueryOptions = (id: string) =>
  queryOptions({
    queryKey: keys.link(id),
    queryFn: () => unwrap<Link>(client.GET("/api/links/{linkId}", { params: { path: { linkId: id } } })),
  });

export const historyQueryOptions = (id: string, page: number) =>
  queryOptions({
    queryKey: keys.history(id, page),
    queryFn: () =>
      unwrap<HistoryPage>(
        client.GET("/api/links/{linkId}/history", { params: { path: { linkId: id }, query: { page, pageSize: 100 } } }),
      ),
  });

export const analyticsQueryOptions = (id: string, days: number) =>
  queryOptions({
    queryKey: keys.analytics(id, days),
    queryFn: () =>
      unwrap<Analytics>(
        client.GET("/api/links/{linkId}/analytics", { params: { path: { linkId: id }, query: { days } } }),
      ),
  });

export const useMe = () => useQuery(meQueryOptions());
export const useConfig = () => useQuery(configQueryOptions());
export const useOrganizations = () => useQuery(organizationsQueryOptions());
export const useSessions = () => useQuery(sessionsQueryOptions());
export const useTeams = (orgId: string) => useQuery(teamsQueryOptions(orgId));
export const useTeamMembers = (teamId: string) => useQuery(teamMembersQueryOptions(teamId));
export const useMembers = (orgId: string) => useQuery(membersQueryOptions(orgId));
export const useInvitations = (orgId: string) => useQuery(invitationsQueryOptions(orgId));
export const usePublicInvitation = (id: string) => useQuery(invitationQueryOptions(id));
export const useLinks = (orgId: string, filters: LinkFilters) => useQuery(linksQueryOptions(orgId, filters));
export const useLink = (id: string) => useQuery(linkQueryOptions(id));
export const useLinkHistory = (id: string, page = 1) => useQuery(historyQueryOptions(id, page));
export const useLinkAnalytics = (id: string, days = 30) => useQuery(analyticsQueryOptions(id, days));
