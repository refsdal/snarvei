-- name: GetUserProfile :one
SELECT "id", COALESCE("name", '') AS name, "email", "image", "two_factor_enabled" FROM "users" WHERE "id" = $1;

-- name: UpdateUserName :exec
UPDATE "users" SET "name" = $2, "updated_at" = now() WHERE "id" = $1;

-- name: UpdateUserImage :exec
UPDATE "users" SET "image" = $2, "updated_at" = now() WHERE "id" = $1;

-- name: UpdateUserEmail :exec
UPDATE "users" SET "email" = $2, "updated_at" = now() WHERE "id" = $1;

-- name: ListUserSessions :many
SELECT "id", "token", "created_at", "last_access", "expires_at", COALESCE("metadata", '') AS metadata
FROM "sessions"
WHERE "user_id" = $1 AND "expires_at" > now()
ORDER BY "created_at" DESC;

-- name: GetUserSessionByID :one
SELECT "id", "token" FROM "sessions" WHERE "id" = $1 AND "user_id" = $2;

-- name: CreateEmailChangeRequest :exec
INSERT INTO "email_change_requests" ("id", "user_id", "new_email", "token_hash", "expires_at")
VALUES ($1, $2, $3, $4, $5);

-- name: DeleteEmailChangeRequestsForUser :exec
DELETE FROM "email_change_requests" WHERE "user_id" = $1;

-- name: GetEmailChangeRequest :one
SELECT "id", "user_id", "new_email", "expires_at" FROM "email_change_requests" WHERE "token_hash" = $1;

-- name: ListOrganizationsWhereSoleOwner :many
-- Organizations where this user holds the owner role and nobody else does.
SELECT o."id", o."name"
FROM "organizations" o
JOIN "organization_members" m ON m."organization_id" = o."id" AND m."user_id" = $1
JOIN "organization_member_roles" r ON r."member_id" = m."id" AND r."role" = 'owner'
WHERE (
    SELECT COUNT(*) FROM "organization_members" m2
    JOIN "organization_member_roles" r2 ON r2."member_id" = m2."id" AND r2."role" = 'owner'
    WHERE m2."organization_id" = o."id" AND m2."user_id" <> $1
) = 0;
