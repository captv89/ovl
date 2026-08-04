// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// User is a local vessel account (architecture 9.3). Fully offline
// capable — there is no external identity provider on the vessel side.
type User struct {
	ID           string // UUIDv7
	Username     string
	PasswordHash string
	Role         Role

	// CanSubmit is the explicit per-user override architecture 9.3
	// describes ("plus any user Master flags with canSubmit"). It is
	// ignored for RoleMaster, which can always submit regardless of this
	// flag's stored value — see CanSubmitReports.
	CanSubmit bool

	// MustChangePassword is set whenever a password is assigned by
	// someone other than the account holder (NewUser, ResetPassword) and
	// cleared once the holder sets their own (ChangePassword) —
	// architecture 9.2's "forced password change on first login" and
	// "no fleet-wide default passwords ever."
	MustChangePassword bool

	// Active gates login (design handoff A9's "deactivate" action). A
	// deactivated user cannot authenticate, and an existing session
	// stops resolving on its very next request — see httpapi's
	// authenticatedUser, which re-loads the user per request rather than
	// trusting a cached session claim.
	Active bool

	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewUser creates a local account with a temporary password that must
// be changed on first login (MustChangePassword is always true; there
// is no way to construct a User that skips this, per "no fleet-wide
// default passwords ever").
func NewUser(username, temporaryPassword string, role Role) (*User, error) {
	// Trimmed so " master" and "master" can't become two distinct
	// accounts that look identical in the UI — a lookalike-account risk
	// that matters more than usual here since Master identity drives
	// super-admin authorization decisions.
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, errors.New("auth: username is required")
	}
	if err := validateRole(role); err != nil {
		return nil, err
	}
	hash, err := hashPassword(temporaryPassword)
	if err != nil {
		return nil, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("auth: generate user id: %w", err)
	}
	now := time.Now().UTC()
	return &User{
		ID:                 id.String(),
		Username:           username,
		PasswordHash:       hash,
		Role:               role,
		MustChangePassword: true,
		Active:             true,
		CreatedAt:          now,
		UpdatedAt:          now,
	}, nil
}

// ChangePassword is a self-initiated password change: it satisfies
// MustChangePassword going forward. Use ResetPassword for a Master
// resetting someone else's password instead.
func (u *User) ChangePassword(newPassword string) error {
	hash, err := hashPassword(newPassword)
	if err != nil {
		return err
	}
	u.PasswordHash = hash
	u.MustChangePassword = false
	u.UpdatedAt = time.Now().UTC()
	return nil
}

// ResetPassword is a Master-initiated reset (architecture 9.3: "Master
// ... resets passwords"): the new password is temporary, so
// MustChangePassword is set rather than cleared — the same "no
// fleet-wide default passwords" reasoning as NewUser applies to a reset
// password just as much as an initial one.
func (u *User) ResetPassword(temporaryPassword string) error {
	hash, err := hashPassword(temporaryPassword)
	if err != nil {
		return err
	}
	u.PasswordHash = hash
	u.MustChangePassword = true
	u.UpdatedAt = time.Now().UTC()
	return nil
}

// SetRole changes u's role (architecture 9.3/12.4's remote user
// administration, 2026-07-21 — no local vessel screen exposes this yet,
// only office's remote UserCommand path does; added here rather than
// inlined at that one call site since role validation belongs with the
// type that owns Role, same as everything else in this file). Refuses
// RoleMaster — becoming Master is a deliberate local ceremony
// (first-run setup only, see vessel/httpapi/setup.go's
// handleSetupMaster), never a side effect of a role change, remote or
// local.
func (u *User) SetRole(role Role) error {
	if role == RoleMaster {
		return errors.New("auth: promoting an account to Master must be done during first-run setup, not by changing an existing account's role")
	}
	if err := validateRole(role); err != nil {
		return err
	}
	u.Role = role
	u.UpdatedAt = time.Now().UTC()
	return nil
}

// SetCanSubmit updates the canSubmit override (architecture 9.3). It is
// harmless to call on a Master account (CanSubmitReports ignores the
// flag for Master either way), but there is normally no reason to.
func (u *User) SetCanSubmit(canSubmit bool) {
	u.CanSubmit = canSubmit
	u.UpdatedAt = time.Now().UTC()
}

// SetActive updates the active flag (design handoff A9's "deactivate"/
// "reactivate" actions). Callers are expected to reject deactivating a
// Master account or the caller's own account before calling this — see
// httpapi's admin user handlers — this method itself has no opinion on
// who may be deactivated.
func (u *User) SetActive(active bool) {
	u.Active = active
	u.UpdatedAt = time.Now().UTC()
}

// CanSubmitReports reports whether u may press Submit on a report
// (architecture 9.3): Master always can; everyone else needs the
// canSubmit flag.
func (u *User) CanSubmitReports() bool {
	return u.Role == RoleMaster || u.CanSubmit
}

// IsSuperAdmin reports whether u has Master's super-admin powers: manage
// users, reset passwords, force-release section locks (architecture
// 9.3; force-release itself is Phase 4/9.5 concurrent-editing work, not
// implemented here).
func (u *User) IsSuperAdmin() bool {
	return u.Role == RoleMaster
}

// Authenticate reports whether password matches u's stored hash.
func (u *User) Authenticate(password string) (bool, error) {
	return VerifyPassword(password, u.PasswordHash)
}
