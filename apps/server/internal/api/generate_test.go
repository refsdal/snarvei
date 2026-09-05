package api_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Regenerates into a temp dir and diffs against the committed files.
func TestGeneratedCodeIsUpToDate(t *testing.T) {
	if _, err := exec.LookPath("oapi-codegen"); err != nil {
		if os.Getenv("CI") != "" {
			t.Fatal("oapi-codegen missing in CI")
		}
		t.Skip("oapi-codegen not on PATH (run `mise install`)")
	}
	tmp := t.TempDir()
	for _, c := range []struct{ cfg, out string }{
		{"gen/cfg-types.yaml", "gen/types.gen.go"},
		{"gen/cfg-server.yaml", "gen/server.gen.go"},
	} {
		cfgBytes, err := os.ReadFile(c.cfg)
		if err != nil {
			t.Fatal(err)
		}
		// Point the output at the temp dir by rewriting the config's output line.
		tmpCfg := filepath.Join(tmp, filepath.Base(c.cfg))
		tmpOut := filepath.Join(tmp, filepath.Base(c.out))
		rewritten := bytes.Replace(cfgBytes, []byte("output: internal/api/"+c.out), []byte("output: "+tmpOut), 1)
		if err := os.WriteFile(tmpCfg, rewritten, 0o644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("oapi-codegen", "-config", tmpCfg, "../../../../openapi/snarvei.yaml")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("oapi-codegen %s: %v\n%s", c.cfg, err, out)
		}
		want, _ := os.ReadFile(tmpOut)
		got, _ := os.ReadFile(c.out)
		if !bytes.Equal(want, got) {
			t.Fatalf("%s is stale: run `go generate ./...` from apps/server and commit", c.out)
		}
	}
}
