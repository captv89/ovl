// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"testing"
	"time"
)

func TestStore_QueueAndListRestoreCommands(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 97)

	now := time.Date(2026, 7, 20, 9, 0, 0, 0, time.UTC)
	cmd := &RestoreCommand{ID: "restore-cmd-1", VesselID: v.ID, Reason: "power outage", IssuedBy: "admin", IssuedAt: now}
	if err := st.QueueRestoreCommand(ctx, cmd); err != nil {
		t.Fatalf("QueueRestoreCommand: %v", err)
	}

	list, err := st.ListRestoreCommandsForVessel(ctx, v.ID)
	if err != nil {
		t.Fatalf("ListRestoreCommandsForVessel: %v", err)
	}
	if len(list) != 1 || list[0].ID != "restore-cmd-1" || list[0].FetchedAt != nil || list[0].AppliedAt != nil {
		t.Fatalf("list = %+v, want exactly one unfetched/unapplied restore-cmd-1", list)
	}

	got, err := st.GetRestoreCommand(ctx, "restore-cmd-1", v.ID)
	if err != nil {
		t.Fatalf("GetRestoreCommand: %v", err)
	}
	if got.Reason != "power outage" {
		t.Errorf("Reason = %q, want %q", got.Reason, "power outage")
	}

	// Scoped to vesselID: looking it up under a different vessel id must
	// behave as not-found, not leak another vessel's command.
	other := createTestVessel(t, st, 98)
	if _, err := st.GetRestoreCommand(ctx, "restore-cmd-1", other.ID); err != ErrNotFound {
		t.Errorf("GetRestoreCommand (wrong vessel) error = %v, want ErrNotFound", err)
	}

	fetchedAt := now.Add(time.Minute)
	if err := st.MarkRestoreCommandFetched(ctx, "restore-cmd-1", fetchedAt); err != nil {
		t.Fatalf("MarkRestoreCommandFetched: %v", err)
	}

	appliedAt := now.Add(2 * time.Minute)
	if err := st.MarkRestoreCommandsApplied(ctx, v.ID, []string{"restore-cmd-1", "does-not-exist"}, appliedAt); err != nil {
		t.Fatalf("MarkRestoreCommandsApplied: %v", err)
	}

	list, err = st.ListRestoreCommandsForVessel(ctx, v.ID)
	if err != nil {
		t.Fatalf("ListRestoreCommandsForVessel (after fetch+apply): %v", err)
	}
	if len(list) != 1 || list[0].FetchedAt == nil || list[0].AppliedAt == nil {
		t.Fatalf("list = %+v, want fetched_at and applied_at both set", list)
	}
	if !list[0].FetchedAt.Equal(fetchedAt) || !list[0].AppliedAt.Equal(appliedAt) {
		t.Errorf("FetchedAt/AppliedAt = %v/%v, want %v/%v", list[0].FetchedAt, list[0].AppliedAt, fetchedAt, appliedAt)
	}
}
