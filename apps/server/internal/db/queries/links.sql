-- name: InsertClick :exec
INSERT INTO "click_events" ("id", "link_id", "clicked_at", "ip_hash", "user_agent", "referer", "country", "host", "path", "query_string", "redirect_status_used")
VALUES ($1, $2, now(), $3, $4, $5, $6, $7, $8, $9, $10);

-- name: CreateLink :exec
INSERT INTO "links" ("id", "organization_id", "team_id", "slug", "target_url", "redirect_status", "is_active", "title", "description", "created_by", "updated_by")
VALUES ($1, $2, $3, $4, $5, $6, true, $7, $8, $9, $9);

-- name: InsertLinkHistory :exec
INSERT INTO "link_target_history" ("id", "link_id", "old_target_url", "new_target_url", "changed_by")
VALUES ($1, $2, $3, $4, $5);

-- name: GetLink :one
-- One row with the team name; callers check team access before answering.
SELECT l."id", l."organization_id", l."team_id", t."name" AS team_name, l."slug", l."target_url", l."redirect_status",
    l."is_active", l."title", l."description", l."created_by", l."updated_by", l."created_at", l."updated_at"
FROM "links" l
JOIN "teams" t ON t."id" = l."team_id"
WHERE l."id" = $1;

-- name: GetActiveLinkBySlug :one
SELECT "id", "target_url", "redirect_status" FROM "links" WHERE "slug" = $1 AND "is_active";

-- name: ListLinks :many
-- Newest first within an organization, optionally restricted to a team set
-- (an org member's accessible teams) or one team.
SELECT l."id", l."organization_id", l."team_id", t."name" AS team_name, l."slug", l."target_url", l."redirect_status",
    l."is_active", l."title", l."description", l."created_by", l."updated_by", l."created_at", l."updated_at"
FROM "links" l
JOIN "teams" t ON t."id" = l."team_id"
WHERE l."organization_id" = sqlc.arg(organization_id)
  AND (sqlc.narg(team_id)::text IS NULL OR l."team_id" = sqlc.narg(team_id))
  AND (sqlc.narg(team_ids)::text[] IS NULL OR l."team_id" = ANY(sqlc.narg(team_ids)::text[]))
ORDER BY l."created_at" DESC, l."id" DESC
LIMIT sqlc.arg(page_size) OFFSET sqlc.arg(page_offset);

-- name: CountLinks :one
SELECT COUNT(*)::int FROM "links" l
WHERE l."organization_id" = sqlc.arg(organization_id)
  AND (sqlc.narg(team_id)::text IS NULL OR l."team_id" = sqlc.narg(team_id))
  AND (sqlc.narg(team_ids)::text[] IS NULL OR l."team_id" = ANY(sqlc.narg(team_ids)::text[]));

-- name: UpdateLink :exec
UPDATE "links" SET "target_url" = $2, "redirect_status" = $3, "is_active" = $4, "title" = $5, "description" = $6,
    "updated_by" = $7, "updated_at" = now()
WHERE "id" = $1;

-- name: DeleteLink :execrows
DELETE FROM "links" WHERE "id" = $1;

-- name: ListLinkHistory :many
SELECT "id", "link_id", "old_target_url", "new_target_url", "changed_by", "changed_at"
FROM "link_target_history"
WHERE "link_id" = $1
ORDER BY "changed_at" DESC, "id" DESC
LIMIT $2 OFFSET $3;

-- name: CountLinkHistory :one
SELECT COUNT(*)::int FROM "link_target_history" WHERE "link_id" = $1;

-- name: AnalyticsTotals :one
SELECT COUNT(*)::int AS total_clicks, COUNT(DISTINCT "ip_hash")::int AS unique_visitors
FROM "click_events" WHERE "link_id" = $1 AND "clicked_at" >= $2 AND "clicked_at" < $3;

-- name: AnalyticsByDay :many
SELECT to_char(date_trunc('day', "clicked_at" AT TIME ZONE 'UTC'), 'YYYY-MM-DD') AS day, COUNT(*)::int AS clicks
FROM "click_events" WHERE "link_id" = $1 AND "clicked_at" >= $2 AND "clicked_at" < $3
GROUP BY 1 ORDER BY 1;

-- name: AnalyticsTopReferers :many
SELECT "referer", COUNT(*)::int AS clicks
FROM "click_events" WHERE "link_id" = $1 AND "clicked_at" >= $2 AND "clicked_at" < $3
GROUP BY "referer" ORDER BY clicks DESC, "referer" NULLS LAST LIMIT 10;

-- name: AnalyticsTopCountries :many
SELECT "country", COUNT(*)::int AS clicks
FROM "click_events" WHERE "link_id" = $1 AND "clicked_at" >= $2 AND "clicked_at" < $3
GROUP BY "country" ORDER BY clicks DESC, "country" NULLS LAST LIMIT 10;
