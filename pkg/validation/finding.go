// SPDX-License-Identifier: AGPL-3.0-only

package validation

// Severity is the effect a rule violation has on the report (architecture
// 10.2). Severity per rule is normally part of the config bundle, so
// companies can tighten or relax within limits — except schema-mandatory
// and hard OVD rules, which are always SeverityError.
type Severity string

const (
	SeverityError   Severity = "error"   // blocks submit
	SeverityWarning Severity = "warning" // shown in health check, submit allowed
	SeverityInfo    Severity = "info"    // advisory only
)

// Finding is a single rule violation against a report.
type Finding struct {
	RuleID   string
	Severity Severity
	Field    string // field name, if the finding is field-specific; "" otherwise
	Message  string
}

// Findings is a list of Finding with small query helpers used by the
// health check (architecture 9.4): errors block submit, warnings and info
// do not.
type Findings []Finding

// HasErrors reports whether any finding is SeverityError.
func (fs Findings) HasErrors() bool {
	for _, f := range fs {
		if f.Severity == SeverityError {
			return true
		}
	}
	return false
}

// ByRule returns the findings produced by the given rule ID, in order.
func (fs Findings) ByRule(ruleID string) Findings {
	var out Findings
	for _, f := range fs {
		if f.RuleID == ruleID {
			out = append(out, f)
		}
	}
	return out
}
