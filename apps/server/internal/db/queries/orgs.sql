-- name: ListOrganizationsForUser :many
-- One row per organization with the member's highest role, owner > admin > member.
SELECT o."id", o."name", o."slug", o."created_at",
    (SELECT r."role" FROM "organization_member_roles" r
     WHERE r."member_id" = m."id"
     ORDER BY CASE r."role" WHEN 'owner' THEN 3 WHEN 'admin' THEN 2 WHEN 'member' THEN 1 ELSE 0 END DESC
     LIMIT 1) AS role
FROM "organizations" o
JOIN "organization_members" m ON m."organization_id" = o."id" AND m."user_id" = $1
ORDER BY o."name";

-- name: GetOrganization :one
SELECT "id", "name", "slug" FROM "organizations" WHERE "id" = $1;

-- name: GetOrganizationBySlug :one
SELECT "id", "name", "slug" FROM "organizations" WHERE "slug" = $1;

-- name: ListOrganizationMembers :many
SELECT m."id" AS member_id, u."id" AS user_id, COALESCE(u."name", '') AS name, u."email", m."created_at",
    (SELECT r."role" FROM "organization_member_roles" r
     WHERE r."member_id" = m."id"
     ORDER BY CASE r."role" WHEN 'owner' THEN 3 WHEN 'admin' THEN 2 WHEN 'member' THEN 1 ELSE 0 END DESC
     LIMIT 1) AS role
FROM "organization_members" m
JOIN "users" u ON u."id" = m."user_id"
WHERE m."organization_id" = $1
ORDER BY m."created_at";
