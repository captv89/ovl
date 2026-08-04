// SPDX-License-Identifier: AGPL-3.0-only

package auth

import "testing"

func TestRole_Valid(t *testing.T) {
	for _, r := range DefaultRoles() {
		if !r.Valid() {
			t.Errorf("DefaultRoles() member %q reports Valid() = false", r)
		}
	}
	if Role("harbor-pilot").Valid() {
		t.Error(`Role("harbor-pilot").Valid() = true, want false`)
	}
	if len(DefaultRoles()) != 6 {
		t.Errorf("DefaultRoles() has %d roles, want 6 (architecture 9.3)", len(DefaultRoles()))
	}
}
