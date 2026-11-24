package fixtures

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"
)

// Target statuses in an end-to-end expected.yaml. They match the target
// statuses of the v2 report (#41).
const (
	TargetOK          = "ok"
	TargetBuildFailed = "build-failed"
)

// E2E is the expected.yaml of an end-to-end layout under fixtures/e2e: a
// whole repository with several targets, rather than one input.
type E2E struct {
	Description string      `json:"description"`
	Targets     []E2ETarget `json:"targets"`
	// Findings are expected from every rule, across all targets.
	Findings []E2EFinding `json:"findings"`
}

// E2ETarget is one kustomization the layout deploys.
type E2ETarget struct {
	// Path is relative to the layout root, with forward slashes.
	Path   string `json:"path"`
	Status string `json:"status"`
	// Error is a substring of the build error when Status is build-failed.
	Error string `json:"error,omitempty"`
	// LoadRestrictor is "none" for a target that reads files outside its
	// own directory and so builds only with LoadRestrictionsNone.
	LoadRestrictor string `json:"loadRestrictor,omitempty"`
}

// E2EFinding is an expected finding in one target.
type E2EFinding struct {
	Target string `json:"target"`
	ExpectedFinding
}

// LoadE2E reads and checks the expected.yaml of an end-to-end layout.
func LoadE2E(dir string) (*E2E, error) {
	path := filepath.Join(dir, ExpectedFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var e E2E
	if err := yaml.UnmarshalStrict(data, &e); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	var errs []error
	if e.Description == "" {
		errs = append(errs, errors.New("description is required"))
	}
	paths := map[string]bool{}
	for _, t := range e.Targets {
		switch {
		case paths[t.Path]:
			errs = append(errs, fmt.Errorf("target %s is listed twice", t.Path))
		case t.Status == TargetBuildFailed && t.Error == "":
			errs = append(errs, fmt.Errorf("target %s: build-failed needs an error substring", t.Path))
		case t.Status != TargetOK && t.Status != TargetBuildFailed:
			errs = append(errs, fmt.Errorf("target %s: status %q is not ok or build-failed", t.Path, t.Status))
		case t.LoadRestrictor != "" && t.LoadRestrictor != "none":
			errs = append(errs, fmt.Errorf("target %s: loadRestrictor %q is not none", t.Path, t.LoadRestrictor))
		}
		paths[t.Path] = true
	}
	for _, f := range e.Findings {
		if !paths[f.Target] {
			errs = append(errs, fmt.Errorf("finding %s on %s: target %q is not listed", f.Rule, f.Resource, f.Target))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &e, nil
}

// ApplicationPaths returns the source paths of every Argo CD Application
// under dir/applications, sorted. It is the fixture's own view of what
// gets deployed, used to check expected.yaml lists every target; target
// discovery proper is #38.
func ApplicationPaths(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(filepath.Join(dir, "applications"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		r := utilyaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(data)))
		for {
			doc, err := r.Read()
			if errors.Is(err, io.EOF) {
				return nil
			}
			if err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			var app struct {
				Kind string `json:"kind"`
				Spec struct {
					Source  *struct{ Path string }  `json:"source"`
					Sources []struct{ Path string } `json:"sources"`
				} `json:"spec"`
			}
			if err := yaml.Unmarshal(doc, &app); err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			if app.Kind != "Application" {
				continue
			}
			if app.Spec.Source != nil && app.Spec.Source.Path != "" {
				out = append(out, app.Spec.Source.Path)
			}
			for _, s := range app.Spec.Sources {
				if s.Path != "" {
					out = append(out, s.Path)
				}
			}
		}
	})
	sort.Strings(out)
	return out, err
}
