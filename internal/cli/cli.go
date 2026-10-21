// Package cli implements the k8s-doctor command line. The full interface
// (cluster and manifest modes, JSON output, exit codes) lands with #20.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/rbalukja15/k8s-doctor/internal/config"
	"github.com/rbalukja15/k8s-doctor/internal/rules"
	"github.com/rbalukja15/k8s-doctor/internal/rules/checks"
	"github.com/rbalukja15/k8s-doctor/internal/snapshot"
	"github.com/rbalukja15/k8s-doctor/internal/version"
)

const usage = `k8s-doctor: a read-only SRE health check for Kubernetes clusters and manifests.

Usage:
  k8s-doctor <command> [flags]

Commands:
  check     Run the rules against a snapshot file
  rules     List the built-in rules
  version   Print the version
  help      Show this help

Run "k8s-doctor <command> -h" for command flags.
`

// Exit codes.
const (
	ExitOK    = 0
	ExitError = 1
	ExitUsage = 2
)

// Run executes the CLI with args (without the program name) and returns the
// process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stdout, usage)
		return ExitOK
	}
	switch args[0] {
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return ExitOK
	case "version", "--version":
		fmt.Fprintln(stdout, "k8s-doctor", version.Version)
		return ExitOK
	case "rules":
		return listRules(stdout)
	case "check":
		return check(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n%s", args[0], usage)
		return ExitUsage
	}
}

func listRules(stdout io.Writer) int {
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSEVERITY\tCATEGORY\tMODES")
	for _, r := range checks.Registry().All() {
		modes := make([]string, len(r.Modes()))
		for i, m := range r.Modes() {
			modes[i] = string(m)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.ID(), r.Severity(), r.Category(), strings.Join(modes, ","))
	}
	if err := tw.Flush(); err != nil {
		return ExitError
	}
	return ExitOK
}

func check(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	snapPath := fs.String("snapshot", "", "path to a snapshot JSON file (required)")
	cfgPath := fs.String("config", "", "path to a config file (default: ./"+config.DefaultFileName+" if present)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return ExitOK
		}
		return ExitUsage
	}
	if *snapPath == "" {
		fmt.Fprintln(stderr, "check: --snapshot is required")
		fs.Usage()
		return ExitUsage
	}

	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		fmt.Fprintln(stderr, "check:", err)
		return ExitError
	}
	snap, err := snapshot.Load(*snapPath)
	if err != nil {
		fmt.Fprintln(stderr, "check:", err)
		return ExitError
	}
	engine, err := rules.NewEngine(checks.Registry(), cfg)
	if err != nil {
		fmt.Fprintln(stderr, "check:", err)
		return ExitError
	}
	res := engine.Run(snap)

	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SEVERITY\tRULE\tRESOURCE\tMESSAGE")
	for _, f := range res.Findings {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", f.Severity, f.RuleID, f.Resource, f.Message)
	}
	if err := tw.Flush(); err != nil {
		return ExitError
	}
	fmt.Fprintf(stdout, "\n%d finding(s), %d rule(s) run", len(res.Findings), len(res.RulesRun))
	if len(res.NotRun) > 0 {
		fmt.Fprintf(stdout, ", %d not run:", len(res.NotRun))
		for _, n := range res.NotRun {
			fmt.Fprintf(stdout, "\n  %s: %s", n.RuleID, n.Reason)
		}
	}
	fmt.Fprintln(stdout)
	return ExitOK
}

func loadConfig(path string) (*config.Config, error) {
	if path == "" {
		return config.LoadDefault(".")
	}
	return config.Load(path)
}
