// SPDX-License-Identifier: AGPL-3.0-only

// Package auth models ovl-office's local accounts and the five
// combinable office roles (architecture 12.2). This is a local stand-in
// pending real OIDC integration (Keycloak or Authentik, broker-federated
// to whichever IdP a deployment's own company uses) — deferred rather
// than built now.
// (An earlier version of this comment tied deferral to "Veracity
// federated as a conditional SSO source" — that idea is dead now that
// DNV has declined Veracity API access entirely, and OIDC was never
// actually blocked on it regardless.) Mirrors vessel/auth's shape (local
// account, argon2id password, forced-change-on-issue) on the
// one-role-per-user assumption dropped in favor of a combinable Roles
// set.
package auth

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/captv89/ovl/pkg/authcrypto"
)

// User is a local office account.
type User struct {
	ID           string // UUIDv7
	Username     string
	PasswordHash string
	Roles        Roles

	// MustChangePassword is set whenever a password is assigned by
	// someone other than the account holder (NewUser, ResetPassword) and
	// cleared once the holder sets their own (ChangePassword) — same "no
	// shared default passwords" reasoning as vessel/auth.User.
	MustChangePassword bool

	// Active gates login (design handoff B10's Users tab "deactivate"
	// action) — a soft toggle, not a delete, so the account's own
	// history (audit events, chat, remarks attributed to this username)
	// stays intact. See Deactivate/Reactivate.
	Active bool

	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewUser creates a local account with a temporary password that must
// be changed on first login. roles must be non-empty and every element
// valid; duplicates are silently deduplicated.
func NewUser(username, temporaryPassword string, roles Roles) (*User, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, errors.New("auth: username is required")
	}
	roles = roles.normalize()
	if err := validateRoles(roles); err != nil {
		return nil, err
	}
	hash, err := authcrypto.HashPassword(temporaryPassword)
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
		Roles:              roles,
		MustChangePassword: true,
		Active:             true,
		CreatedAt:          now,
		UpdatedAt:          now,
	}, nil
}

// SetRoles replaces u's role set.
func (u *User) SetRoles(roles Roles) error {
	roles = roles.normalize()
	if err := validateRoles(roles); err != nil {
		return err
	}
	u.Roles = roles
	u.UpdatedAt = time.Now().UTC()
	return nil
}

// ChangePassword is a self-initiated password change.
func (u *User) ChangePassword(newPassword string) error {
	hash, err := authcrypto.HashPassword(newPassword)
	if err != nil {
		return err
	}
	u.PasswordHash = hash
	u.MustChangePassword = false
	u.UpdatedAt = time.Now().UTC()
	return nil
}

// ResetPassword is an Admin-initiated reset. The new password is
// temporary, so MustChangePassword is set rather than cleared.
func (u *User) ResetPassword(temporaryPassword string) error {
	hash, err := authcrypto.HashPassword(temporaryPassword)
	if err != nil {
		return err
	}
	u.PasswordHash = hash
	u.MustChangePassword = true
	u.UpdatedAt = time.Now().UTC()
	return nil
}

// Authenticate reports whether password matches u's stored hash.
func (u *User) Authenticate(password string) (bool, error) {
	return authcrypto.VerifyPassword(password, u.PasswordHash)
}

// Deactivate blocks u from logging in (design handoff B10). Existing
// sessions are cut off by the caller re-checking Active on the loaded
// User, not by anything stored here — see office/httpapi's
// authenticatedUser.
func (u *User) Deactivate() {
	u.Active = false
	u.UpdatedAt = time.Now().UTC()
}

// Reactivate reverses Deactivate.
func (u *User) Reactivate() {
	u.Active = true
	u.UpdatedAt = time.Now().UTC()
}

// CanManageUsers reports whether u may manage users, vessels, groups,
// and enrollment/credential revocation (architecture 12.2, Admin).
func (u *User) CanManageUsers() bool {
	return u.Roles.Has(RoleAdmin)
}

// CanManageConfig reports whether u may author schema versions, field
// policies, regulatory profiles, cadence rules, and publish bundles
// (architecture 12.2, Config Manager).
func (u *User) CanManageConfig() bool {
	return u.Roles.Has(RoleConfigManager)
}

// CanEditCommercialData reports whether u may enter Cargo Nomination and
// Commercial Period data (architecture 12.2, Commercial Editor).
func (u *User) CanEditCommercialData() bool {
	return u.Roles.Has(RoleCommercialEditor)
}

// CanReview reports whether u may flag fields with remarks and chat with
// vessels (architecture 12.2, Reviewer).
func (u *User) CanReview() bool {
	return u.Roles.Has(RoleReviewer)
}
