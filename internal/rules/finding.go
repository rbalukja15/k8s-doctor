package rules

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/rbalukja15/k8s-doctor/internal/snapshot"
)

// Finding is one problem a rule found on one field of one resource.
type Finding struct {
	// ID is stable across runs: the same rule, resource and field path always
	// produce the same ID. Explanations (#16), correlation groups (#17) and
	// the eval harness (#21) all key on it.
	ID       string               `json:"id"`
	RuleID   string               `json:"ruleId"`
	Severity Severity             `json:"severity"`
	Resource snapshot.ResourceRef `json:"resource"`
	// FieldPath points into the resource as written in its manifest, e.g.
	// spec.template.spec.containers[name=api].readinessProbe. List items are
	// addressed by name, never by index, so the path survives reordering.
	FieldPath string `json:"fieldPath"`
	// Message is a one-line, human-readable statement of the problem.
	Message string `json:"message"`
	// Evidence holds the observed values that triggered the rule.
	Evidence map[string]string `json:"evidence,omitempty"`
}

// FindingID computes the stable ID for a rule, resource and field path.
// Severity and message are deliberately excluded so that config overrides
// and wording changes do not change IDs.
func FindingID(ruleID string, ref snapshot.ResourceRef, fieldPath string) string {
	h := sha256.New()
	for _, part := range []string{ruleID, ref.String(), fieldPath} {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return "KD-" + hex.EncodeToString(h.Sum(nil))[:16]
}

// Key is the identity used to compare actual and expected findings in
// fixtures and evals: rule ID, resource and field path.
type Key struct {
	RuleID    string
	Resource  string
	FieldPath string
}

// Key returns the comparison key for f.
func (f *Finding) Key() Key {
	return Key{RuleID: f.RuleID, Resource: f.Resource.String(), FieldPath: f.FieldPath}
}
