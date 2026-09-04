// Package testrig gives every test package a real, migrated, empty Postgres.
// Setup probes goose_db_version on every call (not sync.Once: migrate_test
// drops the schema and test order is not guaranteed), truncates every public
// table except goose_db_version, and closes the pool on cleanup. Truncation is
// process-wide, so run the suite with `go test -p 1 ./...`.
package testrig

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/refsdal/snarvei/server/internal/db"
)

// DatabaseURL is TEST_DATABASE_URL or the docker-compose.test.yml default.
func DatabaseURL() string {
	if url := os.Getenv("TEST_DATABASE_URL"); url != "" {
		return url
	}
	return "postgres://snarvei:snarvei@127.0.0.1:55432/snarvei_test"
}

var migrateMu sync.Mutex

// Rig is a migrated, truncated database for one test.
type Rig struct {
	Pool *pgxpool.Pool
}

// Setup returns a Rig whose pool closes when the test ends.
func Setup(t *testing.T) *Rig {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	url := DatabaseURL()
	pool, err := db.New(ctx, url)
	if err != nil {
		t.Fatalf("testrig: open pool: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := ensureMigrated(ctx, url, pool); err != nil {
		t.Fatalf("testrig: migrate: %v", err)
	}
	if err := truncateAll(ctx, pool); err != nil {
		t.Fatalf("testrig: truncate: %v", err)
	}
	return &Rig{Pool: pool}
}

func ensureMigrated(ctx context.Context, url string, pool *pgxpool.Pool) error {
	migrateMu.Lock()
	defer migrateMu.Unlock()

	latest, err := db.LatestMigrationVersion()
	if err != nil {
		return err
	}
	var reg *string
	if err := pool.QueryRow(ctx, `SELECT to_regclass('public.goose_db_version')::text`).Scan(&reg); err != nil {
		return fmt.Errorf("probe schema: %w", err)
	}
	if reg != nil {
		var applied int64
		if err := pool.QueryRow(ctx, `SELECT COALESCE(MAX(version_id), 0) FROM goose_db_version WHERE is_applied`).Scan(&applied); err != nil {
			return fmt.Errorf("read applied version: %w", err)
		}
		if applied >= latest {
			return nil
		}
	}
	return db.ApplyMigrations(ctx, url, db.DefaultMigrationLockKey)
}

func truncateAll(ctx context.Context, pool *pgxpool.Pool) error {
	rows, err := pool.Query(ctx, `SELECT tablename FROM pg_tables WHERE schemaname = 'public' AND tablename <> 'goose_db_version'`)
	if err != nil {
		return fmt.Errorf("list tables: %w", err)
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return fmt.Errorf("scan table: %w", err)
		}
		tables = append(tables, `"`+name+`"`)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(tables) == 0 {
		return nil
	}
	_, err = pool.Exec(ctx, "TRUNCATE TABLE "+strings.Join(tables, ", ")+" RESTART IDENTITY CASCADE")
	return err
}
