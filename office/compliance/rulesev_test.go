// SPDX-License-Identifier: AGPL-3.0-only

package compliance

import (
	"errors"
	"testing"

	"github.com/captv89/ovl/pkg/validation"
)

func TestRuleSeverityAssignment_SetSeverity(t *testing.T) {
	tests := []struct {
		name     string
		ruleID   string
		severity validation.Severity
		wantErr  error
	}{
		{"overridable rule ok", validation.RuleImpliedSpeed, validation.SeverityWarning, nil},
		{"error severity ok", validation.RuleTimeChain, validation.SeverityError, nil},
		{"info severity ok", validation.RuleEventOrdering, validation.SeverityInfo, nil},
		{"hard rule rejected", validation.RuleConsumptionSchemeExclusive, validation.SeverityWarning, ErrHardRuleLocked},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := NewRuleSeverityAssignment(FleetScope())
			if err != nil {
				t.Fatalf("NewRuleSeverityAssignment: %v", err)
			}
			err = a.SetSeverity(tt.ruleID, tt.severity)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("SetSeverity() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && a.Severities[tt.ruleID] != tt.severity {
				t.Errorf("Severities[%q] = %q, want %q", tt.ruleID, a.Severities[tt.ruleID], tt.severity)
			}
		})
	}
}

func TestRuleSeverityAssignment_SetSeverity_UnknownRuleOrSeverity(t *testing.T) {
	a, err := NewRuleSeverityAssignment(FleetScope())
	if err != nil {
		t.Fatalf("NewRuleSeverityAssignment: %v", err)
	}
	if err := a.SetSeverity("bogus.rule", validation.SeverityError); err == nil {
		t.Error("SetSeverity(unknown rule): got nil error, want an error")
	}
	if err := a.SetSeverity(validation.RuleImpliedSpeed, validation.Severity("bogus")); err == nil {
		t.Error("SetSeverity(unknown severity): got nil error, want an error")
	}
}

func TestEffectiveSeverities(t *testing.T) {
	groupScope, _ := GroupScope("Fleet A")
	otherGroupScope, _ := GroupScope("Pacific")
	vesselScope, _ := VesselScope("vessel-1")

	fleet, _ := NewRuleSeverityAssignment(FleetScope())
	_ = fleet.SetSeverity(validation.RuleImpliedSpeed, validation.SeverityWarning)
	_ = fleet.SetSeverity(validation.RuleTimeChain, validation.SeverityInfo)

	strictGroup, _ := NewRuleSeverityAssignment(groupScope)
	_ = strictGroup.SetSeverity(validation.RuleTimeChain, validation.SeverityError)

	looseGroup, _ := NewRuleSeverityAssignment(otherGroupScope)
	_ = looseGroup.SetSeverity(validation.RuleTimeChain, validation.SeverityWarning)

	vessel, _ := NewRuleSeverityAssignment(vesselScope)
	_ = vessel.SetSeverity(validation.RuleImpliedSpeed, validation.SeverityError)

	assignments := []*RuleSeverityAssignment{fleet, strictGroup, looseGroup, vessel}

	got := EffectiveSeverities(assignments, "vessel-1", []string{"Fleet A", "Pacific"})

	if got[validation.RuleImpliedSpeed] != validation.SeverityError {
		t.Errorf("RuleImpliedSpeed = %q, want error (vessel override beats fleet)", got[validation.RuleImpliedSpeed])
	}
	if got[validation.RuleTimeChain] != validation.SeverityError {
		t.Errorf("RuleTimeChain = %q, want error (strictest covering group beats fleet)", got[validation.RuleTimeChain])
	}
	if _, ok := got[validation.RuleEventOrdering]; ok {
		t.Errorf("RuleEventOrdering present in result %v, want absent (no assignment overrides it, rule's own default should apply)", got)
	}
}
