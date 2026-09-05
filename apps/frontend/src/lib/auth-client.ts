import { createAuthClient } from "better-auth/react";
import { passkeyClient } from "@better-auth/passkey/client";
import { organizationClient, twoFactorClient } from "better-auth/client/plugins";

type ClientError = {
  message?: string | null;
} | null;

type ClientResult<T> = Promise<{
  data?: T | null;
  error?: ClientError;
}>;

type ClientQueryResult<T> = {
  data: T | null | undefined;
  error?: ClientError;
  isPending: boolean;
  isRefetching?: boolean;
  refetch: () => Promise<void>;
};

type OrganizationSummary = {
  id: string;
  name: string;
  slug?: string;
};

type AuthSession = {
  id: string;
  token: string;
  createdAt?: string | number;
  updatedAt?: string | number;
  expiresAt?: string | number;
  ipAddress?: string | null;
  userAgent?: string | null;
};

type PasskeySummary = {
  id: string;
  name?: string | null;
  createdAt?: string | number | null;
  deviceType?: string | null;
  backedUp?: boolean;
};

type SnarveiAuthClient = {
  useSession: () => ClientQueryResult<unknown>;
  useListOrganizations: () => ClientQueryResult<OrganizationSummary[]>;
  useActiveOrganization: () => ClientQueryResult<OrganizationSummary>;
  updateUser: (input: { name?: string; image?: string | null }) => ClientResult<unknown>;
  changeEmail: (input: { newEmail: string; callbackURL?: string }) => ClientResult<unknown>;
  changePassword: (input: {
    currentPassword: string;
    newPassword: string;
    revokeOtherSessions?: boolean;
  }) => ClientResult<unknown>;
  requestPasswordReset: (input: { email: string; redirectTo?: string }) => ClientResult<unknown>;
  resetPassword: (input: { newPassword: string; token: string }) => ClientResult<unknown>;
  listSessions: () => ClientResult<AuthSession[]>;
  revokeSession: (input: { token: string }) => ClientResult<unknown>;
  revokeOtherSessions: () => ClientResult<unknown>;
  signIn: {
    email: (input: { email: string; password: string }) => ClientResult<unknown>;
    passkey: (input?: {
      autoFill?: boolean;
      extensions?: Record<string, unknown>;
      returnWebAuthnResponse?: boolean;
    }) => ClientResult<unknown>;
  };
  signUp: {
    email: (input: { name: string; email: string; password: string }) => ClientResult<unknown>;
  };
  signOut: () => Promise<unknown>;
  twoFactor: {
    enable: (input: {
      password?: string;
      issuer?: string;
    }) => ClientResult<{ totpURI?: string; backupCodes?: string[] }>;
    disable: (input: { password?: string }) => ClientResult<unknown>;
    getTotpUri: (input: { password?: string }) => ClientResult<{ totpURI?: string }>;
    verifyTotp: (input: { code: string; trustDevice?: boolean }) => ClientResult<unknown>;
    verifyBackupCode: (input: {
      code: string;
      trustDevice?: boolean;
      disableSession?: boolean;
    }) => ClientResult<unknown>;
    generateBackupCodes: (input: { password?: string }) => ClientResult<{ backupCodes?: string[] }>;
    viewBackupCodes: (input?: { userId?: string | null }) => ClientResult<{ backupCodes?: string[] }>;
  };
  passkey: {
    addPasskey: (input?: {
      name?: string;
      authenticatorAttachment?: "platform" | "cross-platform";
      extensions?: Record<string, unknown>;
      returnWebAuthnResponse?: boolean;
      context?: string;
    }) => ClientResult<unknown>;
    listUserPasskeys: (input?: Record<string, never>) => ClientResult<PasskeySummary[]>;
    deletePasskey: (input: { id: string }) => ClientResult<unknown>;
    updatePasskey: (input: { id: string; name: string }) => ClientResult<unknown>;
  };
  organization: {
    create: (input: { name: string; slug: string }) => ClientResult<{ id?: string }>;
    list: (input: Record<string, never>) => ClientResult<OrganizationSummary[]>;
    setActive: (input: { organizationId?: string | null; organizationSlug?: string | null }) => ClientResult<unknown>;
    createTeam: (input: { name: string; organizationId: string }) => ClientResult<{ id?: string }>;
    listTeams: (input: { query: { organizationId: string } }) => ClientResult<unknown>;
    listMembers: (input: { query: { organizationId: string } }) => ClientResult<unknown>;
    listInvitations: (input: { query: { organizationId: string } }) => ClientResult<unknown>;
    inviteMember: (input: {
      email: string;
      role: string | string[];
      organizationId: string;
      teamId?: string;
    }) => ClientResult<unknown>;
    cancelInvitation: (input: { invitationId: string }) => ClientResult<unknown>;
    getInvitation: (input: { query: { id: string } }) => ClientResult<InvitationDetails>;
    acceptInvitation: (input: { invitationId: string }) => ClientResult<unknown>;
    rejectInvitation: (input: { invitationId: string }) => ClientResult<unknown>;
    addTeamMember: (input: { teamId: string; userId: string; organizationId?: string }) => ClientResult<unknown>;
    removeTeamMember: (input: { teamId: string; userId: string; organizationId?: string }) => ClientResult<unknown>;
  };
};

export type InvitationDetails = {
  id: string;
  email: string;
  role: string | string[];
  status: string;
  expiresAt?: string | number;
  organizationId: string;
  organizationName: string;
  organizationSlug?: string;
  inviterEmail?: string;
  teamId?: string | null;
};

// Better Auth's organization plugin currently exposes non-portable inferred types under TS 6.
// Keep a narrow app-local surface here so the build stays green while preserving the methods
// this app actually uses and exercises in the E2E flow.
export const authClient = createAuthClient({
  plugins: [
    organizationClient({
      teams: { enabled: true },
    }),
    twoFactorClient(),
    passkeyClient(),
  ],
}) as unknown as SnarveiAuthClient;
