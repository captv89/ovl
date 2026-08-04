// SPDX-License-Identifier: AGPL-3.0-only

// Package apikey models external customers' bearer credentials for the
// data API (architecture 13.1) — API-key-gated GraphQL/CSV access,
// separate from and parallel to office staff's own session-cookie login.
// Modeled directly on office/synccred (a vessel's sync bearer token):
// same lookup-hash + slow-hash split, since this credential is checked
// on every request rather than redeemed once like an enrollment code.
package apikey

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/captv89/ovl/pkg/authcrypto"
)

// tokenEntropyBytes matches office/synccred's own bearer-token size —
// this is presented on every API call, not a human-typed one-time
// secret, so there's no reason to keep it short.
const tokenEntropyBytes = 32 // 256 bits

// APIKey is one external customer's data-API credential.
//
// TokenHash is the argon2id hash actually verified against a presented
// key (pkg/authcrypto, same primitive every other secret in this project
// uses). TokenLookupHash is a non-secret SHA-256 hex digest used purely
// as an O(1) database index — see office/synccred.Credential's own doc
// comment for why that split (and not a linear argon2id scan, fine for
// enrollment's rare human-redeemed codes but not for a credential
// checked on every request) is the right shape here, not there.
type APIKey struct {
	ID              string
	Label           string
	TokenHash       string
	TokenLookupHash string
	GroupID         *string // free-form vessel-group tag; nil means unscoped (every vessel)
	CreatedBy       string  // issuing admin's username
	CreatedAt       time.Time
	RevokedAt       *time.Time
	LastUsedAt      *time.Time
}

// MintResult carries the one-time plaintext key a customer must present
// as a bearer credential on every request. Never stored anywhere and
// cannot be recovered later — a lost key means issuing a new one, same
// "lost secret means re-issuing" rule every other credential in this
// project follows.
type MintResult struct {
	APIKey *APIKey
	Token  string
}

// Mint issues a fresh API key. label is the admin-supplied name for
// whoever holds this key (e.g. a customer/integration name) — purely
// descriptive, not unique. groupID scopes the key to one vessel group
// tag; nil leaves it unscoped.
func Mint(label, createdBy string, groupID *string) (*MintResult, error) {
	label = strings.TrimSpace(label)
	if label == "" {
		return nil, errors.New("apikey: label is required")
	}
	if createdBy == "" {
		return nil, errors.New("apikey: created-by is required")
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("apikey: generate id: %w", err)
	}
	token, err := authcrypto.RandomToken(tokenEntropyBytes)
	if err != nil {
		return nil, fmt.Errorf("apikey: generate token: %w", err)
	}
	hash, err := authcrypto.HashPassword(token)
	if err != nil {
		return nil, fmt.Errorf("apikey: hash token: %w", err)
	}
	k := &APIKey{
		ID:              id.String(),
		Label:           label,
		TokenHash:       hash,
		TokenLookupHash: LookupHash(token),
		GroupID:         groupID,
		CreatedBy:       createdBy,
		CreatedAt:       time.Now().UTC(),
	}
	return &MintResult{APIKey: k, Token: token}, nil
}

// Verify reports whether token matches k and k has not been revoked.
func (k *APIKey) Verify(token string) (bool, error) {
	if k == nil || k.RevokedAt != nil || k.TokenHash == "" {
		return false, nil
	}
	return authcrypto.VerifyPassword(token, k.TokenHash)
}

// Revoke invalidates k immediately.
func (k *APIKey) Revoke() {
	now := time.Now().UTC()
	k.RevokedAt = &now
}

// LookupHash computes the non-secret index value for token — see
// APIKey.TokenLookupHash's doc comment for why a plain SHA-256 digest is
// safe here. Exported so office/httpapi's auth check (authenticatedAPIKey)
// can compute the same value for a presented bearer token to look up its
// candidate row, without duplicating the hashing logic.
func LookupHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
