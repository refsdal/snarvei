import { useCallback, useEffect, useMemo, useState } from "react";
import { authClient } from "../lib/legacy-auth-client";
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
import { initialAnalytics, readCollection, readErrorMessage } from "../types";
import { WorkspaceContext, type WorkspaceContextValue } from "./workspace-context";

const refreshSession = async () => {
  const response = await fetch("/api/me", { credentials: "include" });
  if (!response.ok) {
    return null;
  }

  return (await response.json()) as SessionData;
};

type OrganizationData = { teams: Team[] | null; members: Member[] | null; invitations: Invitation[] | null };

/** Pure loaders (no React state) so effects can apply results with a cancellation guard. */
const fetchOrganizationData = async (organizationId: string): Promise<OrganizationData> => {
  const [teamsResult, membersResult, invitationsResult] = await Promise.allSettled([
    authClient.organization.listTeams({ query: { organizationId } }),
    authClient.organization.listMembers({ query: { organizationId } }),
    authClient.organization.listInvitations({ query: { organizationId } }),
  ]);
  return {
    teams: teamsResult.status === "fulfilled" ? readCollection<Team>(teamsResult.value.data, ["teams", "data"]) : null,
    members:
      membersResult.status === "fulfilled"
        ? readCollection<Member>(membersResult.value.data, ["members", "data"])
        : null,
    invitations:
      invitationsResult.status === "fulfilled"
        ? readCollection<Invitation>(invitationsResult.value.data, ["invitations", "data"])
        : null,
  };
};

const fetchLinks = async (organizationId: string): Promise<Link[] | null> => {
  try {
    const response = await fetch(`/api/organizations/${organizationId}/links`, { credentials: "include" });
    return response.ok ? ((await response.json()) as Link[]) : null;
  } catch {
    return null;
  }
};

const fetchLinkDetails = async (
  linkId: string,
): Promise<{ history: HistoryItem[]; analytics: AnalyticsSummary } | null> => {
  try {
    const [historyResponse, analyticsResponse] = await Promise.all([
      fetch(`/api/links/${linkId}/history`, { credentials: "include" }),
      fetch(`/api/links/${linkId}/analytics`, { credentials: "include" }),
    ]);
    if (!historyResponse.ok || !analyticsResponse.ok) {
      return null;
    }
    return {
      history: (await historyResponse.json()) as HistoryItem[],
      analytics: (await analyticsResponse.json()) as AnalyticsSummary,
    };
  } catch {
    return null;
  }
};

