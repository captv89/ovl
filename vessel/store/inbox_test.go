// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"testing"
	"time"

	"github.com/captv89/ovl/pkg/domain"
)

func TestStore_GetInboxCursors_SeededZero(t *testing.T) {
	s := openTestStore(t)
	c, err := s.GetInboxCursors(context.Background())
	if err != nil {
		t.Fatalf("GetInboxCursors: %v", err)
	}
	if c != (InboxCursors{}) {
		t.Errorf("GetInboxCursors = %+v, want all-zero on a fresh store", c)
	}
}

func TestStore_ApplyInboxPull(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	err := s.ApplyInboxPull(ctx,
		[]PulledSchemaVersion{{SchemaName: "commercial-period", Version: "3.13", Content: []byte(`{"a":1}`), PublishedAt: time.Now().UTC()}},
		[]PulledConfigBundle{{BundleID: "bundle-1", VersionNo: 5, Content: []byte(`{"b":2}`), PublishedAt: time.Now().UTC()}},
		nil,
		nil,
		nil,
		InboxCursors{ConfigBundleCursor: 5, SchemaVersionCursor: 3},
	)
	if err != nil {
		t.Fatalf("ApplyInboxPull: %v", err)
	}

	cursors, err := s.GetInboxCursors(ctx)
	if err != nil {
		t.Fatalf("GetInboxCursors: %v", err)
	}
	if cursors.ConfigBundleCursor != 5 || cursors.SchemaVersionCursor != 3 {
		t.Errorf("cursors = %+v, want ConfigBundleCursor=5 SchemaVersionCursor=3", cursors)
	}

	var content string
	if err := s.db.QueryRowContext(ctx, `SELECT content FROM schema_versions WHERE schema_name = ? AND version = ?`, "commercial-period", "3.13").Scan(&content); err != nil {
		t.Fatalf("query schema_versions: %v", err)
	}
	if content != `{"a":1}` {
		t.Errorf("schema_versions.content = %q, want %q", content, `{"a":1}`)
	}

	var bundleVersionNo int64
	if err := s.db.QueryRowContext(ctx, `SELECT version_no FROM config_bundles WHERE bundle_id = ?`, "bundle-1").Scan(&bundleVersionNo); err != nil {
		t.Fatalf("query config_bundles: %v", err)
	}
	if bundleVersionNo != 5 {
		t.Errorf("config_bundles.version_no = %d, want 5", bundleVersionNo)
	}
}

func TestStore_LatestConfigBundle(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	// Fresh store: no bundle applied yet.
	if _, ok, err := s.LatestConfigBundle(ctx); err != nil || ok {
		t.Fatalf("LatestConfigBundle on fresh store = ok %v, err %v; want ok=false", ok, err)
	}

	// Apply v3, then v5, then re-apply v4 (out of order): the highest
	// version_no must win regardless of insertion order.
	for _, cb := range []PulledConfigBundle{
		{BundleID: "b-3", VersionNo: 3, Content: []byte(`{"wireVersion":1,"bundleId":"b-3"}`), PublishedAt: time.Now().UTC()},
		{BundleID: "b-5", VersionNo: 5, Content: []byte(`{"wireVersion":1,"bundleId":"b-5"}`), PublishedAt: time.Now().UTC()},
		{BundleID: "b-4", VersionNo: 4, Content: []byte(`{"wireVersion":1,"bundleId":"b-4"}`), PublishedAt: time.Now().UTC()},
	} {
		if err := s.ApplyConfigBundle(ctx, cb); err != nil {
			t.Fatalf("ApplyConfigBundle %s: %v", cb.BundleID, err)
		}
	}

	got, ok, err := s.LatestConfigBundle(ctx)
	if err != nil || !ok {
		t.Fatalf("LatestConfigBundle = ok %v, err %v; want ok=true", ok, err)
	}
	if got.BundleID != "b-5" || got.VersionNo != 5 {
		t.Errorf("LatestConfigBundle = %s v%d, want b-5 v5 (highest version_no wins)", got.BundleID, got.VersionNo)
	}
}

