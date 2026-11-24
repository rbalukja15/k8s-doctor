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
			checkReferences(t, build(t, filepath.Join(gitopsDir, tg.Path), types.LoadRestrictionsRootOnly),
				snap.Source.Overlay.NamePrefix, strings.Contains(tg.Path, "/tenant/"))
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

// build runs kustomize directly and decodes the output. The snapshot does
// not model SealedSecrets, KongPlugins, Ingress annotations or envFrom yet
// (#43), so references between them are checked on the rendered objects.
func build(t *testing.T, dir string, lr types.LoadRestrictions) []map[string]any {
	t.Helper()
	opts := krusty.MakeDefaultOptions()
	opts.LoadRestrictions = lr
	res, err := krusty.MakeKustomizer(opts).Run(filesys.MakeFsOnDisk(), dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []map[string]any
	for _, r := range res.Resources() {
		m, err := r.Map()
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, m)
	}
	return out
}

// field reads a nested field of a decoded object; list steps take an index.
func field(obj any, steps ...any) any {
	for _, s := range steps {
		switch k := s.(type) {
		case string:
			m, ok := obj.(map[string]any)
			if !ok {
				return nil
			}
			obj = m[k]
		case int:
			l, ok := obj.([]any)
			if !ok || k >= len(l) {
				return nil
			}
			obj = l[k]
		}
	}
	return obj
}

func named(objs []map[string]any, kind string) []map[string]any {
	var out []map[string]any
	for _, o := range objs {
		if o["kind"] == kind {
			out = append(out, o)
		}
	}
	return out
}

// checkReferences checks that the namePrefix reaches every reference: the
// Deployment's secretRef names the SealedSecret (via the base's
// nameReference config), and every tenant Ingress names its prefixed
// KongPlugin, which must exist.
func checkReferences(t *testing.T, objs []map[string]any, prefix string, tenant bool) {
	t.Helper()
	sealed := named(objs, "SealedSecret")
	if len(sealed) != 1 || field(sealed[0], "metadata", "name") != prefix+"webshop-db" {
		t.Fatalf("SealedSecrets = %v, want one named %swebshop-db", sealed, prefix)
	}
	deploy := named(objs, "Deployment")
	var secretRefs []any
	for _, env := range field(deploy[0], "spec", "template", "spec", "containers", 0, "envFrom").([]any) {
		if ref := field(env, "secretRef", "name"); ref != nil {
			secretRefs = append(secretRefs, ref)
		}
	}
	if len(secretRefs) != 1 || secretRefs[0] != prefix+"webshop-db" {
		t.Errorf("Deployment secretRefs = %v, want [%swebshop-db]", secretRefs, prefix)
	}
	plugins := named(objs, "KongPlugin")
	if !tenant {
		if len(plugins) != 0 {
			t.Errorf("KongPlugins = %v, want none outside the tenants", plugins)
		}
		return
	}
	if len(plugins) != 1 || field(plugins[0], "metadata", "name") != prefix+"ip-whitelist" {
		t.Fatalf("KongPlugins = %v, want one named %sip-whitelist", plugins, prefix)
	}
	for _, ing := range named(objs, "Ingress") {
		if got := field(ing, "metadata", "annotations", "konghq.com/plugins"); got != prefix+"ip-whitelist" {
			t.Errorf("Ingress konghq.com/plugins = %v, want %sip-whitelist", got, prefix)
		}
	}
}
