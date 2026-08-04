// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/captv89/ovl/pkg/domain"
)

// InboxCursors is this vessel's pull position on every PullInbox stream
// (architecture 11.2's SyncCursors) — the vessel-side mirror of what it
// last told the office it has, and what the office echoes back as
// next_cursors.
type InboxCursors struct {
	ConfigBundleCursor       int64
	SchemaVersionCursor      int64
	RemarkCursor             int64
	ChatCursor               int64
	InvalidationNoticeCursor int64
	RestoreCommandCursor     int64
	UserCommandCursor        int64
}

// GetInboxCursors returns this vessel's current pull position. Never
// ErrNotFound — the single row is seeded by migration 00006.
func (s *Store) GetInboxCursors(ctx context.Context) (InboxCursors, error) {
	var c InboxCursors
	row := s.db.QueryRowContext(ctx, `
		SELECT config_bundle_cursor, schema_version_cursor, remark_cursor, chat_cursor, invalidation_notice_cursor, restore_command_cursor, user_command_cursor
		FROM inbox_cursors WHERE id = 1
	`)
	if err := row.Scan(&c.ConfigBundleCursor, &c.SchemaVersionCursor, &c.RemarkCursor, &c.ChatCursor, &c.InvalidationNoticeCursor, &c.RestoreCommandCursor, &c.UserCommandCursor); err != nil {
		return InboxCursors{}, fmt.Errorf("get inbox cursors: %w", err)
	}
	return c, nil
}

// PulledSchemaVersion is one schema version received from PullInbox,
// ready to store.
type PulledSchemaVersion struct {
	SchemaName  string
	Version     string
	Content     []byte
	PublishedAt time.Time
}

// PulledConfigBundle is one config bundle received from PullInbox, ready
// to store.
type PulledConfigBundle struct {
	BundleID    string
	VersionNo   int64
	Content     []byte
	PublishedAt time.Time
}

// ApplyConfigBundle stores one config bundle outside of ApplyInboxPull's
// own transaction — used by the restore-bundle-apply path (architecture
// 12.5), where a bundle's embedded config snapshot arrives via a
// separate FetchRestoreBundle RPC, not a PullInbox cycle. Same insert-
// if-absent semantics as ApplyInboxPull's own config_bundles loop
// (immutable at the source, so a re-apply is always identical content).
func (s *Store) ApplyConfigBundle(ctx context.Context, cb PulledConfigBundle) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO config_bundles (bundle_id, version_no, content, published_at, received_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (bundle_id) DO NOTHING
	`, cb.BundleID, cb.VersionNo, cb.Content, cb.PublishedAt.UTC().Format(timeLayout), time.Now().UTC().Format(timeLayout))
	if err != nil {
		return fmt.Errorf("store config bundle %s: %w", cb.BundleID, err)
	}
	return nil
}

// AppliedConfigBundle is the config bundle a vessel currently runs on —
// the highest-version row in config_bundles. Content is the raw
// pkg/configwire.Bundle JSON the vessel decodes to resolve its effective
// field policy/prefill/severities/profiles/cadence.
type AppliedConfigBundle struct {
	BundleID  string
	VersionNo int64
	Content   []byte
}

// LatestConfigBundle returns the config bundle with the highest version_no
// — the one this vessel currently runs on — or ok=false if none has ever
// been pulled (a fresh vessel that has not yet synced a bundle, which
// falls back to the built-in stand-in config). Ordered by version_no, not
// received_at, so a re-pulled older bundle can never shadow a newer one.
func (s *Store) LatestConfigBundle(ctx context.Context) (AppliedConfigBundle, bool, error) {
	var cb AppliedConfigBundle
	row := s.db.QueryRowContext(ctx, `
		SELECT bundle_id, version_no, content
		FROM config_bundles ORDER BY version_no DESC LIMIT 1
	`)
	err := row.Scan(&cb.BundleID, &cb.VersionNo, &cb.Content)
	if errors.Is(err, sql.ErrNoRows) {
		return AppliedConfigBundle{}, false, nil
	}
	if err != nil {
		return AppliedConfigBundle{}, false, fmt.Errorf("latest config bundle: %w", err)
	}
	return cb, true, nil
}

// ApplyInboxPull stores newly pulled schema versions/config bundles/chat
// messages/invalidation notices/remarks and advances the cursors to
// next, all in one transaction — architecture 11.2's "applies it
// atomically." Storing content is insert-if-absent (ON CONFLICT DO
// NOTHING): schema versions and config bundles are immutable at the
// source so a retransmitted item is never different content, and chat
// messages/remarks carry a stable id minted at authoring time, so a
// re-pulled item is the same row, not a new one — every case is a
// harmless no-op on retransmission. Invalidation notices and remarks are
// handled slightly differently: the row itself is inserted in this same
// transaction, but driving the affected report through
// Invalidate/MarkRemarked + SaveReport + AppendEvent happens afterward,
// outside this transaction — those store methods don't accept a shared
// *sql.Tx, and both Report methods are themselves idempotent (their own
// doc comments), so a crash between the two just means a harmless
// re-apply next time, not a correctness gap.
func (s *Store) ApplyInboxPull(ctx context.Context, schemaVersions []PulledSchemaVersion, configBundles []PulledConfigBundle, chatMessages []domain.ChatMessage, invalidationNotices []domain.InvalidationNotice, remarks []domain.Remark, next InboxCursors) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin inbox pull apply: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op once Commit succeeds

	now := time.Now().UTC().Format(timeLayout)
	for _, sv := range schemaVersions {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO schema_versions (schema_name, version, content, published_at, received_at)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT (schema_name, version) DO NOTHING
		`, sv.SchemaName, sv.Version, sv.Content, sv.PublishedAt.UTC().Format(timeLayout), now); err != nil {
			return fmt.Errorf("store schema version %s@%s: %w", sv.SchemaName, sv.Version, err)
		}
	}
	for _, cb := range configBundles {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO config_bundles (bundle_id, version_no, content, published_at, received_at)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT (bundle_id) DO NOTHING
		`, cb.BundleID, cb.VersionNo, cb.Content, cb.PublishedAt.UTC().Format(timeLayout), now); err != nil {
			return fmt.Errorf("store config bundle %s: %w", cb.BundleID, err)
		}
	}
	for _, m := range chatMessages {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO chat_messages (id, report_id, sender, body, sent_at, direction)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT (id) DO NOTHING
		`, m.ID, m.ReportID, m.Sender, m.Body, m.SentAt.UTC().Format(timeLayout), string(m.Direction)); err != nil {
			return fmt.Errorf("store chat message %s: %w", m.ID, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE inbox_cursors SET
			config_bundle_cursor = ?, schema_version_cursor = ?, remark_cursor = ?,
			chat_cursor = ?, invalidation_notice_cursor = ?, restore_command_cursor = ?, user_command_cursor = ?
		WHERE id = 1
	`, next.ConfigBundleCursor, next.SchemaVersionCursor, next.RemarkCursor,
		next.ChatCursor, next.InvalidationNoticeCursor, next.RestoreCommandCursor, next.UserCommandCursor); err != nil {
		return fmt.Errorf("advance inbox cursors: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit inbox pull apply: %w", err)
	}

	for _, n := range invalidationNotices {
		if err := s.InsertInvalidationNotice(ctx, n, time.Now().UTC()); err != nil {
			return fmt.Errorf("store invalidation notice for %s v%d: %w", n.ReportID, n.VersionNo, err)
		}
		if err := s.applyInvalidationNotice(ctx, n); err != nil {
			return fmt.Errorf("apply invalidation notice for %s v%d: %w", n.ReportID, n.VersionNo, err)
		}
	}

	var newlyInsertedRemarks []domain.Remark
	for _, rm := range remarks {
		var alreadyExists bool
		if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM remarks WHERE id = ?)`, rm.ID).Scan(&alreadyExists); err != nil {
			return fmt.Errorf("check remark %s: %w", rm.ID, err)
		}
		if err := s.InsertRemark(ctx, rm); err != nil {
			return fmt.Errorf("store remark %s: %w", rm.ID, err)
		}
		if !alreadyExists {
			newlyInsertedRemarks = append(newlyInsertedRemarks, rm)
		}
	}
	if err := s.applyRemarks(ctx, newlyInsertedRemarks); err != nil {
		return fmt.Errorf("apply remarks: %w", err)
	}
	return nil
}

