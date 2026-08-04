// SPDX-License-Identifier: AGPL-3.0-only

// Package compliance models the office side of regulatory profile
// assignment (architecture 6.2) and cadence rules (architecture 6.3;
// design handoff B7), both of which the design doc describes with the
// identical assignment shape: "assignable fleet-wide, per group or per
// vessel." Scope captures that shared shape once instead of duplicating
// it across two otherwise-unrelated config types.
package compliance

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ScopeType is how narrowly a ProfileAssignment or CadenceRule applies.
type ScopeType string

const (
	ScopeFleet  ScopeType = "fleet"
	ScopeGroup  ScopeType = "group"
	ScopeVessel ScopeType = "vessel"
)

// Scope identifies one assignment target. Key is empty for ScopeFleet,
// a free-form group tag (matching office/vessels.Vessel.Groups) for
// ScopeGroup, or a vessel ID (UUID) for ScopeVessel.
type Scope struct {
	Type ScopeType
	Key  string
}

// FleetScope is the single fleet-wide scope.
func FleetScope() Scope { return Scope{Type: ScopeFleet} }

// GroupScope is one group-tag scope. group must be non-empty.
func GroupScope(group string) (Scope, error) {
	group = strings.TrimSpace(group)
	if group == "" {
		return Scope{}, errors.New("compliance: group scope requires a non-empty group tag")
	}
	return Scope{Type: ScopeGroup, Key: group}, nil
}

// VesselScope is one vessel-specific scope. vesselID must be non-empty.
func VesselScope(vesselID string) (Scope, error) {
	vesselID = strings.TrimSpace(vesselID)
	if vesselID == "" {
		return Scope{}, errors.New("compliance: vessel scope requires a non-empty vessel id")
	}
	return Scope{Type: ScopeVessel, Key: vesselID}, nil
}

// Validate reports whether s is well-formed (a known Type with a Key
// matching what that Type requires).
func (s Scope) Validate() error {
	switch s.Type {
	case ScopeFleet:
		if s.Key != "" {
			return errors.New("compliance: fleet scope must not carry a key")
		}
	case ScopeGroup, ScopeVessel:
		if s.Key == "" {
			return fmt.Errorf("compliance: %s scope requires a key", s.Type)
		}
	default:
		return fmt.Errorf("compliance: unknown scope type %q", s.Type)
	}
	return nil
}

// CoversVessel reports whether s applies to a vessel with the given ID
// and group tags: fleet scope always matches, group scope matches if
// groups contains Key, vessel scope matches only that exact vessel.
// Exported (not just used internally by this package's own EffectiveX
// resolvers) so office/fieldpolicy.EffectiveFieldPolicy can reuse the
// exact same scope-matching rule rather than re-declaring it.
func (s Scope) CoversVessel(vesselID string, groups []string) bool {
	switch s.Type {
	case ScopeFleet:
		return true
	case ScopeVessel:
		return s.Key == vesselID
	case ScopeGroup:
		return slices.Contains(groups, s.Key)
	default:
		return false
	}
}
