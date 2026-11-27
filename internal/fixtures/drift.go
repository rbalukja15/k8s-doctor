package fixtures

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"sigs.k8s.io/yaml"

	"github.com/rbalukja15/k8s-doctor/internal/collector"
	"github.com/rbalukja15/k8s-doctor/internal/config"
	"github.com/rbalukja15/k8s-doctor/internal/drift"
	"github.com/rbalukja15/k8s-doctor/internal/rules"
	"github.com/rbalukja15/k8s-doctor/internal/snapshot"
)

// DriftDir holds the labeled drift fixtures, which Discover skips.
const DriftDir = "drift"

// DriftExpected is the expected.yaml of a drift fixture: two overlays and
// the findings drift with the default presets must report between them.
type DriftExpected struct {
	Description string `json:"description"`
	// Kind is positive, clean or near-miss, as for rule fixtures.
	Kind string `json:"kind"`
	// Base and Target are kustomize directories, relative to the fixture
	// directory, with forward slashes.
	Base     string            `json:"base"`
	Target   string            `json:"target"`
	Findings []ExpectedFinding `json:"findings"`
}

// DriftFixture is one directory under fixtures/drift.
type DriftFixture struct {
	// Name is the directory path relative to the fixtures root.
	Name     string
	Dir      string
	Expected DriftExpected
}

// DiscoverDrift finds every drift fixture under root/drift, sorted by name.
// A missing drift directory gives none.
func DiscoverDrift(root string) ([]DriftFixture, error) {
	var out []DriftFixture
	err := filepath.WalkDir(filepath.Join(root, DriftDir), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return filepath.SkipAll
			}
			return err
		}
		if d.IsDir() || d.Name() != ExpectedFile {
			return nil
		}
		dir := filepath.Dir(path)
		name, err := filepath.Rel(root, dir)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var e DriftExpected
		if err := yaml.UnmarshalStrict(data, &e); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		out = append(out, DriftFixture{Name: filepath.ToSlash(name), Dir: dir, Expected: e})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Validate checks the fixture: known kind, both overlays named, finding
// counts that match the kind, and findings from drift rules only.
func (f *DriftFixture) Validate() error {
	var errs []error
	e := f.Expected
	switch e.Kind {
	case KindPositive:
		if len(e.Findings) == 0 {
			errs = append(errs, errors.New("positive fixture expects no findings"))
		}
	case KindClean:
		if len(e.Findings) != 0 {
			errs = append(errs, errors.New("clean fixture must expect no findings"))
		}
	case KindNearMiss:
	default:
		errs = append(errs, fmt.Errorf("kind %q is not positive, clean or near-miss", e.Kind))
	}
	if e.Description == "" {
		errs = append(errs, errors.New("description is required"))
	}
	if e.Base == "" || e.Target == "" {
		errs = append(errs, errors.New("base and target are both required"))
	}
	ids := map[string]bool{}
	for _, r := range drift.Rules() {
		ids[r.ID] = true
	}
	for _, ef := range e.Findings {
		if !ids[ef.Rule] {
			errs = append(errs, fmt.Errorf("findings: %q is not a drift rule", ef.Rule))
		}
		if _, err := snapshot.ParseResourceRef(ef.Resource); err != nil {
			errs = append(errs, fmt.Errorf("findings: %w", err))
		}
		if ef.Field == "" {
			errs = append(errs, fmt.Errorf("findings: rule %q on %s has no field", ef.Rule, ef.Resource))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("fixture %s: %w", f.Name, err)
	}
	return nil
}

// Run builds both overlays and diffs them with the default presets.
func (f *DriftFixture) Run() ([]rules.Finding, error) {
	opts, err := drift.NewOptions(config.Drift{})
	if err != nil {
		return nil, err
	}
	base, err := collector.Manifests(f.path(f.Expected.Base))
	if err != nil {
		return nil, fmt.Errorf("base: %w", err)
	}
	target, err := collector.Manifests(f.path(f.Expected.Target))
	if err != nil {
		return nil, fmt.Errorf("target: %w", err)
	}
	return drift.Diff(base, target, opts)
}

func (f *DriftFixture) path(rel string) string {
	return filepath.Join(f.Dir, filepath.FromSlash(strings.TrimSuffix(rel, "/")))
}

// Compare diffs actual drift findings against the expected ones.
func (f *DriftFixture) Compare(got []rules.Finding) Diff {
	return diffKeys(f.Expected.Findings, got, func(string) bool { return true })
}