// applyInvalidationNotice drives the affected report's latest version
// through Invalidate and persists the resulting audit event, so the
// vessel's own audit trail shows the office-originated invalidation
// rather than a silent state flip (architecture 14). A no-op if the
// report doesn't exist locally yet (shouldn't happen in practice — the
// office only computes a notice against a report it already landed from
// this vessel — but defensive rather than fatal if it ever does), or if
// the report is already invalidated with these exact broken rules —
// Invalidate itself always emits a fresh event even when idempotent
// state-wise, so this guard (mirroring office/syncservice/cascade.go's
// own) is what actually keeps a retransmitted notice from duplicating
// the audit trail.
func (s *Store) applyInvalidationNotice(ctx context.Context, n domain.InvalidationNotice) error {
	r, err := s.GetLatestVersion(ctx, n.ReportID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if r.State == domain.StateInvalidated && slices.Equal(r.InvalidatedRules, n.BrokenRules) {
		return nil
	}
	event := r.Invalidate(n.BrokenRules, n.ComputedAt)
	if err := s.SaveReport(ctx, r); err != nil {
		return err
	}
	if _, err := s.AppendEvent(ctx, event); err != nil {
		return err
	}
	return nil
}

// remarkGroupKey identifies one B4 "send remark set" action as seen
// from the vessel side — the wire Remark message carries no set id
// (office/store's remarks table's own migration comment), so a
// (report, created_at, author) triple is what actually groups fields
// remarked together in one call.
type remarkGroupKey struct {
	ReportID  string
	CreatedAt time.Time
	Author    string
}

// applyRemarks groups newly-inserted remarks by remark set and drives
// each affected report through one MarkRemarked call per set (not one
// per field), so the vessel's audit trail shows one remarked event per
// reviewer action, matching the office side's own single-call behavior,
// rather than spamming one event per flagged field.
func (s *Store) applyRemarks(ctx context.Context, newlyInserted []domain.Remark) error {
	groups := make(map[remarkGroupKey][]string)
	for _, rm := range newlyInserted {
		key := remarkGroupKey{ReportID: rm.ReportID, CreatedAt: rm.CreatedAt, Author: rm.Author}
		groups[key] = append(groups[key], rm.FieldName)
	}
	for key, fieldNames := range groups {
		r, err := s.GetLatestVersion(ctx, key.ReportID)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		event, err := r.MarkRemarked(fieldNames, key.Author, key.CreatedAt)
		if err != nil {
			// e.g. the local report is still draft/ready — shouldn't
			// happen in practice (remarks only ever target a submitted-
			// or-later report), but don't fail the whole pull cycle over
			// one report's unexpected local state.
			continue
		}
		if err := s.SaveReport(ctx, r); err != nil {
			return err
		}
		if _, err := s.AppendEvent(ctx, event); err != nil {
			return err
		}
	}
	return nil
}
