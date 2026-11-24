package fixtures_test

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	"github.com/rbalukja15/k8s-doctor/internal/collector"
	"github.com/rbalukja15/k8s-doctor/internal/fixtures"
	"github.com/rbalukja15/k8s-doctor/internal/rules"
	"github.com/rbalukja15/k8s-doctor/internal/rules/checks"
	"github.com/rbalukja15/k8s-doctor/internal/snapshot"
)

const gitopsDir = root + "/" + fixtures.E2EDir + "/gitops"

// TestE2EGitOps builds every target of the synthetic GitOps layout with
// the manifest collector and runs every rule on it.
func TestE2EGitOps(t *testing.T) {
	exp, err := fixtures.LoadE2E(gitopsDir)
	if err != nil {
		t.Fatal(err)
	}
	apps, err := fixtures.ApplicationPaths(gitopsDir)
	if err != nil {
		t.Fatal(err)
	}
	var listed []string
	for _, tg := range exp.Targets {
		listed = append(listed, tg.Path)
	}
	slices.Sort(listed)
	if !slices.Equal(apps, listed) {
		t.Fatalf("expected.yaml targets %v, Applications point at %v", listed, apps)
	}

	engine, err := rules.NewEngine(checks.Registry(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tg := range exp.Targets {
		t.Run(tg.Path, func(t *testing.T) {
			snap, err := collector.Manifests(filepath.Join(gitopsDir, tg.Path))
			if tg.Status == fixtures.TargetBuildFailed {
				if err == nil || !strings.Contains(err.Error(), tg.Error) {
					t.Fatalf("build error = %v, want one containing %q", err, tg.Error)
				}
				if tg.LoadRestrictor == "none" {
					// The layout itself must be valid: it fails only for the
					// restrictor, which #39 makes configurable.
					build(t, filepath.Join(gitopsDir, tg.Path), types.LoadRestrictionsNone)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			checkTargetShape(t, snap)
			checkReferences(t, build(t, filepath.Join(gitopsDir, tg.Path), types.LoadRestrictionsRootOnly), snap.Source.Overlay.NamePrefix)
			want := map[rules.Key]bool{}
			for _, f := range exp.Findings {
				if f.Target == tg.Path {
					want[f.Key()] = true
				}
			}
			res := engine.Run(snap)
			for i := range res.Findings {
				k := res.Findings[i].Key()
				if !want[k] {
					t.Errorf("unexpected finding (false positive): %s", fmtKey(k))
				}
				delete(want, k)
			}
			for k := range want {
				t.Errorf("missing finding (false negative): %s", fmtKey(k))
			}
		})
	}
}

// checkTargetShape pins what the layout is for: the overlay's prefix and
// namespace are recorded and applied, generator hashes and the SealedSecret
// name reference follow the prefix, and no ciphertext is collected.
func checkTargetShape(t *testing.T, s *snapshot.Snapshot) {
	t.Helper()
	o := s.Source.Overlay
	if o == nil || o.NamePrefix == "" || o.Namespace == "" {
		t.Fatalf("overlay = %+v, want a namePrefix and namespace", o)
	}
	if len(s.Spec.Workloads) != 1 {
		t.Fatalf("got %d workloads, want the webshop Deployment", len(s.Spec.Workloads))
	}
	w := s.Spec.Workloads[0]
	if w.Name != o.NamePrefix+"webshop" || w.Namespace != o.Namespace {
		t.Errorf("Deployment is %s/%s, want %s/%swebshop", w.Namespace, w.Name, o.Namespace, o.NamePrefix)
	}
	if len(s.Spec.ConfigMaps) != 1 || !strings.HasPrefix(s.Spec.ConfigMaps[0].Name, o.NamePrefix+"webshop-config-") {
		t.Errorf("ConfigMaps = %+v, want one prefixed, hash-suffixed webshop-config", s.Spec.ConfigMaps)
	}
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "AgFAKE") {
		t.Error("SealedSecret ciphertext reached the snapshot")
	}
}

// build runs kustomize directly. The snapshot does not model SealedSecrets,
// KongPlugins or envFrom yet (#43), so references between them are checked
// on the rendered YAML.
func build(t *testing.T, dir string, lr types.LoadRestrictions) string {
	t.Helper()
	opts := krusty.MakeDefaultOptions()
	opts.LoadRestrictions = lr
	res, err := krusty.MakeKustomizer(opts).Run(filesys.MakeFsOnDisk(), dir)
	if err != nil {
		t.Fatal(err)
	}
	out, err := res.AsYaml()
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// checkReferences checks that the namePrefix reaches every reference: the
// Deployment's secretRef follows the SealedSecret (via the base's
// nameReference config), and a tenant Ingress names its prefixed KongPlugin.
func checkReferences(t *testing.T, out, prefix string) {
	t.Helper()
	if n := strings.Count(out, "name: "+prefix+"webshop-db\n"); n != 2 {
		t.Errorf("%swebshop-db appears %d times, want the SealedSecret and the Deployment's secretRef", prefix, n)
	}
	if strings.Contains(out, "kind: KongPlugin") {
		if !strings.Contains(out, "konghq.com/plugins: "+prefix+"ip-whitelist\n") ||
			!strings.Contains(out, "name: "+prefix+"ip-whitelist\n") {
			t.Errorf("the Ingress does not name the prefixed KongPlugin %sip-whitelist", prefix)
		}
	}
}
