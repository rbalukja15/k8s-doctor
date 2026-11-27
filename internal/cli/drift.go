package cli

import (
	"errors"
	"flag"
	"fmt"
	"path/filepath"
	"time"

	"github.com/rbalukja15/k8s-doctor/internal/collector"
	"github.com/rbalukja15/k8s-doctor/internal/drift"
	"github.com/rbalukja15/k8s-doctor/internal/report"
	"github.com/rbalukja15/k8s-doctor/internal/version"
)

const driftUsage = `Usage: k8s-doctor drift --base <dir> --target <dir> [flags]

Compare two kustomize overlays of the same application, such as two
environments or two tenants, and report what differs beyond the expected
differences in the drift section of the config (see docs/drift.md).
Findings point at objects in the target.

Exit codes are the same as for check.

Flags:
`

type driftOptions struct {
	base, target string
	configPath   string
	format       string
	reportPath   string
	failOn       string
	color        string
	explain      bool
}

func driftCmd(args []string, env Env) int {
	var o driftOptions
	fs := flag.NewFlagSet("drift", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	fs.Usage = func() {
		fmt.Fprint(env.Stderr, driftUsage)
		fs.PrintDefaults()
	}
	fs.StringVar(&o.base, "base", "", "the overlay to compare against (a kustomize directory)")
	fs.StringVar(&o.target, "target", "", "the overlay to check (a kustomize directory)")
	fs.StringVar(&o.configPath, "config", "", "config file (default ./.k8s-doctor.yaml if present)")
	fs.StringVar(&o.format, "format", "text", "output format: text or json")
	fs.StringVar(&o.reportPath, "report", "", "also write the JSON report to this file")
	fs.StringVar(&o.failOn, "fail-on", "high", "lowest severity that makes the exit code non-zero: critical, high, medium, low or none")
	fs.StringVar(&o.color, "color", "auto", "colorize text output: auto, always or never (NO_COLOR is honored)")
	fs.BoolVar(&o.explain, "explain", false, "print each finding's explanation in text output")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return ExitOK
		}
		return ExitUsage
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(env.Stderr, "drift: unexpected argument %q\n", fs.Arg(0))
		return ExitUsage
	}
	code, err := runDrift(&o, env)
	if err != nil {
		fmt.Fprintln(env.Stderr, "drift:", err)
		if errors.Is(err, errUsage) {
			return ExitUsage
		}
		return ExitError
	}
	return code
}

func runDrift(o *driftOptions, env Env) (int, error) {
	if o.base == "" || o.target == "" {
		return 0, fmt.Errorf("%w: --base and --target are both required", errUsage)
	}
	if same, err := sameDir(o.base, o.target); err != nil {
		return 0, err
	} else if same {
		return 0, fmt.Errorf("%w: --base and --target are the same directory", errUsage)
	}
	failOn, err := parseFailOn(o.failOn)
	if err != nil {
		return 0, err
	}
	if o.format != "text" && o.format != "json" {
		return 0, fmt.Errorf("%w: --format must be text or json, got %q", errUsage, o.format)
	}
	color, err := useColor(o.color, env)
	if err != nil {
		return 0, err
	}
	cfg, err := loadConfig(o.configPath)
	if err != nil {
		return 0, err
	}
	for _, info := range drift.Rules() {
		if _, ok := cfg.Rules[info.ID]; ok {
			return 0, fmt.Errorf("config: rules.%s: the rules section applies to check only; use drift.allowlist to accept a drift difference", info.ID)
		}
	}
	opts, err := drift.NewOptions(cfg.Drift)
	if err != nil {
		return 0, err
	}
	base, err := collector.Manifests(o.base)
	if err != nil {
		return 0, err
	}
	target, err := collector.Manifests(o.target)
	if err != nil {
		return 0, err
	}
	findings, err := drift.Diff(base, target, opts)
	if err != nil {
		return 0, err
	}
	rep := report.BuildDrift(base, target, findings, report.Options{Version: version.Version, Now: time.Now()})
	if o.reportPath != "" {
		if err := writeReportFile(rep, o.reportPath); err != nil {
			return 0, err
		}
	}
	if o.format == "json" {
		err = rep.WriteJSON(env.Stdout)
	} else {
		err = rep.WriteText(env.Stdout, report.TextOptions{Color: color, Explain: o.explain})
	}
	if err != nil {
		return 0, err
	}
	worst, found := rep.WorstSeverity()
	if !found || failOn == 0 || worst < failOn {
		return ExitOK, nil
	}
	return ExitCodeFor(worst), nil
}

// sameDir reports whether two paths name the same directory.
func sameDir(a, b string) (bool, error) {
	aa, err := filepath.Abs(a)
	if err != nil {
		return false, err
	}
	bb, err := filepath.Abs(b)
	if err != nil {
		return false, err
	}
	return aa == bb, nil
}