func TestStore_ApplyInboxPull_InvalidationNotices(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	r := &domain.Report{
		ReportID: "report-inv-1", VersionNo: 1, SchemaName: "log-abstract", EventType: "Departure",
		EventTime: time.Now().UTC(), Fields: map[string]any{"IMO": 1.0}, State: domain.StateSubmitted,
	}
	if err := s.SaveReport(ctx, r); err != nil {
		t.Fatalf("SaveReport: %v", err)
	}

	notice := domain.InvalidationNotice{
		ReportID: "report-inv-1", VersionNo: 1, BrokenRules: []string{"continuity.timeChain"},
		ComputedAt: time.Date(2026, 7, 12, 9, 0, 0, 0, time.UTC),
	}
	if err := s.ApplyInboxPull(ctx, nil, nil, nil, []domain.InvalidationNotice{notice}, nil, InboxCursors{InvalidationNoticeCursor: 5}); err != nil {
		t.Fatalf("ApplyInboxPull: %v", err)
	}

	got, err := s.GetLatestVersion(ctx, "report-inv-1")
	if err != nil {
		t.Fatalf("GetLatestVersion: %v", err)
	}
	if got.State != domain.StateInvalidated {
		t.Errorf("State = %q, want %q", got.State, domain.StateInvalidated)
	}
	if got.InvalidatedFrom != domain.StateSubmitted {
		t.Errorf("InvalidatedFrom = %q, want %q", got.InvalidatedFrom, domain.StateSubmitted)
	}
	if len(got.InvalidatedRules) != 1 || got.InvalidatedRules[0] != "continuity.timeChain" {
		t.Errorf("InvalidatedRules = %v, want [continuity.timeChain]", got.InvalidatedRules)
	}

	events, err := s.ListEvents(ctx, "report-inv-1")
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events) != 1 || events[0].Type != domain.EventInvalidated {
		t.Fatalf("events = %+v, want exactly one invalidated event", events)
	}

	cursors, err := s.GetInboxCursors(ctx)
	if err != nil {
		t.Fatalf("GetInboxCursors: %v", err)
	}
	if cursors.InvalidationNoticeCursor != 5 {
		t.Errorf("InvalidationNoticeCursor = %d, want 5", cursors.InvalidationNoticeCursor)
	}

	notices, err := s.ListInvalidationNotices(ctx, "report-inv-1")
	if err != nil {
		t.Fatalf("ListInvalidationNotices: %v", err)
	}
	if len(notices) != 1 {
		t.Errorf("len(notices) = %d, want 1", len(notices))
	}

	// Re-pulling the same notice (cursor not yet advanced, e.g. a dropped
	// link) must not duplicate the audit event.
	if err := s.ApplyInboxPull(ctx, nil, nil, nil, []domain.InvalidationNotice{notice}, nil, InboxCursors{InvalidationNoticeCursor: 5}); err != nil {
		t.Fatalf("ApplyInboxPull (retransmit): %v", err)
	}
	events, err = s.ListEvents(ctx, "report-inv-1")
	if err != nil {
		t.Fatalf("ListEvents (after retransmit): %v", err)
	}
	if len(events) != 1 {
		t.Errorf("len(events) after retransmit = %d, want still 1 (no duplicate)", len(events))
	}
}

func TestStore_ApplyInboxPull_Remarks(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	r := &domain.Report{
		ReportID: "report-remark-1", VersionNo: 1, SchemaName: "log-abstract", EventType: "Departure",
		EventTime: time.Now().UTC(), Fields: map[string]any{"IMO": 1.0}, State: domain.StateSubmitted,
	}
	if err := s.SaveReport(ctx, r); err != nil {
		t.Fatalf("SaveReport: %v", err)
	}

	remark := domain.Remark{
		ID: "remark-pulled-1", ReportID: "report-remark-1", VersionNo: 1, FieldName: "Cargo_Mt",
		Body: "please double-check", Author: "reviewer1", CreatedAt: time.Date(2026, 7, 12, 9, 0, 0, 0, time.UTC),
	}
	if err := s.ApplyInboxPull(ctx, nil, nil, nil, nil, []domain.Remark{remark}, InboxCursors{RemarkCursor: 4}); err != nil {
		t.Fatalf("ApplyInboxPull: %v", err)
	}

	got, err := s.GetLatestVersion(ctx, "report-remark-1")
	if err != nil {
		t.Fatalf("GetLatestVersion: %v", err)
	}
	if got.State != domain.StateRemarked {
		t.Errorf("State = %q, want %q", got.State, domain.StateRemarked)
	}

	events, err := s.ListEvents(ctx, "report-remark-1")
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events) != 1 || events[0].Type != domain.EventRemarked {
		t.Fatalf("events = %+v, want exactly one remarked event", events)
	}

	cursors, err := s.GetInboxCursors(ctx)
	if err != nil {
		t.Fatalf("GetInboxCursors: %v", err)
	}
	if cursors.RemarkCursor != 4 {
		t.Errorf("RemarkCursor = %d, want 4", cursors.RemarkCursor)
	}

	remarks, err := s.ListRemarks(ctx, "report-remark-1")
	if err != nil {
		t.Fatalf("ListRemarks: %v", err)
	}
	if len(remarks) != 1 {
		t.Errorf("len(remarks) = %d, want 1", len(remarks))
	}

	// Re-pulling the same remark (cursor not yet advanced) must not
	// duplicate the remarked audit event.
	if err := s.ApplyInboxPull(ctx, nil, nil, nil, nil, []domain.Remark{remark}, InboxCursors{RemarkCursor: 4}); err != nil {
		t.Fatalf("ApplyInboxPull (retransmit): %v", err)
	}
	events, err = s.ListEvents(ctx, "report-remark-1")
	if err != nil {
		t.Fatalf("ListEvents (after retransmit): %v", err)
	}
	if len(events) != 1 {
		t.Errorf("len(events) after retransmit = %d, want still 1 (no duplicate)", len(events))
	}
}

