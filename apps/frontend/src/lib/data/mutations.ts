import { useMutation, useQueryClient } from "@tanstack/react-query";
import { client, unwrap } from "../api";
import { resetCache } from "../query";
import { keys } from "./keys";
import type { Invitation, Link, Me, Organization, Team } from "./types";
import type { paths } from "../api-schema";

type Body<P extends keyof paths, M extends keyof paths[P]> = paths[P][M] extends {
  requestBody: { content: { "application/json": infer B } };
}
  ? B
  : never;

export function useCreateOrganization() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Body<"/api/organizations", "post">) =>
      unwrap<Organization>(client.POST("/api/organizations", { body })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.organizations }),
  });
}

export function useSwitchOrganization() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (orgId: string) =>
      unwrap<void>(client.POST("/api/organizations/{orgId}/switch", { params: { path: { orgId } } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.me }),
  });
}

export function useCreateTeam(orgId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Body<"/api/organizations/{orgId}/teams", "post">) =>
      unwrap<Team>(client.POST("/api/organizations/{orgId}/teams", { params: { path: { orgId } }, body })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.teams(orgId) }),
  });
}

export function useAddTeamMember(teamId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (userId: string) =>
      unwrap<void>(client.POST("/api/teams/{teamId}/members", { params: { path: { teamId } }, body: { userId } })),
    onSuccess: () =>
      Promise.all([
        qc.invalidateQueries({ queryKey: keys.teamMembers(teamId) }),
        qc.invalidateQueries({ queryKey: ["teams"] }),
      ]),
  });
}

export function useRemoveTeamMember(teamId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (userId: string) =>
      unwrap<void>(client.DELETE("/api/teams/{teamId}/members/{userId}", { params: { path: { teamId, userId } } })),
    onSuccess: () =>
      Promise.all([
        qc.invalidateQueries({ queryKey: keys.teamMembers(teamId) }),
        qc.invalidateQueries({ queryKey: ["teams"] }),
      ]),
  });
}

export function useCreateInvitation(orgId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Body<"/api/organizations/{orgId}/invitations", "post">) =>
      unwrap<Invitation>(client.POST("/api/organizations/{orgId}/invitations", { params: { path: { orgId } }, body })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.invitations(orgId) }),
  });
}

export function useCancelInvitation(orgId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (invitationId: string) =>
      unwrap<void>(
        client.DELETE("/api/organizations/{orgId}/invitations/{invitationId}", {
          params: { path: { orgId, invitationId } },
        }),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.invitations(orgId) }),
  });
}

export function useAcceptInvitation() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (invitationId: string) =>
      unwrap<unknown>(client.POST("/api/invitations/{invitationId}/accept", { params: { path: { invitationId } } })),
    onSuccess: () =>
      Promise.all([
        qc.invalidateQueries({ queryKey: keys.organizations }),
        qc.invalidateQueries({ queryKey: keys.me }),
      ]),
  });
}

export function useRejectInvitation() {
  return useMutation({
    mutationFn: (invitationId: string) =>
      unwrap<unknown>(client.POST("/api/invitations/{invitationId}/reject", { params: { path: { invitationId } } })),
  });
}

export function useRegisterWithInvitation() {
  return useMutation({
    mutationFn: ({
      invitationId,
      ...body
    }: { invitationId: string } & Body<"/api/invitations/{invitationId}/register", "post">) =>
      unwrap<Me>(client.POST("/api/invitations/{invitationId}/register", { params: { path: { invitationId } }, body })),
    onSuccess: () => resetCache(),
  });
}

export function useCreateLink(orgId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Body<"/api/links", "post">) => unwrap<Link>(client.POST("/api/links", { body })),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["links", orgId] }),
  });
}

export function useUpdateLink(orgId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ linkId, ...body }: { linkId: string } & Body<"/api/links/{linkId}", "patch">) =>
      unwrap<Link>(client.PATCH("/api/links/{linkId}", { params: { path: { linkId } }, body })),
    onSuccess: (link) =>
      Promise.all([
        qc.invalidateQueries({ queryKey: ["links", orgId] }),
        qc.invalidateQueries({ queryKey: keys.link(link.id) }),
        qc.invalidateQueries({ queryKey: ["history", link.id] }),
      ]),
  });
}

export function useDeleteLink(orgId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (linkId: string) =>
      unwrap<void>(client.DELETE("/api/links/{linkId}", { params: { path: { linkId } } })),
    onSuccess: (_, linkId) =>
      Promise.all([
        qc.invalidateQueries({ queryKey: ["links", orgId] }),
        qc.removeQueries({ queryKey: keys.link(linkId) }),
      ]),
  });
}

export function useUpdateMe() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Body<"/api/me", "patch">) => unwrap<Me>(client.PATCH("/api/me", { body })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.me }),
  });
}

// The profile-image routes are hand-mounted outside the OpenAPI document
// (apps/server/internal/api/images.go): multipart field "file", ≤ 2 MiB,
// png/jpeg/webp, answering { imageUrl: string | null }. They bypass the typed
// client and go through unwrap's raw-Response path.
export function useUploadProfileImage() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (file: File) => {
      const form = new FormData();
      form.append("file", file);
      return unwrap<{ imageUrl: string | null }>(
        fetch("/api/me/profile-image", { method: "POST", body: form, credentials: "include" }),
      );
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.me }),
  });
}

export function useDeleteProfileImage() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () =>
      unwrap<{ imageUrl: null }>(fetch("/api/me/profile-image", { method: "DELETE", credentials: "include" })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.me }),
  });
}

export function useRequestEmailChange() {
  return useMutation({
    mutationFn: (body: Body<"/api/me/email", "post">) => unwrap<void>(client.POST("/api/me/email", { body })),
  });
}

export function useConfirmEmailChange() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Body<"/api/me/email/confirm", "post">) =>
      unwrap<Me>(client.POST("/api/me/email/confirm", { body })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.me }),
  });
}

export function useRevokeSession() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (sessionId: string) =>
      unwrap<void>(client.DELETE("/api/me/sessions/{sessionId}", { params: { path: { sessionId } } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.sessions }),
  });
}

export function useRevokeOtherSessions() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => unwrap<void>(client.DELETE("/api/me/sessions")),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.sessions }),
  });
}

export function useDeleteMe() {
  return useMutation({
    mutationFn: (body: Body<"/api/me", "delete">) => unwrap<void>(client.DELETE("/api/me", { body })),
    onSuccess: () => resetCache(),
  });
}
