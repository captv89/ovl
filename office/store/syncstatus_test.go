// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/captv89/ovl/office/synccred"
)

func TestStore_UpsertAndGetVesselSyncStatus(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 34)

	if _, err := st.GetVesselSyncStatus(ctx, v.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetVesselSyncStatus (never seen) error = %v, want ErrNotFound", err)
	}

	seenAt := time.Now().UTC().Truncate(time.Microsecond)
	if err := st.UpsertVesselSyncStatus(ctx, &VesselSyncStatus{
		VesselID: v.ID, AppVersion: "1.2.3", LastSeenAt: seenAt,
		AppliedBundleID: "bundle-abc", AppliedBundleVersion: 7,
	}); err != nil {
		t.Fatalf("UpsertVesselSyncStatus: %v", err)
	}

	got, err := st.GetVesselSyncStatus(ctx, v.ID)
	if err != nil {
		t.Fatalf("GetVesselSyncStatus: %v", err)
	}
	if got.AppVersion != "1.2.3" {
		t.Errorf("AppVersion = %q, want %q", got.AppVersion, "1.2.3")
	}
	if !got.LastSeenAt.Equal(seenAt) {
		t.Errorf("LastSeenAt = %v, want %v", got.LastSeenAt, seenAt)
	}
	if got.AppliedBundleID != "bundle-abc" || got.AppliedBundleVersion != 7 {
		t.Errorf("applied bundle = %q v%d, want bundle-abc v7", got.AppliedBundleID, got.AppliedBundleVersion)
	}

	secondSeenAt := seenAt.Add(time.Hour)
	if err := st.UpsertVesselSyncStatus(ctx, &VesselSyncStatus{VesselID: v.ID, AppVersion: "1.2.4", LastSeenAt: secondSeenAt}); err != nil {
		t.Fatalf("UpsertVesselSyncStatus (second): %v", err)
	}
	got, err = st.GetVesselSyncStatus(ctx, v.ID)
	if err != nil {
		t.Fatalf("GetVesselSyncStatus (second): %v", err)
	}
	if got.AppVersion != "1.2.4" || !got.LastSeenAt.Equal(secondSeenAt) {
		t.Errorf("got = %+v, want AppVersion=1.2.4 LastSeenAt=%v (replaced in place)", got, secondSeenAt)
	}
}

func TestStore_TouchVesselCredentialLastUsed(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 35)

	result, err := synccred.Mint(v.ID)
	if err != nil {
		t.Fatalf("synccred.Mint: %v", err)
	}
	if err := st.UpsertVesselCredential(ctx, result.Credential); err != nil {
		t.Fatalf("UpsertVesselCredential: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := st.TouchVesselCredentialLastUsed(ctx, v.ID, now); err != nil {
		t.Fatalf("TouchVesselCredentialLastUsed: %v", err)
	}

	got, err := st.GetVesselCredentialByLookupHash(ctx, result.Credential.TokenLookupHash)
	if err != nil {
		t.Fatalf("GetVesselCredentialByLookupHash: %v", err)
	}
	if got.LastUsedAt == nil || !got.LastUsedAt.Equal(now) {
		t.Errorf("LastUsedAt = %v, want %v", got.LastUsedAt, now)
	}
}
