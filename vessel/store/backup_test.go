// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestStore_SnapshotTo(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	r := newTestReport(t)
	if err := s.SaveReport(ctx, r); err != nil {
		t.Fatalf("SaveReport: %v", err)
	}

	snapDir := t.TempDir()
	snapPath := filepath.Join(snapDir, "nested", "ovl.db")
	if err := s.SnapshotTo(ctx, snapPath); err != nil {
		t.Fatalf("SnapshotTo: %v", err)
	}

	// The snapshot is a real, independently-openable database (Open just
	// applies goose migrations to whatever's there, which is a no-op on an
	// already-migrated db).
	snapStore, err := Open(filepath.Dir(snapPath))
	if err != nil {
		t.Fatalf("Open snapshot: %v", err)
	}
	defer func() { _ = snapStore.Close() }()

	got, err := snapStore.GetReport(ctx, r.ReportID, r.VersionNo)
	if err != nil {
		t.Fatalf("GetReport from snapshot: %v", err)
	}
	if got.ReportID != r.ReportID {
		t.Errorf("snapshot's report = %+v, want matching %s", got, r.ReportID)
	}
}

func TestRestoreDatabase_RevertsToSnapshotPoint(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	s, err := Open(dataDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	before := newTestReport(t)
	if err := s.SaveReport(ctx, before); err != nil {
		t.Fatalf("SaveReport(before): %v", err)
	}

	snapPath := filepath.Join(t.TempDir(), "ovl.db")
	if err := s.SnapshotTo(ctx, snapPath); err != nil {
		t.Fatalf("SnapshotTo: %v", err)
	}

	// Written after the snapshot — restoring must make this disappear.
	after := newTestReport(t)
	after.ReportID = "after-report-id"
	if err := s.SaveReport(ctx, after); err != nil {
		t.Fatalf("SaveReport(after): %v", err)
	}

	// RestoreDatabase's own doc comment requires the Store to be closed
	// first — SQLite must not have the destination file open while it's
	// replaced.
	if err := s.Close(); err != nil {
		t.Fatalf("Close before restore: %v", err)
	}

	if err := RestoreDatabase(dataDir, snapPath); err != nil {
		t.Fatalf("RestoreDatabase: %v", err)
	}

	restored, err := Open(dataDir)
	if err != nil {
		t.Fatalf("Open after restore: %v", err)
	}
	defer func() { _ = restored.Close() }()

	if _, err := restored.GetReport(ctx, before.ReportID, before.VersionNo); err != nil {
		t.Errorf("GetReport(before) after restore: %v, want it to still exist", err)
	}
	if _, err := restored.GetReport(ctx, after.ReportID, after.VersionNo); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetReport(after) after restore: err = %v, want ErrNotFound (restore should have reverted it)", err)
	}
}

// AttachmentStore.CopyAllTo's own behavior (including the "missing
// BaseDir is a no-op" case) is tested where that type now lives:
// pkg/attachmentstore/attachmentstore_test.go.
