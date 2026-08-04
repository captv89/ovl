// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/captv89/ovl/office/auth"
)

// CreateUser inserts a new user. Returns an error if username is
// already taken (the users.username UNIQUE constraint).
func (s *Store) CreateUser(ctx context.Context, u *auth.User) error {
	rolesJSON, err := json.Marshal(u.Roles)
	if err != nil {
		return fmt.Errorf("marshal roles for user %s: %w", u.Username, err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO users (id, username, password_hash, roles, must_change_password, active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, u.ID, u.Username, u.PasswordHash, string(rolesJSON), u.MustChangePassword, u.Active, u.CreatedAt, u.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create user %s: %w", u.Username, err)
	}
	return nil
}

// UpdateUser persists changes to an existing user (password
// reset/change, role changes).
func (s *Store) UpdateUser(ctx context.Context, u *auth.User) error {
	rolesJSON, err := json.Marshal(u.Roles)
	if err != nil {
		return fmt.Errorf("marshal roles for user %s: %w", u.ID, err)
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE users SET
			username = $1, password_hash = $2, roles = $3, must_change_password = $4, active = $5, updated_at = $6
		WHERE id = $7
	`, u.Username, u.PasswordHash, string(rolesJSON), u.MustChangePassword, u.Active, u.UpdatedAt, u.ID)
	if err != nil {
		return fmt.Errorf("update user %s: %w", u.ID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update user %s: %w", u.ID, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetUser returns the user with the given ID.
func (s *Store) GetUser(ctx context.Context, id string) (*auth.User, error) {
	row := s.db.QueryRowContext(ctx, userColumns+` FROM users WHERE id = $1`, id)
	return scanUser(row)
}

// GetUserByUsername returns the user with the given username.
func (s *Store) GetUserByUsername(ctx context.Context, username string) (*auth.User, error) {
	row := s.db.QueryRowContext(ctx, userColumns+` FROM users WHERE username = $1`, username)
	return scanUser(row)
}

// HasAnyUser reports whether at least one user has been created.
// office has no first-run wizard equivalent yet (unlike vessel's
// architecture 9.2) — this is here for whenever an initial-Admin
// bootstrap flow is designed, same forward-looking reasoning as
// vessel/store's own HasAnyUser.
func (s *Store) HasAnyUser(ctx context.Context) (bool, error) {
	var exists bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users)`).Scan(&exists); err != nil {
		return false, fmt.Errorf("check for any user: %w", err)
	}
	return exists, nil
}

// ListUsers returns every user, ordered by username.
func (s *Store) ListUsers(ctx context.Context) ([]*auth.User, error) {
	rows, err := s.db.QueryContext(ctx, userColumns+` FROM users ORDER BY username ASC`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer func() { _ = rows.Close() }() // rows.Err(), checked below, covers the meaningful failure

	var out []*auth.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate users: %w", err)
	}
	return out, nil
}

const userColumns = `SELECT id, username, password_hash, roles, must_change_password, active, created_at, updated_at`

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanUser(row rowScanner) (*auth.User, error) {
	var (
		u         auth.User
		rolesJSON []byte
	)
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &rolesJSON, &u.MustChangePassword, &u.Active, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan user: %w", err)
	}
	if err := json.Unmarshal(rolesJSON, &u.Roles); err != nil {
		return nil, fmt.Errorf("unmarshal roles for user %s: %w", u.ID, err)
	}
	return &u, nil
}
