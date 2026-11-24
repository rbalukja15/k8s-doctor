package collector_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rbalukja15/k8s-doctor/internal/collector"
)

func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

const originsDeployment = `apiVersion: apps/v1
kind: Deployment
metadata: {name: api}
spec:
  selector: {matchLabels: {app: api}}
  template:
    metadata: {labels: {app: api}}
    spec: {containers: [{name: api, image: "app:1"}]}
`

// TestOrigins pins how the collector maps a built name back to the name in
// the source file: affixes from every kustomization on the path, outer
// prefix first, inner suffix first, and generator hashes.
func TestOrigins(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"base/kustomization.yaml": "namePrefix: in-\nnameSuffix: -v1\nresources: [deployment.yaml]\n" +
			"configMapGenerator: [{name: api-config, literals: [A=1]}]\n",
		"base/deployment.yaml": originsDeployment,
		// The tenant layer sets the prefix; the leaf only sets a namespace.
		"tenant-layer/kustomization.yaml": "namePrefix: acme-\nresources: [../base, extra.yaml]\n",
		"tenant-layer/extra.yaml":         "apiVersion: v1\nkind: Service\nmetadata: {name: extra}\nspec: {ports: [{port: 80}]}\n",
		"leaf/kustomization.yaml":         "namespace: acme\nresources: [../tenant-layer]\n",
	})
	snap, err := collector.Manifests(filepath.Join(root, "leaf"))
	if err != nil {
		t.Fatal(err)
	}
	got := snap.Source.Overlay.Origins
	var cm string
	for _, c := range snap.Spec.ConfigMaps {
		cm = c.Name
	}
	want := map[string]string{
		"Deployment/acme/acme-in-api-v1": "api",
		"Service/acme/acme-extra":        "extra",
		"ConfigMap/acme/" + cm:           "api-config",
	}
	if len(got) != len(want) {
		t.Errorf("origins = %v, want %v", got, want)
	}
	for ref, name := range want {
		if got[ref] != name {
			t.Errorf("origin of %s = %q, want %q", ref, got[ref], name)
		}
	}
}

// An object that keeps its name gets no origins entry.
func TestOriginsUnchangedNames(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"kustomization.yaml": "namespace: shop\nresources: [deployment.yaml]\n",
		"deployment.yaml":    originsDeployment,
	})
	snap, err := collector.Manifests(root)
	if err != nil {
		t.Fatal(err)
	}
	if o := snap.Source.Overlay.Origins; len(o) != 0 {
		t.Errorf("origins = %v, want none", o)
	}
}
