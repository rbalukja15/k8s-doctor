package drift_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/rbalukja15/k8s-doctor/internal/collector"
	"github.com/rbalukja15/k8s-doctor/internal/drift"
	"github.com/rbalukja15/k8s-doctor/internal/snapshot"
)

const overlays = "../../fixtures/e2e/gitops/deployables/webshop/overlays"

func load(t *testing.T, dir string) *snapshot.Snapshot {
	t.Helper()
	s, err := collector.Manifests(filepath.Join(overlays, dir))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func identities(ps []drift.Pair) string {
	var out []string
	for _, p := range ps {
		out = append(out, p.Identity.String())
	}
	return strings.Join(out, " ")
}

// Two environments: different prefixes and namespaces, generator hashes,
// and an HPA that only the prod layer adds.
func TestMatchEnvironments(t *testing.T) {
	m := drift.Match(load(t, "staging"), load(t, "prod/tenant/acme"))
	want := "ConfigMap/webshop-config Deployment/webshop PodDisruptionBudget/webshop Service/webshop"
	if got := identities(m.Pairs); got != want {
		t.Errorf("pairs = %s, want %s", got, want)
	}
	if len(m.OnlyInBase) != 0 {
		t.Errorf("only in staging = %v, want none", m.OnlyInBase)
	}
	if len(m.OnlyInTarget) != 1 || m.OnlyInTarget[0].Identity.String() != "HorizontalPodAutoscaler/webshop" ||
		m.OnlyInTarget[0].Ref.String() != "HorizontalPodAutoscaler/webshop-acme/acme-webshop" {
		t.Fatalf("only in prod = %v, want the acme HPA", m.OnlyInTarget)
	}
	f := m.Findings("staging", "prod")
	if len(f) != 1 || f[0].RuleID != drift.RuleResourceMissing || f[0].Evidence["onlyIn"] != "prod" ||
		f[0].Resource.String() != "HorizontalPodAutoscaler/webshop-acme/acme-webshop" {
		t.Errorf("findings = %+v, want one drift-resource-missing on the acme HPA, only in prod", f)
	}
}

// Two tenants in one environment match one to one despite their prefixes.
func TestMatchTenants(t *testing.T) {
	m := drift.Match(load(t, "prod/tenant/acme"), load(t, "prod/tenant/globex"))
	if len(m.OnlyInBase)+len(m.OnlyInTarget) != 0 {
		t.Errorf("unmatched: only in acme %v, only in globex %v", m.OnlyInBase, m.OnlyInTarget)
	}
	if len(m.Pairs) != 5 {
		t.Errorf("pairs = %s, want all five objects", identities(m.Pairs))
	}
	for _, p := range m.Pairs {
		if !strings.HasPrefix(p.Base.Name, "acme-") || !strings.HasPrefix(p.Target.Name, "globex-") {
			t.Errorf("pair %s joins %s and %s", p.Identity, p.Base, p.Target)
		}
	}
}

func meta(ns, name string) snapshot.ObjectMeta { return snapshot.ObjectMeta{Namespace: ns, Name: name} }

func overlaySnapshot(ns string, origins map[string]string, spec snapshot.Spec) *snapshot.Snapshot {
	return &snapshot.Snapshot{
		Source: snapshot.Source{Overlay: &snapshot.Overlay{Namespace: ns, Origins: origins}},
		Spec:   spec,
	}
}

// An overlay's own Namespace object matches across overlays whatever its
// name, and two objects with one source name in two namespaces stay apart.
func TestMatchNamespaces(t *testing.T) {
	base := overlaySnapshot("dev", nil, snapshot.Spec{
		Namespaces: []snapshot.Namespace{{Name: "dev"}},
		Services:   []snapshot.Service{{ObjectMeta: meta("dev", "api")}, {ObjectMeta: meta("monitoring", "api")}},
	})
	target := overlaySnapshot("prod", nil, snapshot.Spec{
		Namespaces: []snapshot.Namespace{{Name: "prod"}},
		Services:   []snapshot.Service{{ObjectMeta: meta("prod", "api")}},
	})
	m := drift.Match(base, target)
	if got := identities(m.Pairs); got != "Namespace/(overlay namespace)" {
		t.Errorf("pairs = %s, want only the overlay namespaces", got)
	}
	if len(m.OnlyInBase) != 2 || len(m.OnlyInTarget) != 1 {
		t.Errorf("only in base %v, only in target %v: ambiguous names must not be paired", m.OnlyInBase, m.OnlyInTarget)
	}
}

// A prefix, a suffix and a namespace together, with generated objects.
func TestMatchPrefixAndSuffix(t *testing.T) {
	base, err := collector.Manifests("../collector/testdata/kustomize/base")
	if err != nil {
		t.Fatal(err)
	}
	prod, err := collector.Manifests("../collector/testdata/kustomize/overlays/prod")
	if err != nil {
		t.Fatal(err)
	}
	m := drift.Match(base, prod)
	if len(m.OnlyInBase)+len(m.OnlyInTarget) != 0 {
		t.Errorf("unmatched: only in base %v, only in prod %v", m.OnlyInBase, m.OnlyInTarget)
	}
	for _, p := range m.Pairs {
		if !strings.HasPrefix(p.Target.Name, "prod-") {
			t.Errorf("pair %s joins %s and %s", p.Identity, p.Base, p.Target)
		}
	}
	if len(m.Pairs) == 0 {
		t.Error("no pairs")
	}
}

// Two objects with one kind, source name and namespace (two prefixes in one
// overlay) are both kept, under their built names, instead of one
// replacing the other.
func TestMatchAmbiguousIdentity(t *testing.T) {
	origins := map[string]string{"Service/shop/a-api": "api", "Service/shop/b-api": "api"}
	spec := snapshot.Spec{Services: []snapshot.Service{{ObjectMeta: meta("shop", "a-api")}, {ObjectMeta: meta("shop", "b-api")}}}
	m := drift.Match(overlaySnapshot("shop", origins, spec), overlaySnapshot("shop", origins, spec))
	if len(m.Pairs) != 2 || len(m.OnlyInBase)+len(m.OnlyInTarget) != 0 {
		t.Errorf("pairs %s, only in base %v, only in target %v; want both services paired", identities(m.Pairs), m.OnlyInBase, m.OnlyInTarget)
	}
}
