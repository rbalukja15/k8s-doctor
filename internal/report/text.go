package report

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/rbalukja15/k8s-doctor/internal/rules"
	"github.com/rbalukja15/k8s-doctor/internal/snapshot"
)

// ANSI styles. Only used when color is enabled.
const (
	reset   = "\033[0m"
	bold    = "\033[1m"
	dim     = "\033[2m"
	red     = "\033[31m"
	yellow  = "\033[33m"
	cyan    = "\033[36m"
	green   = "\033[32m"
	boldRed = "\033[1;31m"
)

var severityStyle = map[rules.Severity]string{
	rules.SeverityCritical: boldRed,
	rules.SeverityHigh:     red,
	rules.SeverityMedium:   yellow,
	rules.SeverityLow:      cyan,
}

// TextOptions controls terminal rendering.
type TextOptions struct {
	Color bool
	// Explain adds each finding's explanation under it.
	Explain bool
}

type textWriter struct {
	w     *bufio.Writer
	color bool
}

func (t *textWriter) style(code, s string) string {
	if !t.color || code == "" {
		return s
	}
	return code + s + reset
}

func (t *textWriter) printf(format string, args ...any) {
	fmt.Fprintf(t.w, format, args...)
}

// WriteText renders the report for a terminal: a header, findings from most
// to least severe, a summary line, and the rules that did not run.
func (r *Report) WriteText(w io.Writer, opts TextOptions) error {
	t := &textWriter{w: bufio.NewWriter(w), color: opts.Color}

	source := r.Input.Path
	if r.Input.Mode == snapshot.ModeCluster {
		source = r.Input.Cluster
	}
	header := fmt.Sprintf("k8s-doctor %s · %s mode", r.Tool.Version, r.Input.Mode)
	if source != "" {
		header += " · " + source
	}
	if r.Input.Overlay != "" {
		header += " (overlay " + r.Input.Overlay + ")"
	}
	t.printf("%s\n\n", t.style(bold, header))

	if len(r.Findings) == 0 {
		t.printf("%s\n", t.style(green, "No findings."))
	}

	width := 0
	for _, s := range rules.Severities() {
		width = max(width, len(s.String()))
	}
	indent := strings.Repeat(" ", width+2)
	for _, f := range r.Findings {
		label := fmt.Sprintf("%-*s", width, strings.ToUpper(f.Severity.String()))
		t.printf("%s  %s  %s\n", t.style(severityStyle[f.Severity], label), t.style(bold, f.Resource.String()), t.style(dim, f.RuleID))
		t.printf("%s%s\n", indent, f.Message)
		t.printf("%s%s\n", indent, t.style(dim, "at "+f.FieldPath))
		if opts.Explain {
			for _, line := range wrap(f.Explanation.Text, 80) {
				t.printf("%s%s\n", indent, t.style(dim, line))
			}
		}
		t.printf("\n")
	}

	t.printf("%s\n", r.summaryLine(t))

	var notRun []RuleStatus
	for _, rs := range r.Rules {
		if rs.Status == StatusNotRun {
			notRun = append(notRun, rs)
		}
	}
	if len(notRun) > 0 {
		t.printf("\n%s\n", t.style(bold, "Not run:"))
		for _, rs := range notRun {
			t.printf("  %s  %s\n", rs.ID, t.style(dim, rs.Reason))
		}
	}
	return t.w.Flush()
}

func (r *Report) summaryLine(t *textWriter) string {
	var parts []string
	for _, s := range rules.Severities() {
		if n := r.Summary.BySeverity[s.String()]; n > 0 {
			parts = append(parts, t.style(severityStyle[s], fmt.Sprintf("%d %s", n, s)))
		}
	}
	line := fmt.Sprintf("%d %s", r.Summary.Total, plural(r.Summary.Total, "finding", "findings"))
	if len(parts) > 0 {
		line += " (" + strings.Join(parts, ", ") + ")"
	}
	line += fmt.Sprintf(" · %d %s run", r.Summary.RulesRun, plural(r.Summary.RulesRun, "rule", "rules"))
	if r.Summary.RulesNotRun > 0 {
		line += fmt.Sprintf(" · %d not run", r.Summary.RulesNotRun)
	}
	return line
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// wrap breaks text on spaces into lines of at most width bytes (a single
// longer word gets its own line).
func wrap(text string, width int) []string {
	var lines []string
	var cur strings.Builder
	for _, word := range strings.Fields(text) {
		if cur.Len() > 0 && cur.Len()+1+len(word) > width {
			lines = append(lines, cur.String())
			cur.Reset()
		}
		if cur.Len() > 0 {
			cur.WriteByte(' ')
		}
		cur.WriteString(word)
	}
	if cur.Len() > 0 {
		lines = append(lines, cur.String())
	}
	return lines
}
