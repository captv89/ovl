// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"

	"github.com/captv89/ovl/office/store"
	"github.com/captv89/ovl/pkg/authcrypto"
)

// runResetAdminPassword is `ovl-office reset-admin-password`: the
// office-admin-lockout recovery path (2026-07-21, alongside architecture
// 9.3/12.4's remote vessel-user administration — see that feature's own
// PROJECT.md entry for why office and vessel need two different answers
// to "the person who'd normally fix this is themselves locked out").
// Office has no one above it the way a vessel has office — if every
// Admin account is locked out, there is no in-app recovery path, only
// whoever already has direct access to the database this binary talks
// to. This subcommand doesn't grant that access; it requires it
// (-db-dsn, the same connection string OVL_OFFICE_DB_DSN/-db-dsn already
// use to run the server at all) — deliberately not a new secret to
// generate, print, and then have to protect (the risk the user
// specifically flagged for a bespoke recovery-code/CLI-token approach):
// whoever can already reach the database can already do this by hand
// with raw SQL and a hand-computed argon2id hash; this just makes doing
// it safely easier than that, the same shape Django's changepassword,
// GitLab's rails console reset, and WordPress's wp-cli all take for the
// identical problem.
func runResetAdminPassword(args []string) error {
	fs := flag.NewFlagSet("reset-admin-password", flag.ExitOnError)
	dsn := fs.String("db-dsn", envOr("OVL_OFFICE_DB_DSN", devDSN), "Postgres connection string (same as the server's own -db-dsn/OVL_OFFICE_DB_DSN)")
	username := fs.String("username", "", "username of the office account to reset (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *username == "" {
		return errors.New("reset-admin-password: -username is required")
	}

	st, err := store.Open(*dsn)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer func() { _ = st.Close() }()

	ctx := context.Background()
	u, err := st.GetUserByUsername(ctx, *username)
	if errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("no office user named %q", *username)
	}
	if err != nil {
		return fmt.Errorf("look up user %q: %w", *username, err)
	}

	temporaryPassword, err := authcrypto.RandomToken(12)
	if err != nil {
		return fmt.Errorf("generate temporary password: %w", err)
	}
	if err := u.ResetPassword(temporaryPassword); err != nil {
		return fmt.Errorf("reset password: %w", err)
	}
	// A locked-out Admin is sometimes locked out because the account was
	// deactivated (by another Admin, by mistake, or by itself before this
	// tool was needed), not just a forgotten password — reactivating here
	// too means this one command actually gets someone back in, not just
	// resets a password on an account that still can't log in.
	u.Reactivate()
	if err := st.UpdateUser(ctx, u); err != nil {
		return fmt.Errorf("save reset user: %w", err)
	}

	fmt.Printf("Password reset for %q. Temporary password (shown once, here only):\n\n  %s\n\n", *username, temporaryPassword)
	fmt.Println("This account must change its password on next login.")
	return nil
}
