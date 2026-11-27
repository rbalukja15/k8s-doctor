package collector

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// kustomize rejects a cycle when it builds, but the tree walk must stop on
// its own rather than recurse with an ever longer prefix.
func TestReadKustTreeCycle(t *testing.T) {
	root := t.TempDir()
	for dir, body := range map[string]string{
		"a": "namePrefix: a-\nresources: [../b]\n",
		"b": "namePrefix: b-\nresources: [../a]\n",
	} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, dir, "kustomization.yaml"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := readKustTree(filepath.Join(root, "a")); err == nil || !strings.Contains(err.Error(), "includes itself") {
		t.Errorf("err = %v, want a cycle error", err)
	}
}