func TestStore_ApplyInboxPull_ChatMessages(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	msg := domain.ChatMessage{
		ID: "chat-pulled-1", ReportID: "report-1", Sender: "reviewer1", Body: "thanks, looks good now",
		SentAt: time.Date(2026, 7, 12, 9, 5, 0, 0, time.UTC), Direction: domain.ChatFromOffice,
	}
	if err := s.ApplyInboxPull(ctx, nil, nil, []domain.ChatMessage{msg}, nil, nil, InboxCursors{ChatCursor: 7}); err != nil {
		t.Fatalf("ApplyInboxPull: %v", err)
	}

	cursors, err := s.GetInboxCursors(ctx)
	if err != nil {
		t.Fatalf("GetInboxCursors: %v", err)
	}
	if cursors.ChatCursor != 7 {
		t.Errorf("ChatCursor = %d, want 7", cursors.ChatCursor)
	}

	got, err := s.ListChatMessages(ctx, "report-1")
	if err != nil {
		t.Fatalf("ListChatMessages: %v", err)
	}
	if len(got) != 1 || got[0].ID != "chat-pulled-1" || got[0].Direction != domain.ChatFromOffice {
		t.Errorf("got = %+v, want the pulled office message", got)
	}
}

func TestStore_ApplyInboxPull_ChatMessages_IdempotentOnRepull(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	msg := domain.ChatMessage{
		ID: "chat-pulled-2", ReportID: "report-1", Sender: "reviewer1", Body: "hello",
		SentAt: time.Now().UTC(), Direction: domain.ChatFromOffice,
	}
	if err := s.ApplyInboxPull(ctx, nil, nil, []domain.ChatMessage{msg}, nil, nil, InboxCursors{ChatCursor: 1}); err != nil {
		t.Fatalf("ApplyInboxPull (first): %v", err)
	}
	// Retransmission after a dropped link: same message, applied again.
	if err := s.ApplyInboxPull(ctx, nil, nil, []domain.ChatMessage{msg}, nil, nil, InboxCursors{ChatCursor: 1}); err != nil {
		t.Fatalf("ApplyInboxPull (retransmit): %v", err)
	}
	got, err := s.ListChatMessages(ctx, "report-1")
	if err != nil {
		t.Fatalf("ListChatMessages: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("len(got) = %d, want 1 (no duplicate)", len(got))
	}
}

func TestStore_ApplyInboxPull_RetransmissionIsANoOp(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	sv := PulledSchemaVersion{SchemaName: "commercial-period", Version: "3.13", Content: []byte(`{"a":1}`), PublishedAt: time.Now().UTC()}
	if err := s.ApplyInboxPull(ctx, []PulledSchemaVersion{sv}, nil, nil, nil, nil, InboxCursors{SchemaVersionCursor: 1}); err != nil {
		t.Fatalf("ApplyInboxPull (first): %v", err)
	}
	// Same content pulled again (e.g. cursor didn't advance for some
	// other reason) must not error or duplicate.
	if err := s.ApplyInboxPull(ctx, []PulledSchemaVersion{sv}, nil, nil, nil, nil, InboxCursors{SchemaVersionCursor: 1}); err != nil {
		t.Fatalf("ApplyInboxPull (retransmit): %v", err)
	}

	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_versions WHERE schema_name = ? AND version = ?`, "commercial-period", "3.13").Scan(&count); err != nil {
		t.Fatalf("count schema_versions: %v", err)
	}
	if count != 1 {
		t.Errorf("schema_versions row count = %d, want 1 (no duplicate)", count)
	}
}
