package report_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/rbalukja15/k8s-doctor/internal/collector"
	"github.com/rbalukja15/k8s-doctor/internal/config"
	"github.com/rbalukja15/k8s-doctor/internal/drift"
	"github.com/rbalukja15/k8s-doctor/internal/report"
	"github.com/rbalukja15/k8s-doctor/internal/rules"
)

const gitops = "../../fixtures/e2e/gitops/deployables/webshop/overlays/"

func driftReport(t *testing.T, base, target string) *report.Report {
	t.Helper()
	b, err := collector.Manifests(gitops + base)
	if err != nil {
		t.Fatal(err)
	}
	tg, err := collector.Manifests(gitops + target)
	if err != nil {
		t.Fatal(err)
	}
	none := []string{}
	o, err := drift.NewOptions(config.Drift{Presets: &none})
	if err != nil {
		t.Fatal(err)
	}
	fs, err := drift.Diff(b, tg, o)
	if err != nil {
		t.Fatal(err)
	}
	return report.BuildDrift(b, tg, fs, report.Options{Version: "test", Now: time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC)})
}

func TestDriftReport(t *testing.T) {
	r := driftReport(t, "staging", "prod/tenant/acme")
	if r.Drift == nil || r.Drift.Base.Overlay != "staging" || r.Drift.Target.Overlay != "acme" {
		t.Fatalf("drift section = %+v", r.Drift)
	}
	if r.Summary.Total == 0 || r.Summary.Total != len(r.Findings) {
		t.Fatalf("summary = %+v with %d findings", r.Summary, len(r.Findings))
	}
	for i, f := range r.Findings {
		if f.Category != drift.Category || f.Explanation.Source != report.SourceRule || f.Explanation.Text == "" {
			t.Errorf("finding %s: category %q, explanation %+v", f.ID, f.Category, f.Explanation)
		}
		if i > 0 && f.Severity > r.Findings[i-1].Severity {
			t.Errorf("findings are not sorted most severe first")
		}
	}
	if len(r.Rules) != len(drift.Rules()) {
		t.Errorf("rules = %+v, want the drift finding types", r.Rules)
	}

	var buf bytes.Buffer
	if err := r.WriteJSON(&buf); err != nil {
		t.Fatal(err)
	}
	doc, err := jsonschema.UnmarshalJSON(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if err := compileSchema(t).Validate(doc); err != nil {
		t.Errorf("drift report does not match schema: %v", err)
	}

	buf.Reset()
	if err := r.WriteText(&buf, report.TextOptions{}); err != nil {
		t.Fatal(err)
	}
	text := buf.String()
	for _, want := range []string{"drift", "staging", "acme", rules.SeverityLow.String(), "drift-resource-missing"} {
		if !strings.Contains(text, want) {
			t.Errorf("text output lacks %q:\n%s", want, text)
		}
	}
}

func TestDriftReportClean(t *testing.T) {
	r := driftReport(t, "prod/tenant/acme", "prod/tenant/globex")
	if r.Summary.Total != 0 || len(r.Findings) != 0 {
		t.Errorf("acme vs globex: %+v", r.Findings)
	}
}
