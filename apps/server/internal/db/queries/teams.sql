-- name: CreateTeam :one
INSERT INTO "teams" ("id", "organization_id", "name") VALUES ($1, $2, $3)
RETURNING "id", "organization_id", "name", "created_at", "updated_at";

-- name: GetTeam :one
SELECT "id", "organization_id", "name", "created_at", "updated_at" FROM "teams" WHERE "id" = $1;

-- name: ListTeams :many
-- Every team in the organization with its member count (org admins).
SELECT t."id", t."organization_id", t."name", t."created_at", t."updated_at",
    (SELECT COUNT(*)::int FROM "team_members" tm WHERE tm."team_id" = t."id") AS member_count
FROM "teams" t
WHERE t."organization_id" = $1
ORDER BY t."name";

-- name: ListTeamsForMember :many
-- Only the teams the user belongs to (org members).
SELECT t."id", t."organization_id", t."name", t."created_at", t."updated_at",
    (SELECT COUNT(*)::int FROM "team_members" tm WHERE tm."team_id" = t."id") AS member_count
FROM "teams" t
JOIN "team_members" me ON me."team_id" = t."id" AND me."user_id" = $2
WHERE t."organization_id" = $1
ORDER BY t."name";

-- name: IsTeamMember :one
SELECT COUNT(*)::int FROM "team_members" WHERE "team_id" = $1 AND "user_id" = $2;

-- name: AddTeamMember :exec
INSERT INTO "team_members" ("team_id", "user_id") VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: RemoveTeamMember :execrows
DELETE FROM "team_members" WHERE "team_id" = $1 AND "user_id" = $2;

-- name: ListTeamMembers :many
SELECT tm."user_id", COALESCE(u."name", '') AS name, u."email", tm."created_at"
FROM "team_members" tm
JOIN "users" u ON u."id" = tm."user_id"
WHERE tm."team_id" = $1
ORDER BY tm."created_at";

-- name: ListAccessibleTeamIDs :many
-- Team ids a member may see in an organization (org admins use ListTeams).
SELECT t."id" FROM "teams" t
JOIN "team_members" tm ON tm."team_id" = t."id" AND tm."user_id" = $2
WHERE t."organization_id" = $1;
