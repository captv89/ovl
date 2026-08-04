// SPDX-License-Identifier: AGPL-3.0-only

package validation

import "time"

// Report is the minimal, schema-driven view of a report version that the
// rule engine needs. It intentionally does not depend on a persistence
// layer or the full report aggregate (pkg/domain, not yet built) so the
// engine can run identically, and be unit tested, on both vessel and
// office from plain data.
type Report struct {
	ReportID   string
	VersionNo  int
	SchemaName string    // matches schema.Schema.SchemaName, e.g. "log-abstract"
	EventType  string    // OVD event type, e.g. "Departure"
	EventTime  time.Time // UTC; combined Date_UTC + Time_UTC

	// Fields holds the schema-driven field values, keyed by field name.
	// Values are decoded JSON-shaped: float64 for decimal/wholeNumber,
	// string for text/enum/date/time/dateTime, bool for boolean. A field
	// absent from the map means it was left empty on the report.
	Fields map[string]any
}

// Float returns the field as a float64, if present and numeric.
func (r *Report) Float(name string) (float64, bool) {
	v, ok := r.Fields[name]
	if !ok {
		return 0, false
	}
	f, ok := v.(float64)
	return f, ok
}

// String returns the field as a string, if present and a string.
func (r *Report) String(name string) (string, bool) {
	v, ok := r.Fields[name]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// Bool returns the field as a bool, if present and a bool.
func (r *Report) Bool(name string) (bool, bool) {
	v, ok := r.Fields[name]
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

// IsEmpty reports whether the field is absent, nil, an empty string, or
// (for numeric fields) not applicable — used by mandatoriness rules.
func (r *Report) IsEmpty(name string) bool {
	v, ok := r.Fields[name]
	if !ok || v == nil {
		return true
	}
	if s, ok := v.(string); ok {
		return s == ""
	}
	return false
}
