package db

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
	"github.com/pressly/goose/v3"
)

// DefaultMigrationLockKey is the advisory-lock key used unless
// MIGRATION_LOCK_KEY overrides it. Every process migrating the same database
// must use the same key, or they stop contending and the lock is pointless.
const DefaultMigrationLockKey int64 = 1935762089

// ApplyMigrations runs every pending goose migration under a session-level
// Postgres advisory lock, so several containers booting at once serialise
// instead of racing to apply the same DDL. It returns errors rather than
// exiting so both the default dispatch mode and `snarvei migrate` own their
// exit code.
//
// pg_advisory_lock is per physical connection, so the lock, the migration and
// the unlock must all run on the same connection: the pool here is pinned to
// exactly one.
func ApplyMigrations(ctx context.Context, databaseURL string, lockKey int64) error {
	sqlDB, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("db: open database: %w", err)
	}
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	if _, err := sqlDB.ExecContext(ctx, "SELECT pg_advisory_lock($1)", lockKey); err != nil {
		return fmt.Errorf("db: acquire advisory lock: %w", err)
	}
	defer func() { _, _ = sqlDB.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", lockKey) }()

	dir, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("db: resolve embedded migrations dir: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, dir)
	if err != nil {
		return fmt.Errorf("db: create goose provider: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("db: apply migrations: %w", err)
	}
	return nil
}
