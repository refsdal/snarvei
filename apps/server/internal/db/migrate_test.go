package db

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func testDatabaseURL() string {
	if url := os.Getenv("TEST_DATABASE_URL"); url != "" {
		return url
	}
	return "postgres://snarvei:snarvei@127.0.0.1:55432/snarvei_test"
}

func resetSchema(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), "DROP SCHEMA public CASCADE; CREATE SCHEMA public"); err != nil {
		t.Fatalf("reset schema: %v", err)
	}
}

func TestApplyMigrationsOnEmptyDatabase(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := New(ctx, testDatabaseURL())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer pool.Close()
	resetSchema(t, pool)

	if err := ApplyMigrations(ctx, testDatabaseURL(), DefaultMigrationLockKey); err != nil {
		t.Fatalf("ApplyMigrations: %v", err)
	}

	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_tables WHERE schemaname = 'public' AND tablename IN ('users','organizations','teams','links','click_events','rate_limit')`).Scan(&n); err != nil {
		t.Fatalf("count tables: %v", err)
	}
	if n != 6 {
		t.Fatalf("expected 6 known tables, found %d", n)
	}

	latest, err := LatestMigrationVersion()
	if err != nil {
		t.Fatalf("LatestMigrationVersion: %v", err)
	}
	var applied int64
	if err := pool.QueryRow(ctx, `SELECT max(version_id) FROM goose_db_version WHERE is_applied`).Scan(&applied); err != nil {
		t.Fatalf("read goose version: %v", err)
	}
	if applied != latest {
		t.Fatalf("applied %d, embedded latest %d", applied, latest)
	}

	// Idempotent: a second run finds nothing pending.
	if err := ApplyMigrations(ctx, testDatabaseURL(), DefaultMigrationLockKey); err != nil {
		t.Fatalf("second ApplyMigrations: %v", err)
	}
}

func TestApplyMigrationsHoldsTheAdvisoryLock(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := New(ctx, testDatabaseURL())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer pool.Close()

	// Hold the lock from an independent connection; ApplyMigrations must block.
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", DefaultMigrationLockKey); err != nil {
		t.Fatalf("lock: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- ApplyMigrations(ctx, testDatabaseURL(), DefaultMigrationLockKey) }()

	select {
	case err := <-done:
		t.Fatalf("ApplyMigrations returned (%v) while the lock was held", err)
	case <-time.After(2 * time.Second):
	}

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_unlock($1)", DefaultMigrationLockKey); err != nil {
		t.Fatalf("unlock: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("ApplyMigrations after unlock: %v", err)
	}
}

func TestIsUniqueViolation(t *testing.T) {
	ctx := context.Background()
	pool, err := New(ctx, testDatabaseURL())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer pool.Close()
	if err := ApplyMigrations(ctx, testDatabaseURL(), DefaultMigrationLockKey); err != nil {
		t.Fatalf("ApplyMigrations: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM organizations WHERE slug = 'dup'`); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organizations (name, slug) VALUES ('a', 'dup')`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO organizations (name, slug) VALUES ('b', 'dup')`)
	if !IsUniqueViolation(err) {
		t.Fatalf("expected a unique violation, got %v", err)
	}
	if IsUniqueViolation(context.Canceled) {
		t.Fatal("context.Canceled is not a unique violation")
	}
}
