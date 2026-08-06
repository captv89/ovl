// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/captv89/ovl/office/enrollment"
)

// UpsertEnrollment inserts e, or replaces the existing row for
// e.VesselID if one already exists — Issue and Reissue both produce a
// complete replacement of the row's contents (architecture 12.4:
// "revoke or re-issue at any time"), so there is no separate
// create-vs-update distinction at the store layer the way users/vessels
// have.
func (s *Store) UpsertEnrollment(ctx context.Context, e *enrollment.Enrollment) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO enrollments (vessel_id, state, code_hash, initial_master_username, initial_master_password_hash, dr_public_key, issued_at, revoked_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (vessel_id) DO UPDATE SET
			state = EXCLUDED.state,
			code_hash = EXCLUDED.code_hash,
			initial_master_username = EXCLUDED.initial_master_username,
			initial_master_password_hash = EXCLUDED.initial_master_password_hash,
			dr_public_key = EXCLUDED.dr_public_key,
			issued_at = EXCLUDED.issued_at,
			revoked_at = EXCLUDED.revoked_at,
			updated_at = EXCLUDED.updated_at
	`, e.VesselID, string(e.State), e.CodeHash, e.InitialMasterUsername, e.InitialMasterPasswordHash, nullIfEmpty(e.DRPublicKey),
		e.IssuedAt, e.RevokedAt, e.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert enrollment for vessel %s: %w", e.VesselID, err)
	}
	return nil
}

// nullIfEmpty maps an empty string to a real SQL NULL rather than an
// empty-string row value — DRPublicKey's own doc comment: absence
// (never redeemed with a keypair yet) is a distinct, meaningful state
// from "redeemed with an empty key," which should never happen but
// shouldn't be conflated with the real absent case if it somehow did.
func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// GetEnrollment returns the enrollment for vesselID. Returns ErrNotFound
// if the vessel has never had one issued — the absence of a row *is*
// "not enrolled," rather than a row with a NotEnrolled state, since
// nothing needs to exist before the first Issue.
func (s *Store) GetEnrollment(ctx context.Context, vesselID string) (*enrollment.Enrollment, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT vessel_id, state, code_hash, initial_master_username, initial_master_password_hash, dr_public_key, issued_at, revoked_at, updated_at
		FROM enrollments WHERE vessel_id = $1
	`, vesselID)
	return scanEnrollment(row)
}

// ListIssuedEnrollments returns every enrollment currently in
// StateIssued — the candidate set office/enrollment.Redeem scans against
// when a vessel presents a one-time code with no vessel id attached
// (codes are globally unique and self-identifying, Phase 4 decision).
func (s *Store) ListIssuedEnrollments(ctx context.Context) ([]*enrollment.Enrollment, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT vessel_id, state, code_hash, initial_master_username, initial_master_password_hash, dr_public_key, issued_at, revoked_at, updated_at
		FROM enrollments WHERE state = $1
	`, string(enrollment.StateIssued))
	if err != nil {
		return nil, fmt.Errorf("list issued enrollments: %w", err)
	}
	defer func() { _ = rows.Close() }() // rows.Err(), checked below, covers the meaningful failure

	var out []*enrollment.Enrollment
	for rows.Next() {
		e, err := scanEnrollment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate issued enrollments: %w", err)
	}
	return out, nil
}

func scanEnrollment(row rowScanner) (*enrollment.Enrollment, error) {
	var (
		e           enrollment.Enrollment
		state       string
		drPublicKey sql.NullString
		revokedAt   sql.NullTime
	)
	err := row.Scan(&e.VesselID, &state, &e.CodeHash, &e.InitialMasterUsername, &e.InitialMasterPasswordHash,
		&drPublicKey, &e.IssuedAt, &revokedAt, &e.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan enrollment: %w", err)
	}
	e.State = enrollment.State(state)
	if drPublicKey.Valid {
		e.DRPublicKey = drPublicKey.String
	}
	if revokedAt.Valid {
		e.RevokedAt = &revokedAt.Time
	}
	return &e, nil
}
