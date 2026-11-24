package drift

import (
	"testing"

	"github.com/rbalukja15/k8s-doctor/internal/config"
)

func TestImageRepo(t *testing.T) {
	for image, want := range map[string]string{
		"nginx":                          "nginx",
		"nginx:1.27":                     "nginx",
		"registry:5000/app":              "registry:5000/app",
		"registry:5000/app:2.0":          "registry:5000/app",
		"ghcr.io/acme/app@sha256:abc":    "ghcr.io/acme/app",
		"ghcr.io/acme/app:1.0@sha256:ab": "ghcr.io/acme/app",
	} {
		if got := imageRepo(image); got != want {
			t.Errorf("imageRepo(%q) = %q, want %q", image, got, want)
		}
	}
}

func TestGlob(t *testing.T) {
	cases := []struct {
		pattern, kind, path string
		want                bool
	}{
		{"spec.replicas", "Deployment", "spec.replicas", true},
		{"spec.replicas", "Deployment", "spec.replicasX", false},
		{"**.resources.**", "Deployment", "spec.template.spec.containers[name=a].resources.limits.memory", true},
		{"**.resources", "Deployment", "spec.template.spec.containers[name=a].resources", true},
		{"spec.template.spec.containers[*].image", "Deployment", "spec.template.spec.containers[name=a].image", true},
		{"spec.template.spec.containers*.image", "Deployment", "spec.template.spec.containers[name=a].image", false},
		{"spec.template.spec.containers**.image", "Deployment", "spec.template.spec.containers[name=a].image", true},
		{"metadata.labels[app.kubernetes.io/name]", "Service", "metadata.labels[app.kubernetes.io/name]", true},
		{"metadata.labels.*", "Service", "metadata.labels[app.kubernetes.io/name]", false},
		{"HorizontalPodAutoscaler:spec.maxReplicas", "HorizontalPodAutoscaler", "spec.maxReplicas", true},
		{"HorizontalPodAutoscaler:spec.maxReplicas", "Deployment", "spec.maxReplicas", false},
	}
	for _, c := range cases {
		g, err := parseGlob(c.pattern)
		if err != nil {
			t.Fatal(err)
		}
		o := Options{allow: []glob{g}}
		if got := o.allowed(c.kind, c.path, "a", "b"); got != c.want {
			t.Errorf("%q on %s %s = %v, want %v", c.pattern, c.kind, c.path, got, c.want)
		}
	}
}

func TestPresets(t *testing.T) {
	o, err := NewOptions(configWith("hpaBounds", "resources", "imageTags"))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		kind, path, base, target string
		want                     bool
	}{
		{"HorizontalPodAutoscaler", "spec.minReplicas", "2", "3", true},
		{"HorizontalPodAutoscaler", "spec.metrics[0].target", "Utilization", "AverageValue", false},
		{"Deployment", "spec.template.spec.containers[name=a].resources", "{}", "(unset)", true},
		{"Deployment", "spec.template.spec.containers[name=a].image", "reg/app:1", "reg/app:2", true},
		{"Deployment", "spec.template.spec.initContainers[name=m].image", "reg/app:1", "reg/app@sha256:ab", true},
		{"Deployment", "spec.template.spec.containers[name=a].image", "reg/app:1", "reg/other:1", false},
		{"Service", "metadata.labels.image", "app:1", "app:2", false},
	}
	for _, c := range cases {
		if got := o.allowed(c.kind, c.path, c.base, c.target); got != c.want {
			t.Errorf("%s %s %s -> %s: allowed = %v, want %v", c.kind, c.path, c.base, c.target, got, c.want)
		}
	}
}

func TestHostnameToken(t *testing.T) {
	norm := hostnameNormalizer("staging")
	for in, want := range map[string]string{
		"webshop.staging.example.com":        "webshop.{overlay}.example.com",
		"staging.example.com":                "{overlay}.example.com",
		"https://api-staging.example.com/v1": "https://api-{overlay}.example.com/v1",
		"staging":                            "staging",
		"ENVIRONMENT=staging":                "ENVIRONMENT=staging",
		"registry.example.com/staging/app:1": "registry.example.com/staging/app:1",
		"stagingarea.example.com":            "stagingarea.example.com",
	} {
		if got := norm(in); got != want {
			t.Errorf("norm(%q) = %q, want %q", in, got, want)
		}
	}
}

func configWith(presets ...string) config.Drift { return config.Drift{Presets: &presets} }
