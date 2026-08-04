// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"fmt"
	"time"
)

// VesselChatNotificationRow is one vessel-authored chat message, for
// the notification panel's "remark" category (Office UI rework Phase
// O6, design handoff B1·N: "Reply from MV Aurelia"). Office-authored
// messages never generate a notification for an office user — this is
// deliberately scoped to direction='vessel' only.
type VesselChatNotificationRow struct {
	MessageID  string
	VesselID   string
	VesselName string
	VesselIMO  string
	ReportID   string
	EventType  string
	Sender     string
	Body       string
	SentAt     time.Time
}

// ListRecentVesselChatMessages returns every vessel-authored chat
// message sent since since, newest first — capped by the caller's own
// lookback window, not by this query, since "recent" is a notification-
// panel concept, not a store one.
func (s *Store) ListRecentVesselChatMessages(ctx context.Context, since time.Time) ([]VesselChatNotificationRow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT cm.id, cm.vessel_id, v.name, v.imo, cm.report_id,
			COALESCE((
				SELECT rv.event_type FROM report_versions rv
				WHERE rv.vessel_id = cm.vessel_id AND rv.report_id = cm.report_id
				ORDER BY rv.version_no DESC LIMIT 1
			), ''),
			cm.sender, cm.body, cm.sent_at
		FROM chat_messages cm
		JOIN vessels v ON v.id = cm.vessel_id
		WHERE cm.direction = 'vessel' AND cm.sent_at >= $1
		ORDER BY cm.sent_at DESC
	`, since)
	if err != nil {
		return nil, fmt.Errorf("list recent vessel chat messages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []VesselChatNotificationRow
	for rows.Next() {
		var row VesselChatNotificationRow
		if err := rows.Scan(&row.MessageID, &row.VesselID, &row.VesselName, &row.VesselIMO, &row.ReportID,
			&row.EventType, &row.Sender, &row.Body, &row.SentAt); err != nil {
			return nil, fmt.Errorf("scan vessel chat notification row: %w", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate vessel chat notification rows: %w", err)
	}
	return out, nil
}

// SyncNotificationRow is one vessel's report-landing count for one day,
// for the notification panel's "sync" category ("12 reports synced ·
// Handysize"). LastReceivedAt anchors the notification's own timestamp
// (the day's last landing, not the day's start) so it sorts sensibly
// alongside chat/overdue notifications.
type SyncNotificationRow struct {
	VesselID       string
	VesselName     string
	Day            string // "2026-07-15", UTC
	Count          int
	LastReceivedAt time.Time
}

// ListRecentSyncActivity groups report_versions landings by (vessel,
// day) since since — the same "reports synced" rollup design handoff
// B1·N shows, computed live rather than stored, matching this file's own
// package doc comment on why there is no notifications table.
func (s *Store) ListRecentSyncActivity(ctx context.Context, since time.Time) ([]SyncNotificationRow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT rv.vessel_id, v.name, TO_CHAR(rv.received_at, 'YYYY-MM-DD'), COUNT(*), MAX(rv.received_at)
		FROM report_versions rv
		JOIN vessels v ON v.id = rv.vessel_id
		WHERE rv.received_at >= $1
		GROUP BY rv.vessel_id, v.name, TO_CHAR(rv.received_at, 'YYYY-MM-DD')
		ORDER BY MAX(rv.received_at) DESC
	`, since)
	if err != nil {
		return nil, fmt.Errorf("list recent sync activity: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []SyncNotificationRow
	for rows.Next() {
		var row SyncNotificationRow
		if err := rows.Scan(&row.VesselID, &row.VesselName, &row.Day, &row.Count, &row.LastReceivedAt); err != nil {
			return nil, fmt.Errorf("scan sync notification row: %w", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sync notification rows: %w", err)
	}
	return out, nil
}

// ListReadNotificationIDs returns every notification id userID has
// already marked read, as a set — the notification list itself is
// recomputed live on every request, so this is the only state that
// actually persists.
func (s *Store) ListReadNotificationIDs(ctx context.Context, userID string) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT notification_id FROM notification_read_state WHERE user_id = $1`, userID)
	if err != nil {
		return nil, fmt.Errorf("list read notification ids for user %s: %w", userID, err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan read notification id: %w", err)
		}
		out[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate read notification ids: %w", err)
	}
	return out, nil
}

// MarkNotificationsRead upserts a read row for each of ids, owned by
// userID. Idempotent — marking an already-read id again is a no-op.
func (s *Store) MarkNotificationsRead(ctx context.Context, userID string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	now := time.Now().UTC()
	for _, id := range ids {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO notification_read_state (user_id, notification_id, read_at)
			VALUES ($1, $2, $3)
			ON CONFLICT (user_id, notification_id) DO NOTHING
		`, userID, id, now)
		if err != nil {
			return fmt.Errorf("mark notification %s read for user %s: %w", id, userID, err)
		}
	}
	return nil
}
