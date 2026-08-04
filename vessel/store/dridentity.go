// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// DRIdentity is this vessel's restore-bundle decryption keypair
// (architecture 12.5, pkg/backupcrypto).
type DRIdentity struct {
	PublicKey  string
	PrivateKey string
	IssuedAt   time.Time
}

// SaveDRIdentity replaces this vessel's stored DR keypair — at most one,
// same "supersedes in place" shape as SaveSyncCredential.
func (s *Store) SaveDRIdentity(ctx context.Context, id *DRIdentity) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO dr_identity (id, public_key, private_key, issued_at)
		VALUES (1, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET public_key = excluded.public_key, private_key = excluded.private_key, issued_at = excluded.issued_at
	`, id.PublicKey, id.PrivateKey, id.IssuedAt.UTC().Format(timeLayout))
	if err != nil {
		return fmt.Errorf("save dr identity: %w", err)
	}
	return nil
}

// GetDRIdentity returns this vessel's stored DR keypair. Returns
// ErrNotFound if the vessel has never redeemed an enrollment code since
// this field was added.
func (s *Store) GetDRIdentity(ctx context.Context) (*DRIdentity, error) {
	row := s.db.QueryRowContext(ctx, `SELECT public_key, private_key, issued_at FROM dr_identity WHERE id = 1`)
	var (
		id       DRIdentity
		issuedAt string
	)
	err := row.Scan(&id.PublicKey, &id.PrivateKey, &issuedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan dr identity: %w", err)
	}
	if id.IssuedAt, err = time.Parse(timeLayout, issuedAt); err != nil {
		return nil, fmt.Errorf("parse issued_at: %w", err)
	}
	return &id, nil
}
