package rules

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Severity ranks how urgently a finding should be fixed.
type Severity int

// Severities from least to most severe. The zero value is invalid so a
// rule that forgets to set one is caught by tests.
const (
	SeverityLow Severity = iota + 1
	SeverityMedium
	SeverityHigh
	SeverityCritical
)

var severityNames = map[Severity]string{
	SeverityLow:      "low",
	SeverityMedium:   "medium",
	SeverityHigh:     "high",
	SeverityCritical: "critical",
}

// Severities returns every valid severity, most severe first.
func Severities() []Severity {
	return []Severity{SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow}
}

func (s Severity) String() string {
	if n, ok := severityNames[s]; ok {
		return n
	}
	return fmt.Sprintf("Severity(%d)", int(s))
}

// Valid reports whether s is one of the defined severities.
func (s Severity) Valid() bool { _, ok := severityNames[s]; return ok }

// ParseSeverity parses a severity name, case-insensitively.
func ParseSeverity(name string) (Severity, error) {
	n := strings.ToLower(strings.TrimSpace(name))
	for s, sn := range severityNames {
		if sn == n {
			return s, nil
		}
	}
	return 0, fmt.Errorf("unknown severity %q (want critical, high, medium or low)", name)
}

// MarshalJSON encodes the severity as its name.
func (s Severity) MarshalJSON() ([]byte, error) {
	if !s.Valid() {
		return nil, fmt.Errorf("cannot marshal invalid severity %d", int(s))
	}
	return json.Marshal(s.String())
}

// UnmarshalJSON decodes a severity name.
func (s *Severity) UnmarshalJSON(b []byte) error {
	var name string
	if err := json.Unmarshal(b, &name); err != nil {
		return err
	}
	v, err := ParseSeverity(name)
	if err != nil {
		return err
	}
	*s = v
	return nil
}
