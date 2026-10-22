package checks

import (
	"testing"

	"github.com/rbalukja15/k8s-doctor/internal/rules"
)

func TestBuiltinRulesRegister(t *testing.T) {
	reg := rules.NewRegistry()
	if err := reg.Register(All()...); err != nil {
		t.Fatal(err)
	}
	for _, r := range reg.All() {
		if r.Category() == "" || r.Description() == "" {
			t.Errorf("rule %s is missing a category or description", r.ID())
		}
	}
}

func TestParseImage(t *testing.T) {
	cases := []struct {
		image   string
		tag     string
		mutable bool
	}{
		{"nginx", "", true},
		{"nginx:latest", "latest", true},
		{"nginx:1.27", "1.27", false},
		{"registry:5000/app", "", true},
		{"registry:5000/app:2.0", "2.0", false},
		{"ghcr.io/acme/app:latest@sha256:abc", "latest", false},
		{"ghcr.io/acme/app@sha256:abc", "", false},
	}
	for _, c := range cases {
		ref := parseImage(c.image)
		if ref.tag != c.tag || ref.mutable() != c.mutable {
			t.Errorf("parseImage(%q) = tag %q mutable %v, want tag %q mutable %v",
				c.image, ref.tag, ref.mutable(), c.tag, c.mutable)
		}
	}
}
