package auth_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOnlyThisPackageImportsLimen(t *testing.T) {
	root := "../.."
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") {
			return err
		}
		if strings.Contains(p, string(filepath.Separator)+"internal"+string(filepath.Separator)+"auth"+string(filepath.Separator)) {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if strings.Contains(string(b), `"github.com/thecodearcher/limen`) {
			t.Errorf("%s imports Limen; only internal/auth may", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
