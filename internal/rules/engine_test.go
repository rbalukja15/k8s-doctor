package rules_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/rbalukja15/k8s-doctor/internal/config"
	"github.com/rbalukja15/k8s-doctor/internal/rules"
	"github.com/rbalukja15/k8s-doctor/internal/snapshot"
)

// fakeRule emits one finding per workload, or panics when told to.
type fakeRule struct {
	id    string
	sev   rules.Severity
	modes []snapshot.Mode
	panic bool
}

func (r fakeRule) ID() string               { return r.id }
func (r fakeRule) Category() string         { return "test" }
func (r fakeRule) Severity() rules.Severity { return r.sev }
func (r fakeRule) Modes() []snapshot.Mode   { return r.modes }
func (r fakeRule) Description() string      { return "test rule" }

func (r fakeRule) Evaluate(s *snapshot.Snapshot) []rules.Finding {
	if r.panic {
		panic("boom")
	}
	var out []rules.Finding
	for i := range s.Spec.Workloads {
		w := &s.Spec.Workloads[i]
		out = append(out, rules.Finding{Resource: w.Ref(), FieldPath: "spec.replicas", Message: "m"})
	}
	return out
}

func testSnapshot(mode snapshot.Mode) *snapshot.Snapshot {
	s := &snapshot.Snapshot{
		SchemaVersion: snapshot.SchemaVersion,
		Mode:          mode,
		Spec: snapshot.Spec{Workloads: []snapshot.Workload{
			{Kind: snapshot.KindDeployment, ObjectMeta: snapshot.ObjectMeta{Name: "b", Namespace: "ns"}},
			{Kind: snapshot.KindDeployment, ObjectMeta: snapshot.ObjectMeta{Name: "a", Namespace: "ns"}},
		}},
	}
	if mode == snapshot.ModeCluster {
		s.Runtime = &snapshot.Runtime{}
	}
	return s
}

