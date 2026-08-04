// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/captv89/ovl/vessel/auth"
)

// CreateUser inserts a new user. Returns an error if username is
// already taken (the users.username UNIQUE constraint).
func (s *Store) CreateUser(ctx context.Context, u *auth.User) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO users (id, username, password_hash, role, can_submit, must_change_password, active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, u.ID, u.Username, u.PasswordHash, string(u.Role), u.CanSubmit, u.MustChangePassword, u.Active,
		u.CreatedAt.UTC().Format(timeLayout), u.UpdatedAt.UTC().Format(timeLayout))
	if err != nil {
		return fmt.Errorf("create user %s: %w", u.Username, err)
	}
	return nil
}

// UpdateUser persists changes to an existing user (password
// reset/change, canSubmit toggle, role change).
func (s *Store) UpdateUser(ctx context.Context, u *auth.User) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE users SET
			username = ?, password_hash = ?, role = ?, can_submit = ?, must_change_password = ?, active = ?, updated_at = ?
		WHERE id = ?
	`, u.Username, u.PasswordHash, string(u.Role), u.CanSubmit, u.MustChangePassword, u.Active,
		u.UpdatedAt.UTC().Format(timeLayout), u.ID)
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
	row := s.db.QueryRowContext(ctx, userColumns+` FROM users WHERE id = ?`, id)
	return scanUser(row)
}

// GetUserByUsername returns the user with the given username.
func (s *Store) GetUserByUsername(ctx context.Context, username string) (*auth.User, error) {
	row := s.db.QueryRowContext(ctx, userColumns+` FROM users WHERE username = ?`, username)
	return scanUser(row)
}

// HasAnyUser reports whether at least one user has been created — the
// first-run wizard uses this to decide whether Master bootstrap
// (architecture 9.2 step 3) still needs to run.
func (s *Store) HasAnyUser(ctx context.Context) (bool, error) {
	var exists bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users)`).Scan(&exists); err != nil {
		return false, fmt.Errorf("check for any user: %w", err)
	}
	return exists, nil
}

// ListUsers returns every user, ordered by username — the shape a
// Master's user management screen needs.
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

const userColumns = `SELECT id, username, password_hash, role, can_submit, must_change_password, active, created_at, updated_at`

func scanUser(row rowScanner) (*auth.User, error) {
	var (
		u                    auth.User
		role                 string
		createdAt, updatedAt string
	)
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &role, &u.CanSubmit, &u.MustChangePassword, &u.Active, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan user: %w", err)
	}
	u.Role = auth.Role(role)
	if u.CreatedAt, err = time.Parse(timeLayout, createdAt); err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	if u.UpdatedAt, err = time.Parse(timeLayout, updatedAt); err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}
	return &u, nil
}
