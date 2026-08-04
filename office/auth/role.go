// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"fmt"
	"slices"
)

// Role is one of the five office roles (architecture 12.2). Unlike
// vessel/auth.Role (one fixed role per user), office roles are
// explicitly combinable — a user holds a set of them (see Roles below).
type Role string

const (
	// RoleAdmin manages users, vessels, groups, enrollment, and
	// credential revocation.
	RoleAdmin Role = "admin"
	// RoleConfigManager manages schema versions, field policies,
	// regulatory profiles, cadence rules, and bundle publishing.
	RoleConfigManager Role = "configManager"
	// RoleCommercialEditor enters Cargo Nomination and Commercial Period
	// data.
	RoleCommercialEditor Role = "commercialEditor"
	// RoleReviewer views everything, flags fields with remarks, and
	// chats with vessels.
	RoleReviewer Role = "reviewer"
	// RoleViewer has read-only access across reports, the dashboard, and
	// logs.
	RoleViewer Role = "viewer"
)

// AllRoles lists the five roles architecture 12.2 defines.
func AllRoles() []Role {
	return []Role{RoleAdmin, RoleConfigManager, RoleCommercialEditor, RoleReviewer, RoleViewer}
}

// Valid reports whether r is one of AllRoles.
func (r Role) Valid() bool {
	return slices.Contains(AllRoles(), r)
}

// Roles is a set of Role values held by one user — office roles are
// combinable (architecture 12.2), unlike vessel/auth.Role's one-per-user
// model.
type Roles []Role

// Has reports whether rs contains r.
func (rs Roles) Has(r Role) bool {
	return slices.Contains(rs, r)
}

// normalize returns rs deduplicated. Order is not meaningful, so
// duplicates (e.g. from a caller-constructed slice) are silently
// dropped rather than treated as an error.
func (rs Roles) normalize() Roles {
	out := make(Roles, 0, len(rs))
	for _, r := range rs {
		if !slices.Contains(out, r) {
			out = append(out, r)
		}
	}
	return out
}

func validateRoles(rs Roles) error {
	if len(rs) == 0 {
		return fmt.Errorf("auth: at least one role is required")
	}
	for _, r := range rs {
		if !r.Valid() {
			return fmt.Errorf("auth: %q is not a valid role", r)
		}
	}
	return nil
}