func newEngine(t *testing.T, cfg *config.Config, rs ...rules.Rule) *rules.Engine {
	t.Helper()
	reg := rules.NewRegistry()
	if err := reg.Register(rs...); err != nil {
		t.Fatal(err)
	}
	e, err := rules.NewEngine(reg, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestRunIsDeterministic(t *testing.T) {
	e := newEngine(t, nil,
		fakeRule{id: "low-rule", sev: rules.SeverityLow, modes: rules.AllModes},
		fakeRule{id: "high-rule", sev: rules.SeverityHigh, modes: rules.AllModes},
	)
	first := e.Run(testSnapshot(snapshot.ModeManifest))
	second := e.Run(testSnapshot(snapshot.ModeManifest))
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("results differ between runs:\n%+v\n%+v", first, second)
	}
	if len(first.Findings) != 4 {
		t.Fatalf("got %d findings, want 4", len(first.Findings))
	}
	got := []string{}
	for _, f := range first.Findings {
		got = append(got, f.RuleID+":"+f.Resource.Name)
	}
	want := []string{"high-rule:a", "high-rule:b", "low-rule:a", "low-rule:b"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
	for _, f := range first.Findings {
		if f.ID != rules.FindingID(f.RuleID, f.Resource, f.FieldPath) {
			t.Errorf("finding %s has ID %s, want the stable ID", f.RuleID, f.ID)
		}
	}
}

func TestFindingIDIsStableAndDistinct(t *testing.T) {
	ref := snapshot.ResourceRef{Kind: "Deployment", Namespace: "ns", Name: "api"}
	id := rules.FindingID("rule", ref, "spec.replicas")
	if id != rules.FindingID("rule", ref, "spec.replicas") {
		t.Fatal("same inputs gave different IDs")
	}
	if !strings.HasPrefix(id, "KD-") || len(id) != 19 {
		t.Errorf("unexpected ID format %q", id)
	}
	others := []string{
		rules.FindingID("other", ref, "spec.replicas"),
		rules.FindingID("rule", snapshot.ResourceRef{Kind: "Deployment", Namespace: "ns", Name: "web"}, "spec.replicas"),
		rules.FindingID("rule", ref, "spec.template"),
		// Field separators stop "a"+"bc" from colliding with "ab"+"c".
		rules.FindingID("rul", snapshot.ResourceRef{Kind: "eDeployment", Namespace: "ns", Name: "api"}, "spec.replicas"),
	}
	for _, o := range others {
		if o == id {
			t.Errorf("different inputs collided on %s", id)
		}
	}
}

func TestModeMismatchIsReportedNotRun(t *testing.T) {
	e := newEngine(t, nil, fakeRule{id: "needs-cluster", sev: rules.SeverityHigh, modes: rules.ClusterOnly})

	res := e.Run(testSnapshot(snapshot.ModeManifest))
	if len(res.Findings) != 0 || len(res.RulesRun) != 0 {
		t.Fatalf("cluster-only rule ran on a manifest snapshot: %+v", res)
	}
	if len(res.NotRun) != 1 || !strings.Contains(res.NotRun[0].Reason, "requires cluster mode") {
		t.Fatalf("NotRun = %+v", res.NotRun)
	}

	res = e.Run(testSnapshot(snapshot.ModeCluster))
	if len(res.Findings) != 2 || len(res.NotRun) != 0 {
		t.Fatalf("cluster-only rule did not run on a cluster snapshot: %+v", res)
	}
}

func TestConfigDisablesAndOverrides(t *testing.T) {
	off := false
	cfg := &config.Config{Rules: map[string]config.RuleConfig{
		"disabled": {Enabled: &off},
		"bumped":   {Severity: "critical"},
	}}
	e := newEngine(t, cfg,
		fakeRule{id: "disabled", sev: rules.SeverityHigh, modes: rules.AllModes},
		fakeRule{id: "bumped", sev: rules.SeverityLow, modes: rules.AllModes},
	)
	res := e.Run(testSnapshot(snapshot.ModeManifest))
	if len(res.NotRun) != 1 || res.NotRun[0].RuleID != "disabled" {
		t.Fatalf("NotRun = %+v", res.NotRun)
	}
	for _, f := range res.Findings {
		if f.RuleID != "bumped" || f.Severity != rules.SeverityCritical {
			t.Errorf("finding %s has severity %s, want bumped/critical", f.RuleID, f.Severity)
		}
	}
	// A severity override must not change finding IDs.
	plain := newEngine(t, nil, fakeRule{id: "bumped", sev: rules.SeverityLow, modes: rules.AllModes})
	if got, want := res.Findings[0].ID, plain.Run(testSnapshot(snapshot.ModeManifest)).Findings[0].ID; got != want {
		t.Errorf("override changed ID: %s vs %s", got, want)
	}
}

func TestNewEngineRejectsBadConfig(t *testing.T) {
	reg := rules.NewRegistry().MustRegister(fakeRule{id: "known", sev: rules.SeverityLow, modes: rules.AllModes})
	for name, cfg := range map[string]*config.Config{
		"unknown rule":     {Rules: map[string]config.RuleConfig{"nope": {}}},
		"invalid severity": {Rules: map[string]config.RuleConfig{"known": {Severity: "urgent"}}},
	} {
		if _, err := rules.NewEngine(reg, cfg); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestPanickingRuleIsIsolated(t *testing.T) {
	e := newEngine(t, nil,
		fakeRule{id: "broken", sev: rules.SeverityHigh, modes: rules.AllModes, panic: true},
		fakeRule{id: "fine", sev: rules.SeverityLow, modes: rules.AllModes},
	)
	res := e.Run(testSnapshot(snapshot.ModeManifest))
	if len(res.Findings) != 2 {
		t.Errorf("got %d findings from the healthy rule, want 2", len(res.Findings))
	}
	if len(res.NotRun) != 1 || res.NotRun[0].RuleID != "broken" || !strings.Contains(res.NotRun[0].Reason, "boom") {
		t.Errorf("NotRun = %+v", res.NotRun)
	}
}

func TestRegistryValidation(t *testing.T) {
	cases := map[string]rules.Rule{
		"not kebab-case": fakeRule{id: "Bad_ID", sev: rules.SeverityLow, modes: rules.AllModes},
		"no severity":    fakeRule{id: "no-sev", modes: rules.AllModes},
		"no modes":       fakeRule{id: "no-modes", sev: rules.SeverityLow},
	}
	for name, r := range cases {
		if err := rules.NewRegistry().Register(r); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	reg := rules.NewRegistry().MustRegister(fakeRule{id: "dup", sev: rules.SeverityLow, modes: rules.AllModes})
	if err := reg.Register(fakeRule{id: "dup", sev: rules.SeverityLow, modes: rules.AllModes}); err == nil {
		t.Error("duplicate ID: expected an error")
	}
}

func TestSeverityRoundTrip(t *testing.T) {
	for _, s := range rules.Severities() {
		b, err := s.MarshalJSON()
		if err != nil {
			t.Fatal(err)
		}
		var got rules.Severity
		if err := got.UnmarshalJSON(b); err != nil {
			t.Fatal(err)
		}
		if got != s {
			t.Errorf("round trip of %s gave %s", s, got)
		}
	}
	if s, err := rules.ParseSeverity(" HIGH "); err != nil || s != rules.SeverityHigh {
		t.Errorf("ParseSeverity(HIGH) = %v, %v", s, err)
	}
	if _, err := rules.ParseSeverity("urgent"); err == nil {
		t.Error("ParseSeverity(urgent): expected an error")
	}
	if _, err := rules.Severity(0).MarshalJSON(); err == nil {
		t.Error("marshaling the zero severity should fail")
	}
}
