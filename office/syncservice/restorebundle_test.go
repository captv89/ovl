// SPDX-License-Identifier: AGPL-3.0-only

package syncservice

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	syncv1 "github.com/captv89/ovl/pkg/syncproto/gen/ovl/sync/v1"

	"github.com/captv89/ovl/office/enrollment"
	"github.com/captv89/ovl/office/store"
	"github.com/captv89/ovl/pkg/backupcrypto"
	"github.com/captv89/ovl/pkg/domain"
	"github.com/captv89/ovl/pkg/restorebundle"
)

// enrollWithDRKey mirrors what vessel/sync.Redeem does at enrollment-
// redemption time (mint a DR keypair, send the public half) — but
// directly through the store, since this test package deliberately
// doesn't depend on office/httpapi's HTTP redeem handler (same
// reasoning as mintTestCredential bypassing the enrollment-code exchange
// entirely). Returns the private half so the test can decrypt what
// FetchRestoreBundle serves.
func enrollWithDRKey(t *testing.T, st *store.Store, vesselID string) *backupcrypto.Identity {
	t.Helper()
	identity, err := backupcrypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}
	result, err := enrollment.Issue(vesselID, "master")
	if err != nil {
		t.Fatalf("enrollment.Issue: %v", err)
	}
	result.Enrollment.State = enrollment.StateEnrolled
	result.Enrollment.DRPublicKey = identity.PublicKey
	if err := st.UpsertEnrollment(context.Background(), result.Enrollment); err != nil {
		t.Fatalf("UpsertEnrollment: %v", err)
	}
	return identity
}

// TestFetchRestoreBundle_RoundTrip is architecture 12.5/11.2's DR push
// path exercised end to end at the sync-credential-authenticated RPC
// (the auto-fetch path a vessel drives itself, distinct from office/
// httpapi's Admin-facing browser-download test): once a restore command
// is queued and the vessel has a DR key on file, FetchRestoreBundle
// serves ciphertext that decrypts to a Bundle with the vessel's landed
// report data, and marks the command fetched.
func TestFetchRestoreBundle_RoundTrip(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 99)
	token := mintTestCredential(t, st, v)
	identity := enrollWithDRKey(t, st, v.ID)
	srv := newTestServer(t, st)
	client := newTestClient(srv, token)

	report := &domain.Report{
		ReportID: "report-fetch-1", VersionNo: 1, SchemaName: "log-abstract", EventType: "Departure",
		EventTime: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), State: domain.StateSubmitted,
		Fields: map[string]any{"IMO": 9074729.0},
	}
	if err := st.UpsertReportVersion(ctx, v.ID, report, "3.13", time.Now().UTC()); err != nil {
		t.Fatalf("UpsertReportVersion: %v", err)
	}

	cmdID, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("uuid.NewV7: %v", err)
	}
	if err := st.QueueRestoreCommand(ctx, &store.RestoreCommand{
		ID: cmdID.String(), VesselID: v.ID, Reason: "power outage", IssuedBy: "admin", IssuedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("QueueRestoreCommand: %v", err)
	}

	resp, err := client.FetchRestoreBundle(ctx, connect.NewRequest(&syncv1.FetchRestoreBundleRequest{CommandId: cmdID.String()}))
	if err != nil {
		t.Fatalf("FetchRestoreBundle: %v", err)
	}
	plaintext, err := backupcrypto.Decrypt(resp.Msg.GetCiphertext(), identity.PrivateKey)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	var bundle restorebundle.Bundle
	if err := json.Unmarshal(plaintext, &bundle); err != nil {
		t.Fatalf("unmarshal bundle: %v", err)
	}
	if bundle.VesselID != v.ID {
		t.Errorf("VesselID = %q, want %q", bundle.VesselID, v.ID)
	}
	if len(bundle.Reports) != 1 || bundle.Reports[0].ReportID != "report-fetch-1" {
		t.Fatalf("Reports = %+v, want exactly report-fetch-1", bundle.Reports)
	}

	list, err := st.ListRestoreCommandsForVessel(ctx, v.ID)
	if err != nil {
		t.Fatalf("ListRestoreCommandsForVessel: %v", err)
	}
	if len(list) != 1 || list[0].FetchedAt == nil {
		t.Fatalf("list = %+v, want fetched_at set after FetchRestoreBundle", list)
	}
}

func TestFetchRestoreBundle_UnknownCommandIsNotFound(t *testing.T) {
	st := openTestStore(t)
	v := createTestVessel(t, st, 82)
	token := mintTestCredential(t, st, v)
	enrollWithDRKey(t, st, v.ID)
	srv := newTestServer(t, st)
	client := newTestClient(srv, token)

	_, err := client.FetchRestoreBundle(context.Background(), connect.NewRequest(&syncv1.FetchRestoreBundleRequest{CommandId: "does-not-exist"}))
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("error = %v, want CodeNotFound", err)
	}
}

func TestFetchRestoreBundle_CommandBelongsToAnotherVesselIsNotFound(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 83)
	other := createTestVessel(t, st, 84)
	token := mintTestCredential(t, st, v)
	enrollWithDRKey(t, st, v.ID)
	srv := newTestServer(t, st)
	client := newTestClient(srv, token)

	cmdID, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("uuid.NewV7: %v", err)
	}
	if err := st.QueueRestoreCommand(ctx, &store.RestoreCommand{
		ID: cmdID.String(), VesselID: other.ID, Reason: "unrelated", IssuedBy: "admin", IssuedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("QueueRestoreCommand: %v", err)
	}

	_, err = client.FetchRestoreBundle(ctx, connect.NewRequest(&syncv1.FetchRestoreBundleRequest{CommandId: cmdID.String()}))
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("error = %v, want CodeNotFound (command belongs to a different vessel)", err)
	}
}

// TestSyncStatus_AppliedRestoreCommandIds proves the "confirming it's
// pushed" ack path: a vessel reporting applied_restore_command_ids on
// its next SyncStatus call marks those commands' applied_at.
func TestSyncStatus_AppliedRestoreCommandIds(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 85)
	token := mintTestCredential(t, st, v)
	srv := newTestServer(t, st)
	client := newTestClient(srv, token)

	cmdID, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("uuid.NewV7: %v", err)
	}
	if err := st.QueueRestoreCommand(ctx, &store.RestoreCommand{
		ID: cmdID.String(), VesselID: v.ID, Reason: "power outage", IssuedBy: "admin", IssuedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("QueueRestoreCommand: %v", err)
	}

	_, err = client.SyncStatus(ctx, connect.NewRequest(&syncv1.SyncStatusRequest{
		AppVersion: "1.0.0", AppliedRestoreCommandIds: []string{cmdID.String()},
	}))
	if err != nil {
		t.Fatalf("SyncStatus: %v", err)
	}

	list, err := st.ListRestoreCommandsForVessel(ctx, v.ID)
	if err != nil {
		t.Fatalf("ListRestoreCommandsForVessel: %v", err)
	}
	if len(list) != 1 || list[0].AppliedAt == nil {
		t.Fatalf("list = %+v, want applied_at set after SyncStatus reported it", list)
	}
}
