package report

import (
	"github.com/rbalukja15/k8s-doctor/internal/drift"
	"github.com/rbalukja15/k8s-doctor/internal/rules"
	"github.com/rbalukja15/k8s-doctor/internal/snapshot"
)

// Drift identifies the two overlays a drift report compares. Input then
// describes the target.
type Drift struct {
	Base   DriftSide `json:"base"`
	Target DriftSide `json:"target"`
}

// DriftSide is one overlay of a drift comparison.
type DriftSide struct {
	Path    string `json:"path"`
	Overlay string `json:"overlay,omitempty"`
}

func driftSide(s *snapshot.Snapshot) DriftSide {
	d := DriftSide{Path: s.Source.Path}
	if s.Source.Overlay != nil {
		d.Overlay = s.Source.Overlay.Name
	}
	return d
}

// BuildDrift assembles a report from drift findings between two overlays.
// Findings are sorted most severe first, as in a check report, and Rules
// lists the drift finding types.
func BuildDrift(base, target *snapshot.Snapshot, findings []rules.Finding, opts Options) *Report {
	fs := append([]rules.Finding(nil), findings...)
	rules.SortFindings(fs)
	r := &Report{
		SchemaVersion: SchemaVersion,
		Tool:          Tool{Name: "k8s-doctor", Version: opts.Version},
		GeneratedAt:   opts.Now.UTC(),
		Input:         Input{Mode: target.Mode, Path: target.Source.Path},
		Drift:         &Drift{Base: driftSide(base), Target: driftSide(target)},
		Findings:      make([]Finding, 0, len(fs)),
		Groups:        []Group{},
		Rules:         []RuleStatus{},
	}
	r.Input.Overlay = r.Drift.Target.Overlay
	described := map[string]string{}
	for _, info := range drift.Rules() {
		described[info.ID] = info.Description
		r.Rules = append(r.Rules, RuleStatus{
			ID:          info.ID,
			Category:    drift.Category,
			Severity:    info.Severity,
			Modes:       []snapshot.Mode{snapshot.ModeManifest},
			Description: info.Description,
			Status:      StatusRun,
		})
	}
	for _, f := range fs {
		r.Findings = append(r.Findings, Finding{
			Finding:     f,
			Category:    drift.Category,
			Explanation: Explanation{Text: described[f.RuleID], Source: SourceRule},
		})
	}
	r.summarize(len(r.Rules), 0)
	return r
}
