-- name: InsertClick :exec
INSERT INTO "click_events" ("id", "link_id", "clicked_at", "ip_hash", "user_agent", "referer", "country", "host", "path", "query_string", "redirect_status_used")
VALUES ($1, $2, now(), $3, $4, $5, $6, $7, $8, $9, $10);
