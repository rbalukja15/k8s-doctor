package fixtures_test

import (
	"testing"

	"github.com/rbalukja15/k8s-doctor/internal/fixtures"
)

// TestDriftFixtures runs every drift fixture and diffs actual against
// expected findings.
func TestDriftFixtures(t *testing.T) {
	all, err := fixtures.DiscoverDrift(root)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	for _, f := range all {
		kinds[f.Expected.Kind] = true
		t.Run(f.Name, func(t *testing.T) {
			if err := f.Validate(); err != nil {
				t.Fatal(err)
			}
			got, err := f.Run()
			if err != nil {
				t.Fatal(err)
			}
			d := f.Compare(got)
			for _, k := range d.Missing {
				t.Errorf("missing finding (false negative): %s", fmtKey(k))
			}
			for _, k := range d.Unexpected {
				t.Errorf("unexpected finding (false positive): %s", fmtKey(k))
			}
		})
	}
	for _, k := range []string{fixtures.KindPositive, fixtures.KindClean, fixtures.KindNearMiss} {
		if !kinds[k] {
			t.Errorf("no %s drift fixture", k)
		}
	}
}
