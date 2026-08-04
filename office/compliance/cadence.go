// SPDX-License-Identifier: AGPL-3.0-only

package compliance

import (
	"fmt"
	"time"
)

// Architecture 6.3's stated defaults: at least one report per rolling
// 24 hours, no gap between consecutive reports exceeding 12 hours.
const (
	DefaultMinReportIntervalHours = 24.0
	DefaultMaxGapHours            = 12.0
)

// CadenceRule is one scope's reporting-frequency requirement
// (architecture 6.3, design handoff B7). Both thresholds are
// independent knobs, matching the architecture doc's own wording ("both
// configurable") — it does not say MaxGapHours must be less than or
// equal to MinReportIntervalHours, so this does not enforce that
// relationship, only that each is a positive, finite number.
type CadenceRule struct {
	Scope                  Scope
	MinReportIntervalHours float64
	MaxGapHours            float64
	UpdatedAt              time.Time
}

// NewCadenceRule validates scope and both hour thresholds.
func NewCadenceRule(scope Scope, minReportIntervalHours, maxGapHours float64) (*CadenceRule, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	if minReportIntervalHours <= 0 {
		return nil, fmt.Errorf("compliance: min report interval must be positive, got %v", minReportIntervalHours)
	}
	if maxGapHours <= 0 {
		return nil, fmt.Errorf("compliance: max gap must be positive, got %v", maxGapHours)
	}
	return &CadenceRule{
		Scope:                  scope,
		MinReportIntervalHours: minReportIntervalHours,
		MaxGapHours:            maxGapHours,
		UpdatedAt:              time.Now().UTC(),
	}, nil
}

// EffectiveCadence resolves the cadence that applies to one vessel from
// whatever mix of fleet/group/vessel rules exist, falling back to the
// architecture 6.3 hardcoded defaults if none are assigned at all.
//
// Precedence, most to least specific: a vessel-scoped rule always wins
// outright (an explicit per-vessel choice is the most direct signal);
// otherwise every group-scoped rule covering one of the vessel's groups
// is considered, and the most restrictive one wins (smallest
// MaxGapHours, then smallest MinReportIntervalHours as a tiebreak) —
// unlike EffectiveProfiles' union semantics, a numeric threshold has no
// natural "union," and picking the strictest applicable rule is the
// safe default when a vessel belongs to two groups with different
// requirements; otherwise a fleet-wide rule; otherwise the hardcoded
// defaults.
func EffectiveCadence(rules []*CadenceRule, vesselID string, vesselGroups []string) CadenceRule {
	var vesselRule, fleetRule *CadenceRule
	var groupRule *CadenceRule
	for _, r := range rules {
		switch r.Scope.Type {
		case ScopeVessel:
			if r.Scope.Key == vesselID {
				vesselRule = r
			}
		case ScopeFleet:
			fleetRule = r
		case ScopeGroup:
			if !r.Scope.CoversVessel(vesselID, vesselGroups) {
				continue
			}
			if groupRule == nil || stricterCadence(r, groupRule) {
				groupRule = r
			}
		}
	}
	switch {
	case vesselRule != nil:
		return *vesselRule
	case groupRule != nil:
		return *groupRule
	case fleetRule != nil:
		return *fleetRule
	default:
		return CadenceRule{
			Scope:                  FleetScope(),
			MinReportIntervalHours: DefaultMinReportIntervalHours,
			MaxGapHours:            DefaultMaxGapHours,
		}
	}
}

// stricterCadence reports whether a is at least as restrictive as b (a
// smaller max gap, or equal max gap with a smaller min interval).
func stricterCadence(a, b *CadenceRule) bool {
	if a.MaxGapHours != b.MaxGapHours {
		return a.MaxGapHours < b.MaxGapHours
	}
	return a.MinReportIntervalHours < b.MinReportIntervalHours
}
