// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/captv89/ovl/office/synccred"
)

// UpsertVesselCredential inserts c, or replaces the existing row for
// c.VesselID if one already exists — re-enrolling (Redeem after a
// re-issued code) supersedes any prior credential in place, matching
// UpsertEnrollment's own "replace in place" pattern.
func (s *Store) UpsertVesselCredential(ctx context.Context, c *synccred.Credential) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO vessel_credentials (vessel_id, token_hash, token_lookup_hash, issued_at, revoked_at, last_used_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (vessel_id) DO UPDATE SET
			token_hash = EXCLUDED.token_hash,
			token_lookup_hash = EXCLUDED.token_lookup_hash,
			issued_at = EXCLUDED.issued_at,
			revoked_at = EXCLUDED.revoked_at,
			last_used_at = EXCLUDED.last_used_at
	`, c.VesselID, c.TokenHash, c.TokenLookupHash, c.IssuedAt, c.RevokedAt, c.LastUsedAt)
	if err != nil {
		return fmt.Errorf("upsert vessel credential for vessel %s: %w", c.VesselID, err)
	}
	return nil
}

// GetVesselCredentialByLookupHash returns the credential whose
// TokenLookupHash matches lookupHash — the O(1) candidate lookup the
// Step 2 auth interceptor uses before argon2id-verifying a presented
// token against it. Returns ErrNotFound if no row matches.
func (s *Store) GetVesselCredentialByLookupHash(ctx context.Context, lookupHash string) (*synccred.Credential, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT vessel_id, token_hash, token_lookup_hash, issued_at, revoked_at, last_used_at
		FROM vessel_credentials WHERE token_lookup_hash = $1
	`, lookupHash)
	return scanVesselCredential(row)
}

// RevokeVesselCredential revokes vesselID's current credential, if one
// exists — called alongside office/enrollment's own Revoke/Reissue
// (architecture 11.1: the credential is "revocable in the office," and
// design handoff B2's revoke/re-issue is the Admin-facing lever for
// that; a re-issued code implies the previous credential should stop
// working too, not silently keep syncing). A no-op if no credential has
// ever been minted for this vessel (RowsAffected is deliberately
// ignored) — that's the common case for an enrollment revoked/reissued
// before the vessel ever redeemed its code.
func (s *Store) RevokeVesselCredential(ctx context.Context, vesselID string, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE vessel_credentials SET revoked_at = $2 WHERE vessel_id = $1 AND revoked_at IS NULL
	`, vesselID, now)
	if err != nil {
		return fmt.Errorf("revoke vessel credential for vessel %s: %w", vesselID, err)
	}
	return nil
}

// TouchVesselCredentialLastUsed updates vesselID's credential
// last_used_at to now — called by the SyncService auth interceptor
// (Step 2) after every successful authentication, so an Admin reviewing
// vessel_credentials can tell an active credential from a stale one.
func (s *Store) TouchVesselCredentialLastUsed(ctx context.Context, vesselID string, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE vessel_credentials SET last_used_at = $2 WHERE vessel_id = $1
	`, vesselID, now)
	if err != nil {
		return fmt.Errorf("touch last-used for vessel %s: %w", vesselID, err)
	}
	return nil
}

func scanVesselCredential(row rowScanner) (*synccred.Credential, error) {
	var (
		c          synccred.Credential
		revokedAt  sql.NullTime
		lastUsedAt sql.NullTime
	)
	err := row.Scan(&c.VesselID, &c.TokenHash, &c.TokenLookupHash, &c.IssuedAt, &revokedAt, &lastUsedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan vessel credential: %w", err)
	}
	if revokedAt.Valid {
		c.RevokedAt = &revokedAt.Time
	}
	if lastUsedAt.Valid {
		c.LastUsedAt = &lastUsedAt.Time
	}
	return &c, nil
}
