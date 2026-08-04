// SPDX-License-Identifier: AGPL-3.0-only

package validation

import (
	"fmt"
	"math"
	"time"

	"github.com/captv89/ovl/pkg/schema"
)

// Field rule IDs (architecture 10.1, rule class 1).
const (
	RuleFieldRequired  = "field.required"
	RuleFieldType      = "field.type"
	RuleFieldMaxLength = "field.maxLength"
	RuleFieldFormat    = "field.format"
)

// Layouts for the string-encoded date/time field types. dateTime matches
// the "yyyy-mm-dd hh:mm" format used throughout the OVD 3.13 interface
// description.
const (
	dateLayout     = "2006-01-02"
	timeLayout     = "15:04"
	dateTimeLayout = "2006-01-02 15:04"
)

// EvaluateFieldRules generates findings from the schema and field policy
// (architecture 10.1 rule class 1): mandatoriness per field policy, and
// type/length/format conformance for whatever the report actually
// carries. Hidden fields are skipped entirely — they have no health
// check effect (architecture 6.1).
//
// events carries the company's per-event applicability lists (see
// FieldEvents); a field the policy scoped away from r.EventType is treated
// as hidden here, the same as any other hidden field. Pass nil for an
// event-agnostic policy — every field then applies to every event, which is
// the pre-2026-07-28 behavior.
//
// It does not validate enum fields against their enum value catalog
// (schemas/ovd-3.13/enums/*.json); pkg/schema does not load those yet, so
// enum fields are only checked for being present as a string.
func EvaluateFieldRules(r *Report, s *schema.Schema, policy FieldPolicy, events FieldEvents) Findings {
	var findings Findings
	for _, f := range s.Fields {
		state := policy.StateForEvent(f.Name, f.SchemaMandatory, f.Relevance, events, r.EventType)
		if state == FieldHidden {
			continue
		}
		empty := r.IsEmpty(f.Name)
		switch state {
		case FieldSchemaMandatory, FieldCompanyMandatory:
			if empty {
				findings = append(findings, Finding{
					RuleID:   RuleFieldRequired,
					Severity: SeverityError,
					Field:    f.Name,
					Message:  fmt.Sprintf("%s is required", f.Label),
				})
			}
		case FieldRecommended:
			if empty {
				findings = append(findings, Finding{
					RuleID:   RuleFieldRequired,
					Severity: SeverityWarning,
					Field:    f.Name,
					Message:  fmt.Sprintf("%s is recommended but empty", f.Label),
				})
			}
		}
		if !empty {
			findings = append(findings, checkFieldFormat(r, f)...)
		}
	}
	return findings
}

func checkFieldFormat(r *Report, f schema.Field) Findings {
	v := r.Fields[f.Name]
	var findings Findings

	typeErr := func(want string) Finding {
		return Finding{
			RuleID:   RuleFieldType,
			Severity: SeverityError,
			Field:    f.Name,
			Message:  fmt.Sprintf("%s must be %s", f.Label, want),
		}
	}

	switch f.Type {
	case schema.FieldTypeText, schema.FieldTypeEnum:
		s, ok := v.(string)
		if !ok {
			return Findings{typeErr("text")}
		}
		if f.MaxLength != nil && len(s) > *f.MaxLength {
			findings = append(findings, Finding{
				RuleID:   RuleFieldMaxLength,
				Severity: SeverityError,
				Field:    f.Name,
				Message:  fmt.Sprintf("%s exceeds maximum length of %d", f.Label, *f.MaxLength),
			})
		}
	case schema.FieldTypeWholeNumber:
		n, ok := v.(float64)
		if !ok {
			return Findings{typeErr("a whole number")}
		}
		if n != math.Trunc(n) {
			findings = append(findings, Finding{
				RuleID:   RuleFieldFormat,
				Severity: SeverityError,
				Field:    f.Name,
				Message:  fmt.Sprintf("%s must be a whole number", f.Label),
			})
		}
	case schema.FieldTypeDecimal:
		if _, ok := v.(float64); !ok {
			return Findings{typeErr("a number")}
		}
	case schema.FieldTypeBoolean:
		if _, ok := v.(bool); !ok {
			return Findings{typeErr("true or false")}
		}
	case schema.FieldTypeDate, schema.FieldTypeTime, schema.FieldTypeDateTime:
		s, ok := v.(string)
		if !ok {
			return Findings{typeErr("a date/time string")}
		}
		layout := map[schema.FieldType]string{
			schema.FieldTypeDate:     dateLayout,
			schema.FieldTypeTime:     timeLayout,
			schema.FieldTypeDateTime: dateTimeLayout,
		}[f.Type]
		if _, err := time.Parse(layout, s); err != nil {
			findings = append(findings, Finding{
				RuleID:   RuleFieldFormat,
				Severity: SeverityError,
				Field:    f.Name,
				Message:  fmt.Sprintf("%s does not match the expected format", f.Label),
			})
		}
	}
	return findings
}

// ParseEventTime combines a Date_UTC/Time_UTC field pair (schema.
// FieldTypeDate/FieldTypeTime layouts) into the single UTC instant
// architecture 8.2/8.3 uses for natural-key uniqueness and cascade chain
// ordering. ok is false if either value doesn't parse — callers should
// leave any previously-computed event time unchanged in that case rather
// than treating it as "now" or zero.
func ParseEventTime(dateUTC, timeUTC string) (t time.Time, ok bool) {
	parsed, err := time.Parse(dateTimeLayout, dateUTC+" "+timeUTC)
	if err != nil {
		return time.Time{}, false
	}
	return parsed.UTC(), true
}
