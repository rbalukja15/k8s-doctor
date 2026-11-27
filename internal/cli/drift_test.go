package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rbalukja15/k8s-doctor/internal/cli"
	"github.com/rbalukja15/k8s-doctor/internal/report"
)

const overlays = "../../fixtures/e2e/gitops/deployables/webshop/overlays/"

func TestDriftCommand(t *testing.T) {
	// The prod-only HPA is a medium drift-resource-missing finding.
	code, out, errOut := run("drift", "--base", overlays+"staging", "--target", overlays+"prod/tenant/acme", "--fail-on", "medium")
	if code != cli.ExitFindingMedium {
		t.Fatalf("code %d, stderr %q, stdout:\n%s", code, errOut, out)
	}
	for _, want := range []string{"drift", "staging", "acme", "drift-resource-missing", "HorizontalPodAutoscaler"} {
		if !strings.Contains(out, want) {
			t.Errorf("text output lacks %q:\n%s", want, out)
		}
	}
	// --fail-on defaults to medium for drift, so the missing HPA fails the
	// run; --fail-on high lets it pass.
	if code, _, _ := run("drift", "--base", overlays+"staging", "--target", overlays+"prod/tenant/acme"); code != cli.ExitFindingMedium {
		t.Errorf("default --fail-on: code %d, want %d", code, cli.ExitFindingMedium)
	}
	if code, _, _ := run("drift", "--base", overlays+"staging", "--target", overlays+"prod/tenant/acme", "--fail-on", "high"); code != cli.ExitOK {
		t.Errorf("--fail-on high: code %d, want 0", code)
	}
	// Two tenants: clean.
	code, out, _ = run("drift", "--base", overlays+"prod/tenant/acme", "--target", overlays+"prod/tenant/globex", "--format", "json")
	if code != cli.ExitOK {
		t.Fatalf("tenants: code %d", code)
	}
	var r report.Report
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatal(err)
	}
	if r.Drift == nil || r.Drift.Base.Overlay != "acme" || r.Drift.Target.Overlay != "globex" || r.Summary.Total != 0 {
		t.Errorf("tenant report: drift %+v, summary %+v", r.Drift, r.Summary)
	}
}

func TestDriftConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(cfg, []byte("drift:\n  presets: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, _ := run("drift", "--base", overlays+"staging", "--target", overlays+"prod/tenant/acme",
		"--config", cfg, "--fail-on", "low", "--format", "json")
	var r report.Report
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatal(err)
	}
	if code != cli.ExitFindingMedium || r.Summary.BySeverity["low"] == 0 {
		t.Errorf("with presets off: code %d, summary %+v; want the image and resource differences too", code, r.Summary)
	}
	// Per-rule settings do not apply to drift; naming a drift rule there is
	// an error rather than a setting that silently does nothing.
	if err := os.WriteFile(cfg, []byte("rules:\n  drift-field:\n    enabled: false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, errOut := run("drift", "--base", overlays+"staging", "--target", overlays+"prod/tenant/acme", "--config", cfg); code != cli.ExitError ||
		!strings.Contains(errOut, "drift.allowlist") {
		t.Errorf("drift rule in rules section: code %d, stderr %q", code, errOut)
	}
	if err := os.WriteFile(cfg, []byte("drift:\n  presets: [replica]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, errOut := run("drift", "--base", overlays+"staging", "--target", overlays+"prod/tenant/acme", "--config", cfg); code != cli.ExitError ||
		!strings.Contains(errOut, "unknown preset") {
		t.Errorf("unknown preset: code %d, stderr %q", code, errOut)
	}
}

func TestDriftUsage(t *testing.T) {
	for _, args := range [][]string{
		{"drift"},
		{"drift", "--base", overlays + "staging"},
		{"drift", "--base", overlays + "staging", "--target", overlays + "prod/tenant/acme", "--format", "yaml"},
		{"drift", "--base", overlays + "staging", "--target", overlays + "prod/tenant/acme", "extra"},
	} {
		if code, _, _ := run(args...); code != cli.ExitUsage {
			t.Errorf("%v: code %d, want usage error", args, code)
		}
	}
	if code, _, errOut := run("drift", "--base", overlays+"staging", "--target", overlays+"staging/"); code != cli.ExitUsage ||
		!strings.Contains(errOut, "same directory") {
		t.Errorf("same directory: code %d, stderr %q", code, errOut)
	}
	abs, err := filepath.Abs(overlays + "staging")
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "current")
	if err := os.Symlink(abs, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if code, _, errOut := run("drift", "--base", overlays+"staging", "--target", link); code != cli.ExitUsage ||
		!strings.Contains(errOut, "same directory") {
		t.Errorf("symlink to the same directory: code %d, stderr %q", code, errOut)
	}
	if code, _, errOut := run("drift", "--base", "nope", "--target", overlays+"staging"); code != cli.ExitError || errOut == "" {
		t.Errorf("missing base: code %d, stderr %q", code, errOut)
	}
}
