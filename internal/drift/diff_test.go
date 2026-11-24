package drift_test

import (
	"strings"
	"testing"

	"github.com/rbalukja15/k8s-doctor/internal/config"
	"github.com/rbalukja15/k8s-doctor/internal/drift"
	"github.com/rbalukja15/k8s-doctor/internal/rules"
	"github.com/rbalukja15/k8s-doctor/internal/snapshot"
)

func defaults(t *testing.T) drift.Options {
	t.Helper()
	o, err := drift.NewOptions(config.Drift{})
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func diff(t *testing.T, base, target *snapshot.Snapshot, o drift.Options) []rules.Finding {
	t.Helper()
	f, err := drift.Diff(base, target, o)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func describe(fs []rules.Finding) string {
	var out []string
	for _, f := range fs {
		out = append(out, f.RuleID+" "+f.Resource.String()+" "+f.FieldPath+" "+f.Evidence["base"]+" -> "+f.Evidence["target"])
	}
	return strings.Join(out, "\n")
}

// The fixture's tenants differ only by name, host and namespace.
func TestDiffTenantsClean(t *testing.T) {
	if f := diff(t, load(t, "prod/tenant/acme"), load(t, "prod/tenant/globex"), defaults(t)); len(f) != 0 {
		t.Errorf("acme vs globex:\n%s", describe(f))
	}
}

// Staging and a prod tenant differ in image tag and resources, which the
// default presets expect, and in the prod-only HPA, which they do not.
func TestDiffEnvironments(t *testing.T) {
	f := diff(t, load(t, "staging"), load(t, "prod/tenant/acme"), defaults(t))
	if len(f) != 1 || f[0].RuleID != drift.RuleResourceMissing {
		t.Errorf("staging vs acme:\n%s\nwant only the HPA as drift-resource-missing", describe(f))
	}
	none := []string{}
	strict, err := drift.NewOptions(config.Drift{Presets: &none})
	if err != nil {
		t.Fatal(err)
	}
	got := describe(diff(t, load(t, "staging"), load(t, "prod/tenant/acme"), strict))
	for _, want := range []string{
		"spec.template.spec.containers[name=webshop].image registry.example.com/shop/webshop:1.9.0-rc.1 -> registry.example.com/shop/webshop:1.8.0",
		"spec.template.spec.containers[name=webshop].resources.requests.cpu 100m -> 250m",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("with no presets, missing %q in:\n%s", want, got)
		}
	}
}

// Acceptance from #29: a dropped runAsNonRoot is flagged with its path and
// both values, a replicas difference is not, and a config key in one
// tenant only is flagged.
func TestDiffFlagsUnexpectedChanges(t *testing.T) {
	base, target := load(t, "prod/tenant/acme"), load(t, "prod/tenant/globex")
	w := &target.Spec.Workloads[0]
	w.Template.Spec.SecurityContext.RunAsNonRoot = nil
	three := int32(3)
	w.Replicas = &three
	target.Spec.ConfigMaps[0].Keys = append(target.Spec.ConfigMaps[0].Keys, "FEATURE_BETA")

	f := diff(t, base, target, defaults(t))
	if len(f) != 2 {
		t.Fatalf("findings:\n%s\nwant runAsNonRoot and FEATURE_BETA", describe(f))
	}
	sec, key := f[1], f[0]
	if sec.FieldPath != "spec.template.spec.securityContext.runAsNonRoot" || sec.Evidence["base"] != "true" ||
		sec.Evidence["target"] != "(unset)" || sec.Severity != rules.SeverityHigh || sec.Resource.Name != "globex-webshop" {
		t.Errorf("securityContext finding = %+v", sec)
	}
	if key.FieldPath != "data.FEATURE_BETA" || key.Evidence["base"] != "(unset)" || key.Severity != rules.SeverityMedium {
		t.Errorf("config key finding = %+v", key)
	}
}

// Hostnames: the tenant part of a value is a placeholder, so only a real
// difference shows.
func TestDiffHostnames(t *testing.T) {
	base, target := load(t, "prod/tenant/acme"), load(t, "prod/tenant/globex")
	base.Spec.Services[0].Labels["host"] = "acme.webshop.example.com"
	target.Spec.Services[0].Labels["host"] = "globex.webshop.example.com"
	if f := diff(t, base, target, defaults(t)); len(f) != 0 {
		t.Errorf("tenant hosts:\n%s", describe(f))
	}
	target.Spec.Services[0].Labels["host"] = "globex.shop.example.com"
	if f := diff(t, base, target, defaults(t)); len(f) != 1 || f[0].FieldPath != "metadata.labels.host" {
		t.Errorf("a host that differs beyond the tenant:\n%s", describe(f))
	}
}

// The overlay name outside a hostname is a real value: an environment
// label that names the overlay must still show as drift.
func TestDiffOverlayNameOutsideHostname(t *testing.T) {
	base, target := load(t, "staging"), load(t, "prod/tenant/acme")
	base.Spec.Workloads[0].Labels["environment"] = "staging"
	target.Spec.Workloads[0].Labels["environment"] = "acme"
	f := diff(t, base, target, defaults(t))
	var found bool
	for _, x := range f {
		found = found || x.FieldPath == "metadata.labels.environment"
	}
	if !found {
		t.Errorf("environment label difference hidden:\n%s", describe(f))
	}
}

func TestDiffAllowlist(t *testing.T) {
	base, target := load(t, "prod/tenant/acme"), load(t, "prod/tenant/globex")
	target.Spec.ConfigMaps[0].Keys = append(target.Spec.ConfigMaps[0].Keys, "FEATURE_BETA")
	o, err := drift.NewOptions(config.Drift{Allowlist: []string{"ConfigMap:data.FEATURE_*"}})
	if err != nil {
		t.Fatal(err)
	}
	if f := diff(t, base, target, o); len(f) != 0 {
		t.Errorf("allowlisted key still reported:\n%s", describe(f))
	}
	o, err = drift.NewOptions(config.Drift{Allowlist: []string{"Service:data.FEATURE_*"}})
	if err != nil {
		t.Fatal(err)
	}
	if f := diff(t, base, target, o); len(f) != 1 {
		t.Errorf("an allowlist entry for another kind must not apply:\n%s", describe(f))
	}
}

func TestNewOptionsRejectsUnknownPreset(t *testing.T) {
	p := []string{"replicas", "replica"}
	if _, err := drift.NewOptions(config.Drift{Presets: &p}); err == nil || !strings.Contains(err.Error(), `"replica"`) {
		t.Errorf("err = %v, want unknown preset replica", err)
	}
}
