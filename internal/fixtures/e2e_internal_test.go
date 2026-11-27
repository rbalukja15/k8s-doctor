package fixtures

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadE2ERejects covers each check LoadE2E makes, one bad file each.
func TestLoadE2ERejects(t *testing.T) {
	const ok = "  - {path: a, status: ok}\n"
	cases := map[string]struct{ targets, findings, want string }{
		"duplicate target":     {targets: ok + ok, want: "listed twice"},
		"unknown status":       {targets: "  - {path: a, status: fine}\n", want: `status "fine"`},
		"failure without text": {targets: "  - {path: a, status: build-failed}\n", want: "needs an error substring"},
		"bad load restrictor":  {targets: "  - {path: a, status: ok, loadRestrictor: rootOnly}\n", want: `loadRestrictor "rootOnly"`},
		"unlisted target": {
			targets:  ok,
			findings: "  - {target: b, rule: r, resource: Deployment/ns/x, field: spec}\n",
			want:     `target "b" is not listed`,
		},
		// Every problem is reported, not only the first one per target.
		"duplicate with bad status": {targets: ok + "  - {path: a, status: fine}\n", want: `status "fine"`},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			body := "description: d\ntargets:\n" + c.targets + "findings:"
			if c.findings == "" {
				body += " []\n"
			} else {
				body += "\n" + c.findings
			}
			if err := os.WriteFile(filepath.Join(dir, ExpectedFile), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := LoadE2E(dir)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want one containing %q", err, c.want)
			}
		})
	}
}

func TestDriftValidateRejects(t *testing.T) {
	for name, e := range map[string]DriftExpected{
		"unknown kind":     {Description: "d", Kind: "maybe", Base: "a", Target: "b"},
		"clean with finds": {Description: "d", Kind: KindClean, Base: "a", Target: "b", Findings: []ExpectedFinding{{Rule: "drift-field", Resource: "Deployment/ns/x", Field: "spec"}}},
		"positive empty":   {Description: "d", Kind: KindPositive, Base: "a", Target: "b"},
		"no target":        {Description: "d", Kind: KindClean, Base: "a"},
		"no description":   {Kind: KindClean, Base: "a", Target: "b"},
		"not a drift rule": {Description: "d", Kind: KindPositive, Base: "a", Target: "b", Findings: []ExpectedFinding{{Rule: "pdb-missing", Resource: "Deployment/ns/x", Field: "spec"}}},
		"no field":         {Description: "d", Kind: KindPositive, Base: "a", Target: "b", Findings: []ExpectedFinding{{Rule: "drift-field", Resource: "Deployment/ns/x"}}},
	} {
		f := DriftFixture{Name: name, Expected: e}
		if err := f.Validate(); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
