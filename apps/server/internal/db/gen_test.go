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
	entries, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		want, _ := os.ReadFile(filepath.Join(tmp, e.Name()))
		got, err := os.ReadFile(filepath.Join("gen", e.Name()))
		if err != nil || string(want) != string(got) {
			t.Fatalf("internal/db/gen/%s is stale: run `go generate ./...` from apps/server and commit", e.Name())
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
