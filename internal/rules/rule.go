// Package rules is the deterministic rule engine. Rules find problems; the
// LLM layer only ever explains findings produced here.
package rules

import (
	"fmt"
	"regexp"
	"slices"
	"sort"

	"github.com/rbalukja15/k8s-doctor/internal/snapshot"
)

// Rule is a deterministic check over a snapshot.
type Rule interface {
	// ID is a stable kebab-case identifier, e.g. "probe-readiness-missing".
	ID() string
	// Category groups rules for reporting and eval, e.g. "reliability".
	Category() string
	// Severity is the default severity; config may override it.
	Severity() Severity
	// Modes lists the snapshot modes the rule can run on. A rule that needs
	// runtime data returns only snapshot.ModeCluster.
	Modes() []snapshot.Mode
	// Description is a static explanation used when no LLM is available.
	Description() string
	// Evaluate returns the rule's findings. Rules fill Resource, FieldPath,
	// Message and Evidence; the engine sets ID, RuleID and Severity.
	Evaluate(s *snapshot.Snapshot) []Finding
}

// Mode sets for Rule.Modes.
var (
	AllModes    = []snapshot.Mode{snapshot.ModeManifest, snapshot.ModeCluster}
	ClusterOnly = []snapshot.Mode{snapshot.ModeCluster}
)

var ruleIDPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Registry holds the set of known rules.
type Registry struct {
	rules map[string]Rule
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{rules: map[string]Rule{}}
}

// Register adds rules to the registry. It rejects malformed or duplicate IDs,
// invalid default severities and empty mode lists.
func (r *Registry) Register(rs ...Rule) error {
	for _, rule := range rs {
		id := rule.ID()
		if !ruleIDPattern.MatchString(id) {
			return fmt.Errorf("rule id %q is not kebab-case", id)
		}
		if _, dup := r.rules[id]; dup {
			return fmt.Errorf("rule %q registered twice", id)
		}
		if !rule.Severity().Valid() {
			return fmt.Errorf("rule %q has invalid default severity", id)
		}
		if len(rule.Modes()) == 0 {
			return fmt.Errorf("rule %q declares no modes", id)
		}
		r.rules[id] = rule
	}
	return nil
}

// MustRegister is Register that panics on error, for static rule sets.
func (r *Registry) MustRegister(rs ...Rule) *Registry {
	if err := r.Register(rs...); err != nil {
		panic(err)
	}
	return r
}

// Get returns the rule with the given ID.
func (r *Registry) Get(id string) (Rule, bool) {
	rule, ok := r.rules[id]
	return rule, ok
}

// All returns every rule, sorted by ID.
func (r *Registry) All() []Rule {
	out := make([]Rule, 0, len(r.rules))
	for _, rule := range r.rules {
		out = append(out, rule)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out
}

// IDs returns every rule ID, sorted.
func (r *Registry) IDs() []string {
	ids := make([]string, 0, len(r.rules))
	for id := range r.rules {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}
