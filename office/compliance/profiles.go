// SPDX-License-Identifier: AGPL-3.0-only

package compliance

import (
	"time"

	"github.com/captv89/ovl/pkg/validation"
)

// ProfileAssignment is the set of regulatory profiles enabled at one
// scope (architecture 6.2's "toggle cards", assignable fleet-wide, per
// group, or per vessel). Reuses validation.RegulatoryProfile — the same
// four-value vocabulary the health check's readiness engine already
// evaluates against — rather than re-declaring profile names here.
type ProfileAssignment struct {
	Scope     Scope
	Profiles  []validation.RegulatoryProfile
	UpdatedAt time.Time
}

// NewProfileAssignment validates scope and normalizes profiles
// (deduplicated, unknown values rejected).
func NewProfileAssignment(scope Scope, profiles []validation.RegulatoryProfile) (*ProfileAssignment, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	known := make(map[validation.RegulatoryProfile]bool, len(validation.AllRegulatoryProfiles))
	for _, p := range validation.AllRegulatoryProfiles {
		known[p] = true
	}
	seen := make(map[validation.RegulatoryProfile]bool, len(profiles))
	out := make([]validation.RegulatoryProfile, 0, len(profiles))
	for _, p := range profiles {
		if !known[p] {
			return nil, &UnknownProfileError{Profile: p}
		}
		if seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return &ProfileAssignment{Scope: scope, Profiles: out, UpdatedAt: time.Now().UTC()}, nil
}

// UnknownProfileError is returned when a caller names a profile outside
// validation.AllRegulatoryProfiles.
type UnknownProfileError struct {
	Profile validation.RegulatoryProfile
}

func (e *UnknownProfileError) Error() string {
	return "compliance: unknown regulatory profile " + string(e.Profile)
}

// EffectiveProfiles computes which regulatory profiles apply to one
// vessel: the union of every assignment whose scope covers the vessel
// (fleet-wide, any of its groups, or a vessel-specific assignment).
//
// Union, not narrowest-scope-wins, is the deliberate choice here:
// unlike cadence (a numeric threshold, where "most restrictive wins" is
// the safe default — see EffectiveCadence), a regulatory profile is a
// yes/no reporting obligation. Nothing in either handoff doc says a
// vessel-level toggle can turn a fleet-wide obligation back off, and
// silently dropping a compliance profile because of scope precedence
// would be the wrong failure mode for something CII/MRV/DCS-shaped —
// so enabling it anywhere enables it everywhere it applies. Revisit if
// a real deployment surfaces a need for an explicit vessel-level
// opt-out.
func EffectiveProfiles(assignments []*ProfileAssignment, vesselID string, vesselGroups []string) []validation.RegulatoryProfile {
	enabled := map[validation.RegulatoryProfile]bool{}
	for _, a := range assignments {
		if !a.Scope.CoversVessel(vesselID, vesselGroups) {
			continue
		}
		for _, p := range a.Profiles {
			enabled[p] = true
		}
	}
	var out []validation.RegulatoryProfile
	for _, p := range validation.AllRegulatoryProfiles {
		if enabled[p] {
			out = append(out, p)
		}
	}
	return out
}
