// SPDX-License-Identifier: AGPL-3.0-only

package compliance

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/captv89/ovl/pkg/validation"
)

// OverridableRuleIDs are the plausibility/continuity rule IDs a company
// may retune within limits (architecture 10.2: "Severity per rule is
// part of the config bundle so companies can tighten or relax within
// limits"; design handoff B7's "table of plausibility rules with
// severity selector"). Sourced directly from pkg/validation's own rule ID
// constants rather than restating the strings here, so this list can
// never drift from what the rule engine actually evaluates.
//
// Field rules (RuleFieldRequired/RuleFieldType/RuleFieldMaxLength/
// RuleFieldFormat) are deliberately excluded: those are basic data-
// integrity checks the engine always runs at a fixed severity, not the
// "plausibility rules" B7 means — pkg/validation itself never routes
// them through Config.Severities.
var OverridableRuleIDs = []string{
	validation.RuleTimeBucketSum,
	validation.RuleImpliedSpeed,
	validation.RuleNoDistanceStationary,
	validation.RulePositionRequired,
	validation.RulePositionConsistency,
	validation.RuleTimeChain,
	validation.RuleROBContinuity,
	validation.RuleEventOrdering,
	validation.RuleTimestampUniqueness,
}

// HardRuleIDs are rule IDs architecture 10.2 calls "hard OVD rules":
// always SeverityError, never overridable by company config. Currently
// just consumption-scheme exclusivity — pkg/validation's own
// evaluateConsumptionSchemeExclusivity hardcodes SeverityError and never
// consults Config.Severities for it, so this list mirrors that, not the
// other way around.
var HardRuleIDs = []string{
	validation.RuleConsumptionSchemeExclusive,
}

// ErrHardRuleLocked is returned when a caller tries to set a severity for
// a rule in HardRuleIDs.
var ErrHardRuleLocked = errors.New("compliance: this rule's severity is locked at error and cannot be overridden")

// RuleSeverityAssignment is the set of rule severity overrides at one
// scope (design handoff B7, assignable fleet-wide, per group, or per
// vessel — same shape as ProfileAssignment/CadenceRule). A rule absent
// from Severities keeps whatever default pkg/validation's own rule
// functions already hardcode; only overrides are stored, mirroring
// office/fieldpolicy.SchemaFieldPolicy's own "absent = default" shape.
type RuleSeverityAssignment struct {
	Scope      Scope
	Severities map[string]validation.Severity
	UpdatedAt  time.Time
}

// NewRuleSeverityAssignment returns an empty RuleSeverityAssignment for
// scope — every rule starts at its pkg/validation default until
// SetSeverity overrides it.
func NewRuleSeverityAssignment(scope Scope) (*RuleSeverityAssignment, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	return &RuleSeverityAssignment{Scope: scope, Severities: map[string]validation.Severity{}}, nil
}

// SetSeverity overrides ruleID's severity. Returns ErrHardRuleLocked for
// any rule in HardRuleIDs, and an error for any ruleID not in
// OverridableRuleIDs or any severity outside the three known values.
func (a *RuleSeverityAssignment) SetSeverity(ruleID string, severity validation.Severity) error {
	if slices.Contains(HardRuleIDs, ruleID) {
		return ErrHardRuleLocked
	}
	if !slices.Contains(OverridableRuleIDs, ruleID) {
		return fmt.Errorf("compliance: unknown rule id %q", ruleID)
	}
	switch severity {
	case validation.SeverityError, validation.SeverityWarning, validation.SeverityInfo:
	default:
		return fmt.Errorf("compliance: unknown severity %q", severity)
	}
	a.Severities[ruleID] = severity
	a.UpdatedAt = time.Now().UTC()
	return nil
}

// EffectiveSeverities resolves the severity overrides that apply to one
// vessel, per rule ID, from whatever mix of fleet/group/vessel
// assignments exist. The result is meant to be merged directly into a
// pkg/validation.Config's Severities map — a rule ID absent from the
// result keeps that rule's own hardcoded default, exactly like
// Config.severity's own fallback.
//
// Resolution is per rule ID, not per whole assignment (unlike
// EffectiveCadence, which picks one winning CadenceRule wholesale): a
// vessel can reasonably have an explicit override for rule A while
// inheriting rule B's override from a fleet-wide assignment, so each
// rule ID is resolved independently using the same precedence
// EffectiveCadence uses (vessel wins outright; otherwise the strictest
// covering group assignment; otherwise fleet-wide).
func EffectiveSeverities(assignments []*RuleSeverityAssignment, vesselID string, vesselGroups []string) map[string]validation.Severity {
	var vesselAssignment, fleetAssignment *RuleSeverityAssignment
	var groupAssignments []*RuleSeverityAssignment
	for _, a := range assignments {
		switch a.Scope.Type {
		case ScopeVessel:
			if a.Scope.Key == vesselID {
				vesselAssignment = a
			}
		case ScopeFleet:
			fleetAssignment = a
		case ScopeGroup:
			if a.Scope.CoversVessel(vesselID, vesselGroups) {
				groupAssignments = append(groupAssignments, a)
			}
		}
	}

	result := map[string]validation.Severity{}
	for _, ruleID := range OverridableRuleIDs {
		if vesselAssignment != nil {
			if sev, ok := vesselAssignment.Severities[ruleID]; ok {
				result[ruleID] = sev
				continue
			}
		}
		if sev, ok := strictestGroupSeverity(groupAssignments, ruleID); ok {
			result[ruleID] = sev
			continue
		}
		if fleetAssignment != nil {
			if sev, ok := fleetAssignment.Severities[ruleID]; ok {
				result[ruleID] = sev
			}
		}
	}
	return result
}

// strictestGroupSeverity returns the strictest (error > warning > info)
// severity any covering group assignment sets for ruleID.
func strictestGroupSeverity(groupAssignments []*RuleSeverityAssignment, ruleID string) (validation.Severity, bool) {
	var strictest validation.Severity
	found := false
	for _, a := range groupAssignments {
		sev, ok := a.Severities[ruleID]
		if !ok {
			continue
		}
		if !found || severityRank(sev) < severityRank(strictest) {
			strictest = sev
			found = true
		}
	}
	return strictest, found
}

// severityRank orders severities from strictest (0) to loosest, for
// picking the strictest of several candidates.
func severityRank(s validation.Severity) int {
	switch s {
	case validation.SeverityError:
		return 0
	case validation.SeverityWarning:
		return 1
	default: // SeverityInfo
		return 2
	}
}
