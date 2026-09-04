package db

import (
	"embed"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// migrationsFS embeds the goose migration files so the image needs no
// separate copy of the SQL.
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

// LatestMigrationVersion is the highest embedded migration version, derived
// from the filenames. testrig compares it against goose_db_version.
func LatestMigrationVersion() (int64, error) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return 0, fmt.Errorf("db: read embedded migrations: %w", err)
	}
	var latest int64
	for _, entry := range entries {
		version, _, ok := strings.Cut(entry.Name(), "_")
		if !ok {
			continue
		}
		n, err := strconv.ParseInt(version, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("db: migration %q has no numeric version prefix", entry.Name())
		}
		if n > latest {
			latest = n
		}
	}
	if latest == 0 {
		return 0, errors.New("db: no embedded migrations found")
	}
	return latest, nil
}
