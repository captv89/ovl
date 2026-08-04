// SPDX-License-Identifier: AGPL-3.0-only

// Package synccred models the office side of a vessel's long-lived sync
// credential (architecture 11.1: "Authentication via the per-vessel
// credential from enrollment, revocable in the office"). Minted once,
// when an enrollment code is redeemed (architecture 11.2's sync
// handshake, see office/enrollment.Redeem) — every subsequent
// SyncService call authenticates against it until an Admin revokes it.
package synccred

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/captv89/ovl/pkg/authcrypto"
)

// tokenEntropyBytes is larger than enrollment codes' 10 bytes (80 bits):
// this is a bearer token presented on every sync call, not a human-typed
// one-time secret, so there is no reason to keep it short.
const tokenEntropyBytes = 32 // 256 bits

// Credential is one vessel's long-lived sync bearer token.
//
// TokenHash is the argon2id hash actually verified against a presented
// token (matches every other secret in this project, via pkg/authcrypto
// — the same primitive office/enrollment uses for codes and initial
// passwords).
//
// TokenLookupHash is a deterministic SHA-256 hex digest of the token,
// stored purely as a non-secret database index. Unlike enrollment code
// redemption (a one-time, human-driven bootstrap action bounded by fleet
// size, cheap to find by a linear argon2id scan — see
// office/enrollment.Redeem's doc comment), this credential is checked on
// every SyncService RPC from every vessel, so an O(n) argon2id scan
// across the whole fleet per call would be a real cost, not a
// hypothetical one. TokenLookupHash lets the auth interceptor find the
// candidate row in O(1) first. This is safe specifically because the
// token carries 256 bits of entropy: an unsalted SHA-256 digest of a
// secret that large is not brute-forceable, so indexing on it costs
// nothing that TokenHash's slow, salted hash would otherwise buy.
// TokenLookupHash never substitutes for TokenHash as the actual
// authentication check — Verify always argon2id-compares the exact row
// it finds.
type Credential struct {
	VesselID        string
	TokenHash       string
	TokenLookupHash string

	IssuedAt   time.Time
	RevokedAt  *time.Time // nil unless revoked
	LastUsedAt *time.Time // nil until the first successful SyncService call
}

// MintResult carries the one-time plaintext token a vessel must present
// (as a bearer credential) on every subsequent SyncService call. It is
// never stored anywhere and cannot be recovered later — a lost token
// means re-enrolling, the same "lost sheet means re-issuing" rule
// office/enrollment.IssueResult already follows.
type MintResult struct {
	Credential *Credential
	Token      string
}

// Mint issues a fresh credential for vesselID, superseding any existing
// one — a vessel has at most one active credential at a time, matching
// office/enrollment's own "revoke and re-issue in place" pattern rather
// than accumulating credential history.
func Mint(vesselID string) (*MintResult, error) {
	if vesselID == "" {
		return nil, errors.New("synccred: vessel id is required")
	}
	token, err := authcrypto.RandomToken(tokenEntropyBytes)
	if err != nil {
		return nil, fmt.Errorf("synccred: generate token: %w", err)
	}
	hash, err := authcrypto.HashPassword(token)
	if err != nil {
		return nil, fmt.Errorf("synccred: hash token: %w", err)
	}
	now := time.Now().UTC()
	c := &Credential{
		VesselID:        vesselID,
		TokenHash:       hash,
		TokenLookupHash: LookupHash(token),
		IssuedAt:        now,
	}
	return &MintResult{Credential: c, Token: token}, nil
}

// Verify reports whether token matches c and c has not been revoked.
func (c *Credential) Verify(token string) (bool, error) {
	if c == nil || c.RevokedAt != nil || c.TokenHash == "" {
		return false, nil
	}
	return authcrypto.VerifyPassword(token, c.TokenHash)
}

// Revoke invalidates c (architecture 11.1: "revocable in the office").
func (c *Credential) Revoke() {
	now := time.Now().UTC()
	c.RevokedAt = &now
}

// LookupHash computes the non-secret index value for token — see
// Credential.TokenLookupHash's doc comment for why a plain SHA-256 digest
// is safe here despite this project otherwise hashing every secret with
// argon2id. Exported so office/httpapi's auth interceptor (Step 2) can
// compute the same value for a presented bearer token to look up its
// candidate row, without duplicating the hashing logic.
func LookupHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
