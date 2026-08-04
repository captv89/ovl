// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"fmt"
	"slices"
)

// Role is one of the six fixed vessel roles (architecture 9.3). Unlike
// office roles, these are not company-configurable in Phase 2 — the
// architecture doc's "the default role set arrives with the vessel's
// first config bundle" (9.2) describes Phase 3/4 config-bundle delivery,
// which doesn't exist yet; until then this fixed Go enum is the "local
// test config" the Phase 2 checklist anticipates.
type Role string

const (
	RoleMaster         Role = "master"
	RoleChiefOfficer   Role = "chiefOfficer"
	RoleSecondOfficer  Role = "secondOfficer"
	RoleThirdOfficer   Role = "thirdOfficer"
	RoleChiefEngineer  Role = "chiefEngineer"
	RoleSecondEngineer Role = "secondEngineer"
)

// DefaultRoles lists the six roles every vessel starts with.
func DefaultRoles() []Role {
	return []Role{RoleMaster, RoleChiefOfficer, RoleSecondOfficer, RoleThirdOfficer, RoleChiefEngineer, RoleSecondEngineer}
}

// Valid reports whether r is one of DefaultRoles.
func (r Role) Valid() bool {
	return slices.Contains(DefaultRoles(), r)
}

func validateRole(r Role) error {
	if !r.Valid() {
		return fmt.Errorf("auth: %q is not a valid role", r)
	}
	return nil
}
