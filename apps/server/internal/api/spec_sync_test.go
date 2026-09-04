package api_test

import (
	"bytes"
	"os"
	"testing"
)

// The embedded copy must equal the repo-root spec; go:embed happily embeds a
// stale copy otherwise.
func TestEmbeddedSpecMatchesRepoRoot(t *testing.T) {
	embedded, err := os.ReadFile("snarvei.yaml")
	if err != nil {
		t.Fatalf("read embedded copy: %v", err)
	}
	root, err := os.ReadFile("../../../../openapi/snarvei.yaml")
	if err != nil {
		t.Fatalf("read repo-root spec: %v", err)
	}
	if !bytes.Equal(embedded, root) {
		t.Fatal("internal/api/snarvei.yaml has drifted from openapi/snarvei.yaml: run `go generate ./...` from apps/server and commit")
	}
}
