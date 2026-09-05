-- Queries internal/auth runs against Limen's tables. Confined to that package.

-- name: GetAuthSession :one
SELECT
    u."id" AS user_id,
    COALESCE(u."name", '') AS name,
    u."email" AS email,
    u."image" AS image,
    u."two_factor_enabled" AS two_factor_enabled,
    s."id" AS session_id,
    s."expires_at" AS expires_at,
    COALESCE(s."active_organization_id", '') AS active_organization_id
FROM "sessions" s
JOIN "users" u ON u."id" = s."user_id"
WHERE s."token" = $1 AND s."user_id" = $2;

-- name: GetSessionRecord :one
SELECT "user_id", "token", "expires_at" FROM "sessions" WHERE "token" = $1;

-- name: SetSessionActiveOrganization :exec
UPDATE "sessions" SET "active_organization_id" = $2 WHERE "token" = $1;

-- name: ClearActiveOrganizationForUser :exec
UPDATE "sessions" SET "active_organization_id" = NULL WHERE "user_id" = $1 AND "active_organization_id" = $2;

-- name: CountOrganizationMembership :one
SELECT COUNT(*)::int FROM "organization_members" WHERE "organization_id" = $1 AND "user_id" = $2;

-- name: GetMemberRoles :many
-- Every role row the member holds; authz.Highest picks the one that counts.
SELECT COALESCE(r."role", '') AS role
FROM "organization_members" m
JOIN "organization_member_roles" r ON r."member_id" = m."id"
WHERE m."organization_id" = $1 AND m."user_id" = $2;

-- name: GetInvitationToken :one
SELECT "token", "organization_id", "email", "status", "expires_at" FROM "organization_invitations" WHERE "id" = $1;

-- name: CountUsersByEmail :one
SELECT COUNT(*)::int FROM "users" WHERE lower("email") = lower($1);

-- name: DeleteUser :exec
DELETE FROM "users" WHERE "id" = $1;
