package report_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/rbalukja15/k8s-doctor/internal/config"
	"github.com/rbalukja15/k8s-doctor/internal/fixtures"
	"github.com/rbalukja15/k8s-doctor/internal/report"
	"github.com/rbalukja15/k8s-doctor/internal/rules"
	"github.com/rbalukja15/k8s-doctor/internal/rules/checks"
	"github.com/rbalukja15/k8s-doctor/internal/snapshot"
)

func compileSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(report.Schema))
	if err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	c.AssertFormat()
	if err := c.AddResource("report.v1.json", doc); err != nil {
		t.Fatal(err)
	}
	s, err := c.Compile("report.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func build(t *testing.T, snap *snapshot.Snapshot, cfg *config.Config) *report.Report {
	t.Helper()
	reg := checks.Registry()
	engine, err := rules.NewEngine(reg, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return report.Build(snap, reg, engine, engine.Run(snap), report.Options{
		Version: "test",
		Now:     time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC),
	})
}

// TestReportsValidateAgainstSchema renders a report for every fixture and
// validates it against the published JSON Schema.
func TestReportsValidateAgainstSchema(t *testing.T) {
	schema := compileSchema(t)
	all, err := fixtures.Discover("../../fixtures")
	if err != nil {
		t.Fatal(err)
	}
	off := false
	disabled := &config.Config{Rules: map[string]config.RuleConfig{"probe-liveness-missing": {Enabled: &off}}}
	for _, f := range all {
		snap, err := f.Snapshot()
		if err != nil {
			continue
		}
		for name, cfg := range map[string]*config.Config{"default": nil, "with-not-run": disabled} {
			var buf bytes.Buffer
			if err := build(t, snap, cfg).WriteJSON(&buf); err != nil {
				t.Fatal(err)
			}
			doc, err := jsonschema.UnmarshalJSON(&buf)
			if err != nil {
				t.Fatal(err)
			}
			if err := schema.Validate(doc); err != nil {
				t.Errorf("%s (%s): report does not match schema: %v", f.Name, name, err)
			}
		}
	}
}

func TestSummary(t *testing.T) {
	snap, err := snapshot.Load("../../fixtures/image-tag-mutable/positive/snapshot.json")
	if err != nil {
		t.Fatal(err)
	}
	r := build(t, snap, nil)
	if r.Summary.Total != len(r.Findings) || r.Summary.Total == 0 {
		t.Fatalf("total %d, findings %d", r.Summary.Total, len(r.Findings))
	}
	sum := 0
	for _, n := range r.Summary.BySeverity {
		sum += n
	}
	if sum != r.Summary.Total {
		t.Errorf("bySeverity sums to %d, want %d", sum, r.Summary.Total)
	}
	worst, ok := r.WorstSeverity()
	if !ok || worst != r.Findings[0].Severity {
		t.Errorf("worst = %v, %v; first finding is %v", worst, ok, r.Findings[0].Severity)
	}
	for _, f := range r.Findings {
		if f.Explanation.Source != report.SourceRule || f.Explanation.Text == "" || f.Category == "" {
			t.Errorf("finding %s lacks category or rule explanation", f.ID)
		}
	}
}

func TestCleanReport(t *testing.T) {
	snap, err := snapshot.Load("../../fixtures/clean/well-configured/snapshot.json")
	if err != nil {
		t.Fatal(err)
	}
	r := build(t, snap, nil)
	if _, ok := r.WorstSeverity(); ok || r.Summary.Worst != "" {
		t.Errorf("clean report has a worst severity: %q", r.Summary.Worst)
	}
	var buf bytes.Buffer
	if err := r.WriteText(&buf, report.TextOptions{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "No findings.") || strings.Contains(buf.String(), "\033[") {
		t.Errorf("unexpected text output:\n%s", buf.String())
	}
}

func TestJSONIsDeterministic(t *testing.T) {
	snap, err := snapshot.Load("../../fixtures/resources-requests-missing/positive/snapshot.json")
	if err != nil {
		t.Fatal(err)
	}
	render := func() string {
		var buf bytes.Buffer
		if err := build(t, snap, nil).WriteJSON(&buf); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}
	if a, b := render(), render(); a != b {
		t.Fatal("JSON report differs between identical runs")
	}
	var decoded report.Report
	if err := json.Unmarshal([]byte(render()), &decoded); err != nil {
		t.Fatalf("report does not round-trip: %v", err)
	}
}

func TestTextShowsNotRunAndColor(t *testing.T) {
	snap, err := snapshot.Load("../../fixtures/probe-liveness-missing/positive/snapshot.json")
	if err != nil {
		t.Fatal(err)
	}
	off := false
	r := build(t, snap, &config.Config{Rules: map[string]config.RuleConfig{"image-tag-mutable": {Enabled: &off}}})
	var buf bytes.Buffer
	if err := r.WriteText(&buf, report.TextOptions{Color: true, Explain: true}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"Not run:", "image-tag-mutable", "disabled in config", "LOW", "\033[", "Kubernetes never restarts it"} {
		if !strings.Contains(out, want) {
			t.Errorf("text output is missing %q:\n%s", want, out)
		}
	}
}

func TestSchemaRejectsBadReports(t *testing.T) {
	schema := compileSchema(t)
	bad := map[string]string{
		"missing summary":        `{"schemaVersion":"v1","tool":{"name":"k8s-doctor","version":"x"},"generatedAt":"2026-10-22T12:00:00Z","input":{"mode":"manifest","llm":false},"findings":[],"groups":[],"rules":[]}`,
		"bad finding id":         `{"schemaVersion":"v1","tool":{"name":"k8s-doctor","version":"x"},"generatedAt":"2026-10-22T12:00:00Z","input":{"mode":"manifest","llm":false},"summary":{"total":1,"bySeverity":{"critical":0,"high":1,"medium":0,"low":0},"rulesRun":1,"rulesNotRun":0},"findings":[{"id":"nope","ruleId":"r","severity":"high","resource":{"kind":"Deployment","name":"a"},"fieldPath":"spec","message":"m","category":"c","explanation":{"text":"t","source":"rule"}}],"groups":[],"rules":[]}`,
		"not-run without reason": `{"schemaVersion":"v1","tool":{"name":"k8s-doctor","version":"x"},"generatedAt":"2026-10-22T12:00:00Z","input":{"mode":"manifest","llm":false},"summary":{"total":0,"bySeverity":{"critical":0,"high":0,"medium":0,"low":0},"rulesRun":0,"rulesNotRun":1},"findings":[],"groups":[],"rules":[{"id":"r","category":"c","severity":"low","modes":["manifest"],"description":"d","status":"not-run"}]}`,
	}
	for name, raw := range bad {
		doc, err := jsonschema.UnmarshalJSON(strings.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		if err := schema.Validate(doc); err == nil {
			t.Errorf("%s: schema accepted an invalid report", name)
		}
	}
}
