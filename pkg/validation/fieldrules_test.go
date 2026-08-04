// SPDX-License-Identifier: AGPL-3.0-only

package validation

import (
	"testing"

	"github.com/captv89/ovl/pkg/schema"
)

func testSchema() *schema.Schema {
	return &schema.Schema{
		SchemaName: "log-abstract",
		Fields: []schema.Field{
			{Name: "IMO", Label: "IMO Number", Type: schema.FieldTypeWholeNumber, SchemaMandatory: true},
			{Name: "Voyage_Number", Label: "Voyage Number", Type: schema.FieldTypeText, MaxLength: new(10)},
			{Name: "Distance", Label: "Distance", Type: schema.FieldTypeDecimal},
			{Name: "Event_Date", Label: "Event Date", Type: schema.FieldTypeDate},
			{Name: "Hidden_Field", Label: "Hidden", Type: schema.FieldTypeText},
			{Name: "Recommended_Field", Label: "Recommended Field", Type: schema.FieldTypeText},
		},
	}
}

func TestEvaluateFieldRules(t *testing.T) {
	s := testSchema()
	policy := FieldPolicy{
		"Hidden_Field":      FieldHidden,
		"Recommended_Field": FieldRecommended,
		"Voyage_Number":     FieldOptional,
	}

	tests := []struct {
		name       string
		fields     map[string]any
		wantRuleID string
		wantField  string
		wantSev    Severity
		wantNone   bool
	}{
		{
			name: "fully valid report has no findings",
			fields: map[string]any{
				"IMO": 1234567.0, "Voyage_Number": "V1", "Distance": 12.5, "Event_Date": "2026-07-04",
				"Recommended_Field": "present",
			},
			wantNone: true,
		},
		{
			name:       "missing schema-mandatory field is an error",
			fields:     map[string]any{},
			wantRuleID: RuleFieldRequired,
			wantField:  "IMO",
			wantSev:    SeverityError,
		},
		{
			name:       "missing recommended field is a warning",
			fields:     map[string]any{"IMO": 1234567.0},
			wantRuleID: RuleFieldRequired,
			wantField:  "Recommended_Field",
			wantSev:    SeverityWarning,
		},
		{
			name:       "text exceeding maxLength is an error",
			fields:     map[string]any{"IMO": 1234567.0, "Voyage_Number": "way too long a voyage number"},
			wantRuleID: RuleFieldMaxLength,
			wantField:  "Voyage_Number",
			wantSev:    SeverityError,
		},
		{
			name:       "wholeNumber with a fractional value is an error",
			fields:     map[string]any{"IMO": 1234567.5},
			wantRuleID: RuleFieldFormat,
			wantField:  "IMO",
			wantSev:    SeverityError,
		},
		{
			name:       "malformed date is an error",
			fields:     map[string]any{"IMO": 1234567.0, "Event_Date": "07/04/2026"},
			wantRuleID: RuleFieldFormat,
			wantField:  "Event_Date",
			wantSev:    SeverityError,
		},
		{
			name:       "wrong Go type for a decimal field is an error",
			fields:     map[string]any{"IMO": 1234567.0, "Distance": "not a number"},
			wantRuleID: RuleFieldType,
			wantField:  "Distance",
			wantSev:    SeverityError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Report{Fields: tt.fields}
			findings := EvaluateFieldRules(r, s, policy, nil)
			if tt.wantNone {
				if len(findings) != 0 {
					t.Fatalf("findings = %+v, want none", findings)
				}
				return
			}
			matches := findings.ByRule(tt.wantRuleID)
			if len(matches) == 0 {
				t.Fatalf("no finding for rule %s in %+v", tt.wantRuleID, findings)
			}
			found := false
			for _, f := range matches {
				if f.Field == tt.wantField && f.Severity == tt.wantSev {
					found = true
				}
			}
			if !found {
				t.Errorf("no finding matching field=%s severity=%s in %+v", tt.wantField, tt.wantSev, matches)
			}
		})
	}
}

func TestEvaluateFieldRules_HiddenFieldNeverEvaluated(t *testing.T) {
	s := testSchema()
	policy := FieldPolicy{"Hidden_Field": FieldHidden}
	r := &Report{Fields: map[string]any{"IMO": 1234567.0, "Hidden_Field": 12345.0}} // wrong type, would fail if evaluated
	findings := EvaluateFieldRules(r, s, policy, nil)
	if len(findings.ByRule(RuleFieldType)) != 0 {
		t.Errorf("hidden field was evaluated: %+v", findings)
	}
}

// The reported gap this feature closes: a field the company made mandatory
// for port calls must not block, warn on, or otherwise appear in the health
// check of a Noon at sea report.
func TestEvaluateFieldRules_EventGatedFieldNeverEvaluated(t *testing.T) {
	s := testSchema()
	policy := FieldPolicy{"Voyage_Number": FieldCompanyMandatory}
	events := FieldEvents{"Voyage_Number": {"Arrival", "Departure"}}
	fields := map[string]any{"IMO": 1234567.0} // Voyage_Number left empty

	arrival := &Report{EventType: "Arrival", Fields: fields}
	if got := EvaluateFieldRules(arrival, s, policy, events).ByRule(RuleFieldRequired); len(got) == 0 {
		t.Error("Voyage_Number is companyMandatory on Arrival and empty — expected a required finding")
	}

	noon := &Report{EventType: "Noon (Position) - Sea passage", Fields: fields}
	for _, f := range EvaluateFieldRules(noon, s, policy, events) {
		if f.Field == "Voyage_Number" {
			t.Errorf("Voyage_Number does not apply to a Noon report but produced %+v", f)
		}
	}

	// Same policy with no event narrowing still blocks on both, so the gate
	// is what changed the outcome and not something else about the report.
	if got := EvaluateFieldRules(noon, s, policy, nil).ByRule(RuleFieldRequired); len(got) == 0 {
		t.Error("without an event narrowing, Voyage_Number should still be required on a Noon report")
	}
}
