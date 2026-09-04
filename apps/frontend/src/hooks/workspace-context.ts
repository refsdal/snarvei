import { createContext } from "react";
import type {
  AnalyticsSummary,
  AppMessage,
  HistoryItem,
  Invitation,
  InvitationRole,
  Link,
  Member,
  OrganizationSummary,
  SelectedLinkFormValues,
  SessionData,
  Team,
} from "../types";

export type WorkspaceContextValue = {
  session: SessionData | null;
  sessionPending: boolean;
  organizationsPending: boolean;
  organizations: OrganizationSummary[];
  activeOrganizationId: string | null;
  activeOrganization: OrganizationSummary | null;
  teams: Team[];
  activeTeamId: string | null;
  members: Member[];
  invitations: Invitation[];
  links: Link[];
  selectedLink: Link | null;
  history: HistoryItem[];
  analytics: AnalyticsSummary;
  loadingOrganizations: boolean;
  loadingTeams: boolean;
  loadingMembers: boolean;
  loadingInvitations: boolean;
  loadingLinks: boolean;
  loadingDetails: boolean;
  submitting: string | null;
  message: AppMessage | null;
  appOrigin: string;
  setMessage: (message: AppMessage | null) => void;
  refreshSessionState: () => Promise<void>;
  signIn: (input: { email: string; password: string }) => Promise<{
    ok: boolean;
    requiresTwoFactor?: boolean;
    twoFactorMethods?: string[];
  }>;
  signUp: (input: { name: string; email: string; password: string }) => Promise<boolean>;
  signOut: () => Promise<void>;
  switchOrganization: (organizationId: string) => Promise<void>;
  createOrganization: (input: { name: string; slug: string }) => Promise<string | null>;
  createTeam: (input: { name: string }) => Promise<string | null>;
  setSelectedLinkId: (linkId: string | null) => void;
  refreshOrganizations: (options?: { silent?: boolean }) => Promise<void>;
  refreshOrganizationData: (organizationId: string, options?: { silent?: boolean }) => Promise<void>;
  refreshSelectedLinkData: (linkId: string, options?: { silent?: boolean }) => Promise<void>;
  createLink: (input: {
    teamId: string;
    targetUrl: string;
    redirectStatus: 301 | 302 | 307;
    title?: string;
    description?: string;
  }) => Promise<Link | null>;
  updateLink: (linkId: string, values: SelectedLinkFormValues) => Promise<Link | null>;
  deleteLink: (linkId: string) => Promise<boolean>;
  inviteMember: (input: { email: string; role: InvitationRole; teamId?: string | null }) => Promise<boolean>;
  cancelInvitation: (invitationId: string) => Promise<boolean>;
  getLinkById: (linkId: string) => Link | null;
};

export const WorkspaceContext = createContext<WorkspaceContextValue | null>(null);
