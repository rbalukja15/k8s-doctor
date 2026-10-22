package rules

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/rbalukja15/k8s-doctor/internal/config"
	"github.com/rbalukja15/k8s-doctor/internal/snapshot"
)

// NotRun records a rule that was skipped, so the report can say so instead
// of implying the rule ran and found nothing.
type NotRun struct {
	RuleID string `json:"ruleId"`
	Reason string `json:"reason"`
}

// Result is the outcome of running the engine on one snapshot.
type Result struct {
	// Findings are sorted by severity (most severe first), then rule ID,
	// resource and field path.
	Findings []Finding `json:"findings"`
	RulesRun []string  `json:"rulesRun"`
	NotRun   []NotRun  `json:"notRun,omitempty"`
}

// Engine runs registered rules under a config.
type Engine struct {
	reg       *Registry
	cfg       *config.Config
	overrides map[string]Severity
}

// NewEngine validates cfg against the registry and returns an engine.
// Unknown rule IDs and invalid severities in the config are errors.
func NewEngine(reg *Registry, cfg *config.Config) (*Engine, error) {
	if cfg == nil {
		cfg = &config.Config{}
	}
	e := &Engine{reg: reg, cfg: cfg, overrides: map[string]Severity{}}
	var errs []error
	for id, rc := range cfg.Rules {
		if _, ok := reg.Get(id); !ok {
			errs = append(errs, fmt.Errorf("config: unknown rule %q", id))
			continue
		}
		if rc.Severity != "" {
			sev, err := ParseSeverity(rc.Severity)
			if err != nil {
				errs = append(errs, fmt.Errorf("config: rule %q: %w", id, err))
				continue
			}
			e.overrides[id] = sev
		}
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return e, nil
}

// Severity returns the effective severity of a rule: the config override
// if there is one, otherwise the rule's default.
func (e *Engine) Severity(r Rule) Severity {
	if o, ok := e.overrides[r.ID()]; ok {
		return o
	}
	return r.Severity()
}

// Run evaluates every applicable rule against s. Running twice on the same
// snapshot yields identical results, including finding IDs and order.
func (e *Engine) Run(s *snapshot.Snapshot) Result {
	res := Result{Findings: []Finding{}, RulesRun: []string{}}
	seen := map[string]bool{}
	for _, rule := range e.reg.All() {
		id := rule.ID()
		if !e.cfg.RuleEnabled(id) {
			res.NotRun = append(res.NotRun, NotRun{RuleID: id, Reason: "disabled in config"})
			continue
		}
		if !slices.Contains(rule.Modes(), s.Mode) {
			res.NotRun = append(res.NotRun, NotRun{
				RuleID: id,
				Reason: fmt.Sprintf("requires %s mode; snapshot is %s mode", modeList(rule.Modes()), s.Mode),
			})
			continue
		}
		findings, err := evaluate(rule, s)
		if err != nil {
			res.NotRun = append(res.NotRun, NotRun{RuleID: id, Reason: err.Error()})
			continue
		}
		res.RulesRun = append(res.RulesRun, id)
		sev := e.Severity(rule)
		for _, f := range findings {
			f.RuleID = id
			f.Severity = sev
			f.ID = FindingID(id, f.Resource, f.FieldPath)
			if seen[f.ID] {
				continue
			}
			seen[f.ID] = true
			res.Findings = append(res.Findings, f)
		}
	}
	SortFindings(res.Findings)
	return res
}

// evaluate runs one rule and turns a panic into an error, so one broken
// rule cannot take down the whole report.
func evaluate(rule Rule, s *snapshot.Snapshot) (fs []Finding, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("rule failed: %v", r)
		}
	}()
	return rule.Evaluate(s), nil
}

// SortFindings orders findings by severity (most severe first), then by
// rule ID, resource and field path.
func SortFindings(fs []Finding) {
	sort.SliceStable(fs, func(i, j int) bool {
		a, b := fs[i], fs[j]
		if a.Severity != b.Severity {
			return a.Severity > b.Severity
		}
		if a.RuleID != b.RuleID {
			return a.RuleID < b.RuleID
		}
		if ar, br := a.Resource.String(), b.Resource.String(); ar != br {
			return ar < br
		}
		return a.FieldPath < b.FieldPath
	})
}

func modeList(ms []snapshot.Mode) string {
	parts := make([]string, len(ms))
	for i, m := range ms {
		parts[i] = string(m)
	}
	return strings.Join(parts, " or ")
}
