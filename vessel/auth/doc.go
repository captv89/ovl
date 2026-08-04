// SPDX-License-Identifier: AGPL-3.0-only

// Package auth defines ovl-vessel's local user accounts and default
// roles (architecture 9.3): argon2id password hashing, the six fixed
// vessel roles, and the canSubmit/super-admin permission rules. It is
// vessel-only — the office has a completely different role vocabulary
// (architecture 12.2: Admin, Config Manager, Commercial Editor,
// Reviewer, Viewer) — and store-independent; vessel/store persists the
// User type this package defines.
package auth