export function WorkspaceProvider({ children }: { children: React.ReactNode }) {
  const sessionQuery = authClient.useSession();
  const organizationsQuery = authClient.useListOrganizations();
  const activeOrganizationQuery = authClient.useActiveOrganization();

  const session = (sessionQuery.data as SessionData | null | undefined) ?? null;
  const [selectedOrganizationId, setSelectedOrganizationId] = useState<string | null>(null);
  const [activeTeamId, setActiveTeamId] = useState<string | null>(null);
  const [selectedLinkId, setSelectedLinkId] = useState<string | null>(null);
  const [teams, setTeams] = useState<Team[]>([]);
  const [members, setMembers] = useState<Member[]>([]);
  const [invitations, setInvitations] = useState<Invitation[]>([]);
  const [links, setLinks] = useState<Link[]>([]);
  const [history, setHistory] = useState<HistoryItem[]>([]);
  const [analytics, setAnalytics] = useState<AnalyticsSummary>(initialAnalytics);
  const [message, setMessage] = useState<AppMessage | null>(null);
  const [submitting, setSubmitting] = useState<string | null>(null);
  const [loadingOrganizations, setLoadingOrganizations] = useState(false);
  const [loadedOrganizationId, setLoadedOrganizationId] = useState<string | null>(null);
  const [loadedLinksOrganizationId, setLoadedLinksOrganizationId] = useState<string | null>(null);
  const [loadedDetailsLinkId, setLoadedDetailsLinkId] = useState<string | null>(null);

  const appOrigin = typeof window === "undefined" ? "http://localhost:8787" : window.location.origin;
  const organizations = useMemo(
    () => (organizationsQuery.data ?? []) as OrganizationSummary[],
    [organizationsQuery.data],
  );
  const activeOrganizationFromClient = activeOrganizationQuery.data;

  const activeOrganizationId = useMemo(() => {
    if (!session) {
      return null;
    }

    if (selectedOrganizationId && organizations.some((org) => org.id === selectedOrganizationId)) {
      return selectedOrganizationId;
    }

    if (activeOrganizationFromClient?.id && organizations.some((org) => org.id === activeOrganizationFromClient.id)) {
      return activeOrganizationFromClient.id;
    }

    return organizations[0]?.id ?? null;
  }, [activeOrganizationFromClient, organizations, selectedOrganizationId, session]);

  const activeOrganization = useMemo(
    () => organizations.find((organization) => organization.id === activeOrganizationId) ?? null,
    [activeOrganizationId, organizations],
  );

  const visibleTeams = useMemo(
    () => (activeOrganizationId && loadedOrganizationId === activeOrganizationId ? teams : []),
    [activeOrganizationId, loadedOrganizationId, teams],
  );

  const effectiveTeamId = useMemo(() => {
    if (!session || !activeOrganizationId) {
      return null;
    }

    if (activeTeamId && visibleTeams.some((team) => team.id === activeTeamId)) {
      return activeTeamId;
    }

    return visibleTeams[0]?.id ?? null;
  }, [activeOrganizationId, activeTeamId, session, visibleTeams]);

  const visibleMembers = useMemo(
    () => (activeOrganizationId && loadedOrganizationId === activeOrganizationId ? members : []),
    [activeOrganizationId, loadedOrganizationId, members],
  );

  const visibleInvitations = useMemo(
    () => (activeOrganizationId && loadedOrganizationId === activeOrganizationId ? invitations : []),
    [activeOrganizationId, invitations, loadedOrganizationId],
  );

  const visibleLinks = useMemo(
    () => (activeOrganizationId && loadedLinksOrganizationId === activeOrganizationId ? links : []),
    [activeOrganizationId, links, loadedLinksOrganizationId],
  );

  const effectiveSelectedLinkId = useMemo(() => {
    if (!session || !activeOrganizationId) {
      return null;
    }

    if (selectedLinkId && visibleLinks.some((link) => link.id === selectedLinkId)) {
      return selectedLinkId;
    }

    return visibleLinks[0]?.id ?? null;
  }, [activeOrganizationId, selectedLinkId, session, visibleLinks]);

  const selectedLink = useMemo(
    () => visibleLinks.find((link) => link.id === effectiveSelectedLinkId) ?? null,
    [effectiveSelectedLinkId, visibleLinks],
  );

  const visibleHistory = selectedLink && loadedDetailsLinkId === selectedLink.id ? history : [];
  const visibleAnalytics = selectedLink && loadedDetailsLinkId === selectedLink.id ? analytics : initialAnalytics;

  const refreshOrganizations = async (options?: { silent?: boolean }) => {
    setLoadingOrganizations(true);
    try {
      await organizationsQuery.refetch();
      await refreshSession();
    } catch {
      if (!options?.silent) {
        setMessage({ severity: "error", text: "Failed to load organizations." });
      }
    } finally {
      setLoadingOrganizations(false);
    }
  };

  const applyOrganizationData = useCallback(
    (organizationId: string, data: OrganizationData, options?: { silent?: boolean }) => {
      if (data.teams) {
        const nextTeams = data.teams;
        setTeams(nextTeams);
        setActiveTeamId((current) =>
          current && nextTeams.some((team) => team.id === current) ? current : (nextTeams[0]?.id ?? null),
        );
      } else {
        setTeams([]);
        if (!options?.silent) {
          setMessage({ severity: "error", text: "Failed to load teams for the active organization." });
        }
      }

      if (data.members) {
        setMembers(data.members);
      } else {
        setMembers([]);
        if (!options?.silent) {
          setMessage({ severity: "error", text: "Failed to load organization members." });
        }
      }

      if (data.invitations) {
        setInvitations(data.invitations);
      } else {
        setInvitations([]);
        if (!options?.silent) {
          setMessage({ severity: "error", text: "Failed to load invitations." });
        }
      }

      setLoadedOrganizationId(organizationId);
    },
    [],
  );

  const refreshOrganizationData = async (organizationId: string, options?: { silent?: boolean }) => {
    applyOrganizationData(organizationId, await fetchOrganizationData(organizationId), options);
  };

  const applyLinks = useCallback((organizationId: string, data: Link[] | null, options?: { silent?: boolean }) => {
    setLinks(data ?? []);
    setLoadedLinksOrganizationId(organizationId);
    if (!data && !options?.silent) {
      setMessage({ severity: "error", text: "Failed to load organization links." });
    }
  }, []);

  const applyLinkDetails = useCallback(
    (
      linkId: string,
      data: { history: HistoryItem[]; analytics: AnalyticsSummary } | null,
      options?: { silent?: boolean },
    ) => {
      setHistory(data?.history ?? []);
      setAnalytics(data?.analytics ?? initialAnalytics);
      setLoadedDetailsLinkId(linkId);
      if (!data && !options?.silent) {
        setMessage({ severity: "error", text: "Failed to load link history or analytics." });
      }
    },
    [],
  );

  const refreshSelectedLinkData = async (linkId: string, options?: { silent?: boolean }) => {
    applyLinkDetails(linkId, await fetchLinkDetails(linkId), options);
  };

  // Data loading effects: pure fetch, then apply unless a newer request superseded this one.
  const sessionUserId = session?.user.id ?? null;

  useEffect(() => {
    if (!activeOrganizationId || !sessionUserId) {
      return;
    }
    let cancelled = false;
    void fetchOrganizationData(activeOrganizationId).then((data) => {
      if (!cancelled) {
        applyOrganizationData(activeOrganizationId, data, { silent: true });
      }
    });
    return () => {
      cancelled = true;
    };
  }, [activeOrganizationId, applyOrganizationData, sessionUserId]);

  useEffect(() => {
    if (!activeOrganizationId || !sessionUserId) {
      return;
    }
    let cancelled = false;
    void fetchLinks(activeOrganizationId).then((data) => {
      if (!cancelled) {
        applyLinks(activeOrganizationId, data, { silent: true });
      }
    });
    return () => {
      cancelled = true;
    };
  }, [activeOrganizationId, applyLinks, sessionUserId]);

  const selectedLinkIdForDetails = selectedLink?.id ?? null;
  useEffect(() => {
    if (!selectedLinkIdForDetails) {
      return;
    }
    let cancelled = false;
    void fetchLinkDetails(selectedLinkIdForDetails).then((data) => {
      if (!cancelled) {
        applyLinkDetails(selectedLinkIdForDetails, data, { silent: true });
      }
    });
    return () => {
      cancelled = true;
    };
  }, [applyLinkDetails, selectedLinkIdForDetails]);

  const signUp = async (input: { name: string; email: string; password: string }) => {
    setSubmitting("signup");
    setMessage(null);
    const result = await authClient.signUp.email(input);
    if (result.error) {
      setMessage({ severity: "error", text: result.error.message ?? "Unable to sign up." });
      setSubmitting(null);
      return false;
    }

    await Promise.all([sessionQuery.refetch(), refreshOrganizations({ silent: true })]);
    setMessage({ severity: "success", text: "Account created." });
    setSubmitting(null);
    return true;
  };

  const signIn = async (input: { email: string; password: string }) => {
    setSubmitting("signin");
    setMessage(null);
    const result = await authClient.signIn.email(input);
    if (result.error) {
      setMessage({ severity: "error", text: result.error.message ?? "Unable to sign in." });
      setSubmitting(null);
      return { ok: false };
    }

    const signInData = result.data as { twoFactorRedirect?: boolean; twoFactorMethods?: string[] } | null | undefined;
    if (signInData?.twoFactorRedirect) {
      setMessage({ severity: "info", text: "Enter your verification code to finish signing in." });
      setSubmitting(null);
      return {
        ok: false,
        requiresTwoFactor: true,
        twoFactorMethods: signInData.twoFactorMethods ?? [],
      };
    }

    await Promise.all([sessionQuery.refetch(), refreshOrganizations({ silent: true })]);
    setMessage({ severity: "success", text: "Signed in." });
    setSubmitting(null);
    return { ok: true };
  };

  const signOut = async () => {
    await authClient.signOut();
    setSelectedOrganizationId(null);
    setActiveTeamId(null);
    setSelectedLinkId(null);
    setTeams([]);
    setMembers([]);
    setInvitations([]);
    setLinks([]);
    setHistory([]);
    setAnalytics(initialAnalytics);
    setLoadedOrganizationId(null);
    setLoadedLinksOrganizationId(null);
    setLoadedDetailsLinkId(null);
    await Promise.all([sessionQuery.refetch(), organizationsQuery.refetch()]);
  };

  const switchOrganization = async (organizationId: string) => {
    setSelectedOrganizationId(organizationId);
    setActiveTeamId(null);
    setSelectedLinkId(null);
    await authClient.organization.setActive({ organizationId });
    await Promise.all([activeOrganizationQuery.refetch(), refreshOrganizations({ silent: true })]);
    await refreshOrganizationData(organizationId, { silent: true });
  };

  const createOrganization = async (input: { name: string; slug: string }) => {
    setSubmitting("create-organization");
    setMessage(null);
    const result = await authClient.organization.create(input);
    if (result.error) {
      setMessage({ severity: "error", text: result.error.message ?? "Unable to create organization." });
      setSubmitting(null);
      return null;
    }

    await refreshOrganizations({ silent: true });
    const nextOrganizations = (organizationsQuery.data ?? []) as OrganizationSummary[];
    const createdOrganizationId =
      result.data?.id ??
      nextOrganizations.find((organization) => organization.slug === input.slug || organization.name === input.name)
        ?.id ??
      null;

    if (createdOrganizationId) {
      await switchOrganization(createdOrganizationId);
    }

    setMessage({ severity: "success", text: "Organization created." });
    setSubmitting(null);
    return createdOrganizationId;
  };

  const createTeam = async (input: { name: string }) => {
    if (!activeOrganizationId) {
      return null;
    }

    setSubmitting("create-team");
    setMessage(null);
    const result = await authClient.organization.createTeam({
      name: input.name,
      organizationId: activeOrganizationId,
    });

    if (result.error) {
      setMessage({ severity: "error", text: result.error.message ?? "Unable to create team." });
      setSubmitting(null);
      return null;
    }

    await refreshOrganizationData(activeOrganizationId, { silent: true });
    if (result.data?.id) {
      setActiveTeamId(result.data.id);
    }

    setMessage({ severity: "success", text: "Team created." });
    setSubmitting(null);
    return result.data?.id ?? null;
  };

  const createLink = async (input: {
    teamId: string;
    targetUrl: string;
    slug?: string;
    redirectStatus: 301 | 302 | 307;
    title?: string;
    description?: string;
  }) => {
    setSubmitting("create-link");
    setMessage(null);
    const response = await fetch("/api/links", {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input),
    });

    if (!response.ok) {
      setMessage({ severity: "error", text: await readErrorMessage(response, "Unable to create link.") });
      setSubmitting(null);
      return null;
    }

    const createdLink = (await response.json()) as Link;
    setLoadedLinksOrganizationId(createdLink.organizationId);
    setLinks((current) => [createdLink, ...current.filter((link) => link.id !== createdLink.id)]);
    setSelectedLinkId(createdLink.id);
    setMessage({ severity: "success", text: "Short link created." });
    setSubmitting(null);
    return createdLink;
  };

  const updateLink = async (linkId: string, values: SelectedLinkFormValues) => {
    setSubmitting("update-link");
    setMessage(null);
    const response = await fetch(`/api/links/${linkId}`, {
      method: "PATCH",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        targetUrl: values.targetUrl,
        title: values.title || null,
        description: values.description || null,
        redirectStatus: values.redirectStatus,
        isActive: values.isActive,
      }),
    });

    if (!response.ok) {
      setMessage({ severity: "error", text: await readErrorMessage(response, "Unable to update the selected link.") });
      setSubmitting(null);
      return null;
    }

    const updatedLink = (await response.json()) as Link;
    setLinks((current) => current.map((link) => (link.id === updatedLink.id ? updatedLink : link)));
    await refreshSelectedLinkData(updatedLink.id, { silent: true });
    setMessage({ severity: "success", text: "Link updated." });
    setSubmitting(null);
    return updatedLink;
  };

  const deleteLink = async (linkId: string) => {
    setSubmitting("delete-link");
    setMessage(null);
    const response = await fetch(`/api/links/${linkId}`, {
      method: "DELETE",
      credentials: "include",
    });

    if (!response.ok) {
      setMessage({ severity: "error", text: await readErrorMessage(response, "Unable to delete the selected link.") });
      setSubmitting(null);
      return false;
    }

    setLinks((current) => current.filter((link) => link.id !== linkId));
    setSelectedLinkId((current) => (current === linkId ? null : current));
    setHistory([]);
    setAnalytics(initialAnalytics);
    setLoadedDetailsLinkId(null);
    setMessage({ severity: "success", text: "Link deleted." });
    setSubmitting(null);
    return true;
  };

  const inviteMember = async (input: { email: string; role: InvitationRole; teamId?: string | null }) => {
    if (!activeOrganizationId) {
      return false;
    }

    setSubmitting("invite-member");
    setMessage(null);
    const result = await authClient.organization.inviteMember({
      email: input.email,
      role: input.role,
      organizationId: activeOrganizationId,
      teamId: input.teamId ?? undefined,
    });

    if (result.error) {
      setMessage({ severity: "error", text: result.error.message ?? "Unable to send invitation." });
      setSubmitting(null);
      return false;
    }

    await refreshOrganizationData(activeOrganizationId, { silent: true });
    setMessage({ severity: "success", text: `Invitation sent to ${input.email}.` });
    setSubmitting(null);
    return true;
  };

  const cancelInvitation = async (invitationId: string) => {
    if (!activeOrganizationId) {
      return false;
    }

    setSubmitting(`cancel-${invitationId}`);
    setMessage(null);
    const result = await authClient.organization.cancelInvitation({ invitationId });
    if (result.error) {
      setMessage({ severity: "error", text: result.error.message ?? "Unable to cancel invitation." });
      setSubmitting(null);
      return false;
    }

    await refreshOrganizationData(activeOrganizationId, { silent: true });
    setMessage({ severity: "success", text: "Invitation cancelled." });
    setSubmitting(null);
    return true;
  };

  // Loading is derived from what has been loaded for the current selection (no loading flags set inside effects).
  const loadingOrganizationData = Boolean(activeOrganizationId) && loadedOrganizationId !== activeOrganizationId;
  const loadingTeams = loadingOrganizationData;
  const loadingMembers = loadingOrganizationData;
  const loadingInvitations = loadingOrganizationData;
  const loadingLinks = Boolean(activeOrganizationId) && loadedLinksOrganizationId !== activeOrganizationId;
  const loadingDetails = Boolean(selectedLink) && loadedDetailsLinkId !== selectedLink?.id;

  const value: WorkspaceContextValue = {
    session,
    sessionPending: sessionQuery.isPending,
    organizationsPending: organizationsQuery.isPending,
    organizations,
    activeOrganizationId,
    activeOrganization,
    teams: visibleTeams,
    activeTeamId: effectiveTeamId,
    members: visibleMembers,
    invitations: visibleInvitations,
    links: visibleLinks,
    selectedLink,
    history: visibleHistory,
    analytics: visibleAnalytics,
    loadingOrganizations,
    loadingTeams,
    loadingMembers,
    loadingInvitations,
    loadingLinks,
    loadingDetails,
    submitting,
    message,
    appOrigin,
    setMessage,
    refreshSessionState: async () => {
      await sessionQuery.refetch();
    },
    signIn,
    signUp,
    signOut,
    switchOrganization,
    createOrganization,
    createTeam,
    setSelectedLinkId,
    refreshOrganizations,
    refreshOrganizationData,
    refreshSelectedLinkData,
    createLink,
    updateLink,
    deleteLink,
    inviteMember,
    cancelInvitation,
    getLinkById: (linkId: string) => visibleLinks.find((link) => link.id === linkId) ?? null,
  };

  return <WorkspaceContext.Provider value={value}>{children}</WorkspaceContext.Provider>;
}
