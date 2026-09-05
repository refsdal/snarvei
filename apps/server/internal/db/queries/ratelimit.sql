-- Snarvei's own fixed-window counters (table rate_limit; Limen's rate_limits
-- is separate). The key carries the window index, so a bucket never outlives
-- its window; SweepRateLimit removes old windows opportunistically.

-- name: HitRateLimit :one
INSERT INTO "rate_limit" ("key", "window_start", "count")
VALUES ($1, $2, 1)
ON CONFLICT ("key") DO UPDATE SET "count" = "rate_limit"."count" + 1
RETURNING "count";

-- name: SweepRateLimit :execrows
DELETE FROM "rate_limit" WHERE "window_start" < $1;
