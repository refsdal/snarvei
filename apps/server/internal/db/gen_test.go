package db_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestSqlcOutputIsUpToDate regenerates into a temp dir and diffs against the
// committed internal/db/gen. Skips locally without sqlc on PATH; fails in CI.
func TestSqlcOutputIsUpToDate(t *testing.T) {
	if _, err := exec.LookPath("sqlc"); err != nil {
		if os.Getenv("CI") != "" {
			t.Fatal("sqlc missing in CI")
		}
		t.Skip("sqlc not on PATH (run `mise install`)")
	}
	tmp := t.TempDir()
	cfg, err := os.ReadFile("../../sqlc.yaml")
	if err != nil {
		t.Fatal(err)
	}
	// sqlc resolves every path in the config with filepath.Join(configDir,
	// path) unconditionally (Go's filepath.Join does not special-case an
	// already-absolute second argument), so the rewritten config must live
	// next to the real one (apps/server) for the unmodified schema/queries
	// relative paths to keep resolving there, and the scratch "out" dir must
	// be expressed relative to that same directory.
	serverDir, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	relOut, err := filepath.Rel(serverDir, tmp)
	if err != nil {
		t.Fatal(err)
	}
	rewritten := replaceOnce(string(cfg), `out: "internal/db/gen"`, `out: "`+relOut+`"`)
	tmpCfg := filepath.Join(serverDir, ".sqlc-drift-check.yaml")
	if err := os.WriteFile(tmpCfg, []byte(rewritten), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(tmpCfg) })
	cmd := exec.Command("sqlc", "generate", "-f", tmpCfg)
	cmd.Dir = "../.."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sqlc generate: %v\n%s", err, out)
	}
	fresh, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadDir("gen")
	if err != nil {
		t.Fatal(err)
	}
	// Compare the UNION of both listings: checking only the fresh output's
	// names (as an earlier version of this test did) misses a file that is
	// committed under gen/ but that a fresh `sqlc generate` no longer
	// produces — a deleted query, or a renamed one whose generated filename
	// changed.
	names := map[string]struct{}{}
	for _, e := range fresh {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".go" {
			names[e.Name()] = struct{}{}
		}
	}
	for _, e := range committed {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".go" {
			names[e.Name()] = struct{}{}
		}
	}
	for name := range names {
		want, wantErr := os.ReadFile(filepath.Join(tmp, name))
		got, gotErr := os.ReadFile(filepath.Join("gen", name))
		switch {
		case wantErr != nil && gotErr == nil:
			t.Fatalf("internal/db/gen/%s is orphaned (committed but a fresh sqlc generate no longer produces it): run `go generate ./...` from apps/server and commit", name)
		case wantErr == nil && gotErr != nil:
			t.Fatalf("internal/db/gen/%s is missing (a fresh sqlc generate produces it but it is not committed): run `go generate ./...` from apps/server and commit", name)
		case wantErr != nil && gotErr != nil:
			t.Fatalf("internal/db/gen/%s: could not read either side (fresh: %v, committed: %v)", name, wantErr, gotErr)
		case string(want) != string(got):
			t.Fatalf("internal/db/gen/%s is stale: run `go generate ./...` from apps/server and commit", name)
		}
	}
}

func replaceOnce(s, old, new string) string {
	i := len(s)
	for j := 0; j+len(old) <= len(s); j++ {
		if s[j:j+len(old)] == old {
			i = j
			break
		}
	}
	if i == len(s) {
		return s
	}
	return s[:i] + new + s[i+len(old):]
}
