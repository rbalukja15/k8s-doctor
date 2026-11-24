package drift

import "github.com/rbalukja15/k8s-doctor/internal/rules"

// Category is the report category of drift findings.
const Category = "drift"

// RuleInfo describes a drift finding type for reports. Drift compares two
// snapshots, so its checks are not rules.Rule values, which see one.
type RuleInfo struct {
	ID          string
	Severity    rules.Severity
	Description string
}

// Rules lists the drift finding types. drift-field findings carry their
// own severity per field; Severity here is the most common one.
func Rules() []RuleInfo {
	return []RuleInfo{
		{
			ID:       RuleResourceMissing,
			Severity: rules.SeverityMedium,
			Description: "The object exists in one overlay but not the other. If both overlays deploy the same " +
				"application, one of them is missing a piece, such as an HPA, PDB or NetworkPolicy, that the other " +
				"relies on. Add it, or remove it from the other overlay if it is no longer needed.",
		},
		{
			ID:       RuleField,
			Severity: rules.SeverityLow,
			Description: "The field differs between two overlays of the same application and the difference is not " +
				"in the drift allowlist. Security settings rank high and config keys medium. If the difference is " +
				"intended, add the field path to drift.allowlist in .k8s-doctor.yaml.",
		},
	}
}
