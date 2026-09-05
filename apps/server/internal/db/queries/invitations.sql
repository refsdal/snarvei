-- name: GetInvitation :one
-- The public view plus everything accept/register need. The inviter join is
-- LEFT because "inviter_id" is nullable; the team join is LEFT so a deleted
-- team never hides the invitation. "organization_invitations.inviter_id" is
-- ON DELETE CASCADE, so deleting an admin removes their pending invitations.
SELECT i."id", i."organization_id", i."email", COALESCE(i."roles", '') AS roles, i."status",
    i."expires_at", i."created_at",
    o."name" AS organization_name, o."slug" AS organization_slug,
    COALESCE(u."name", '') AS inviter_name,
    it."team_id" AS team_id, t."name" AS team_name
FROM "organization_invitations" i
JOIN "organizations" o ON o."id" = i."organization_id"
LEFT JOIN "users" u ON u."id" = i."inviter_id"
LEFT JOIN "invitation_teams" it ON it."invitation_id" = i."id"
LEFT JOIN "teams" t ON t."id" = it."team_id"
WHERE i."id" = $1;

-- name: ListPendingInvitations :many
SELECT i."id", i."email", COALESCE(i."roles", '') AS roles, i."status", i."expires_at", i."created_at",
    it."team_id" AS team_id, t."name" AS team_name
FROM "organization_invitations" i
LEFT JOIN "invitation_teams" it ON it."invitation_id" = i."id"
LEFT JOIN "teams" t ON t."id" = it."team_id"
WHERE i."organization_id" = $1 AND i."status" = 'pending'
    AND (i."expires_at" IS NULL OR i."expires_at" > now())
ORDER BY i."created_at" DESC;

-- name: SetInvitationTeam :exec
INSERT INTO "invitation_teams" ("invitation_id", "team_id") VALUES ($1, $2)
ON CONFLICT ("invitation_id") DO UPDATE SET "team_id" = EXCLUDED."team_id";
