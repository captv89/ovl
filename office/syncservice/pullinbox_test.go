// SPDX-License-Identifier: AGPL-3.0-only

package syncservice

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	syncv1 "github.com/captv89/ovl/pkg/syncproto/gen/ovl/sync/v1"

	"github.com/captv89/ovl/office/compliance"
	"github.com/captv89/ovl/office/configbundle"
	"github.com/captv89/ovl/office/schemaversions"
	"github.com/captv89/ovl/office/store"
	"github.com/captv89/ovl/pkg/configwire"
	"github.com/captv89/ovl/pkg/domain"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// publishTestSchemaVersion publishes a schema version with a version
// string unique per call (not just per test name) — schema versions
// have no delete path at the store layer (architecture 5.2's
// immutability, matched by office/store having no DeleteSchemaVersion),
// so repeated runs against the same shared Postgres instance need a
// fresh (schema_name, version) pair every time rather than relying on
// cleanup.
func publishTestSchemaVersion(t *testing.T, st interface {
	CreateSchemaVersion(ctx context.Context, sv *schemaversions.SchemaVersion) error
}, schemaName string) *schemaversions.SchemaVersion {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("uuid.NewV7: %v", err)
	}
	sv, err := schemaversions.Publish(schemaName, "3.13-"+id.String(), schemaversions.SourceProjectCurated, []byte(`{"schemaName":"`+schemaName+`"}`), "user-1")
	if err != nil {
		t.Fatalf("schemaversions.Publish: %v", err)
	}
	if err := st.CreateSchemaVersion(context.Background(), sv); err != nil {
		t.Fatalf("CreateSchemaVersion: %v", err)
	}
	return sv
}

func TestPullInbox_SchemaVersionsGlobalStream(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 90)
	token := mintTestCredential(t, st, v)
	srv := newTestServer(t, st)
	client := newTestClient(srv, token)

	sv := publishTestSchemaVersion(t, st, "commercial-period")

	resp, err := client.PullInbox(ctx, connect.NewRequest(&syncv1.PullInboxRequest{Cursors: &syncv1.SyncCursors{}}))
	if err != nil {
		t.Fatalf("PullInbox: %v", err)
	}
	var found bool
	for _, pv := range resp.Msg.GetSchemaVersions() {
		if pv.GetVersion() == sv.Version && pv.GetSchemaKind() == syncv1.ReportSchemaKind_REPORT_SCHEMA_KIND_COMMERCIAL_PERIOD {
			found = true
			if string(pv.GetSchemaJson()) != string(sv.Content) {
				t.Errorf("SchemaJson = %q, want %q", pv.GetSchemaJson(), sv.Content)
			}
		}
	}
	if !found {
		t.Fatal("published schema version not present in PullInbox response")
	}
	if resp.Msg.GetNextCursors().GetSchemaVersionCursor() <= 0 {
		t.Error("NextCursors.SchemaVersionCursor <= 0 after pulling at least one version")
	}

	// A second pull starting from the advanced cursor gets nothing new.
	resp2, err := client.PullInbox(ctx, connect.NewRequest(&syncv1.PullInboxRequest{
		Cursors: &syncv1.SyncCursors{SchemaVersionCursor: resp.Msg.GetNextCursors().GetSchemaVersionCursor()},
	}))
	if err != nil {
		t.Fatalf("PullInbox (second): %v", err)
	}
	for _, pv := range resp2.Msg.GetSchemaVersions() {
		if pv.GetVersion() == sv.Version {
			t.Error("second pull with the advanced cursor still returned the already-pulled version")
		}
	}
}

func TestPullInbox_AssignedConfigBundle(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 91)
	token := mintTestCredential(t, st, v)
	srv := newTestServer(t, st)
	client := newTestClient(srv, token)

	bundle := configbundle.Compose(nil, nil, nil, nil, nil, []string{"master"})
	published, err := configbundle.Publish(bundle, "Test Bundle", "user-1")
	if err != nil {
		t.Fatalf("configbundle.Publish: %v", err)
	}
	if err := st.CreateConfigBundle(ctx, published); err != nil {
		t.Fatalf("CreateConfigBundle: %v", err)
	}
	// No cleanup: config bundles are permanent/immutable by design
	// (architecture 6.5, office/store has no delete path for them) —
	// same reasoning other tests already rely on.

	vesselScope, err := compliance.VesselScope(v.ID)
	if err != nil {
		t.Fatalf("VesselScope: %v", err)
	}
	assignment, err := configbundle.NewBundleAssignment(vesselScope, published.ID)
	if err != nil {
		t.Fatalf("NewBundleAssignment: %v", err)
	}
	if err := st.SaveBundleAssignment(ctx, assignment); err != nil {
		t.Fatalf("SaveBundleAssignment: %v", err)
	}

	resp, err := client.PullInbox(ctx, connect.NewRequest(&syncv1.PullInboxRequest{Cursors: &syncv1.SyncCursors{}}))
	if err != nil {
		t.Fatalf("PullInbox: %v", err)
	}
	bundles := resp.Msg.GetConfigBundles()
	if len(bundles) != 1 {
		t.Fatalf("len(ConfigBundles) = %d, want 1", len(bundles))
	}
	if bundles[0].GetBundleId() != published.ID {
		t.Errorf("BundleId = %q, want %q", bundles[0].GetBundleId(), published.ID)
	}
	// ContentJson is now the tagged, resolved-for-this-vessel wire shape
	// (configwire.Bundle), not a raw json.Marshal of the office struct.
	decoded, err := configwire.Decode(bundles[0].GetContentJson())
	if err != nil {
		t.Fatalf("configwire.Decode ContentJson: %v", err)
	}
	if decoded.BundleID != published.ID || len(decoded.DefaultRoleNames) != 1 || decoded.DefaultRoleNames[0] != "master" {
		t.Errorf("decoded bundle = %+v, want BundleID=%q DefaultRoleNames=[master]", decoded, published.ID)
	}
	if decoded.VersionNo != bundles[0].GetVersionNo() {
		t.Errorf("decoded VersionNo = %d, want %d", decoded.VersionNo, bundles[0].GetVersionNo())
	}
	nextCursor := resp.Msg.GetNextCursors().GetConfigBundleCursor()
	if nextCursor != bundles[0].GetVersionNo() {
		t.Errorf("NextCursors.ConfigBundleCursor = %d, want %d (the bundle's own VersionNo)", nextCursor, bundles[0].GetVersionNo())
	}

	// A second pull from the advanced cursor gets no bundle.
	resp2, err := client.PullInbox(ctx, connect.NewRequest(&syncv1.PullInboxRequest{
		Cursors: &syncv1.SyncCursors{ConfigBundleCursor: nextCursor},
	}))
	if err != nil {
		t.Fatalf("PullInbox (second): %v", err)
	}
	if len(resp2.Msg.GetConfigBundles()) != 0 {
		t.Errorf("second pull with the advanced cursor returned %d bundles, want 0", len(resp2.Msg.GetConfigBundles()))
	}
}

func TestPullInbox_NoAssignedBundle(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 92)
	token := mintTestCredential(t, st, v)
	srv := newTestServer(t, st)
	client := newTestClient(srv, token)

	resp, err := client.PullInbox(ctx, connect.NewRequest(&syncv1.PullInboxRequest{Cursors: &syncv1.SyncCursors{}}))
	if err != nil {
		t.Fatalf("PullInbox: %v", err)
	}
	if len(resp.Msg.GetConfigBundles()) != 0 {
		t.Errorf("ConfigBundles for an unassigned vessel = %v, want empty", resp.Msg.GetConfigBundles())
	}
}

// TestPullInbox_ChatMessagesVesselScoped covers T3.3: a vessel must only
// ever pull office-authored chat messages (never its own, echoed back),
// scoped to itself (never another vessel's), and the cursor must
// advance so a second pull doesn't return the same message again.
func TestPullInbox_ChatMessagesVesselScoped(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 92)
	other := createTestVessel(t, st, 93)
	token := mintTestCredential(t, st, v)
	srv := newTestServer(t, st)
	client := newTestClient(srv, token)

	if err := st.InsertChatMessage(ctx, v.ID, domain.ChatMessage{
		ID: "chat-pull-vessel", ReportID: "report-1", Sender: "master", Body: "from vessel",
		SentAt: time.Now().UTC(), Direction: domain.ChatFromVessel,
	}); err != nil {
		t.Fatalf("InsertChatMessage (vessel-direction): %v", err)
	}
	if err := st.InsertChatMessage(ctx, v.ID, domain.ChatMessage{
		ID: "chat-pull-office", ReportID: "report-1", Sender: "reviewer1", Body: "from office",
		SentAt: time.Now().UTC(), Direction: domain.ChatFromOffice,
	}); err != nil {
		t.Fatalf("InsertChatMessage (office-direction): %v", err)
	}
	if err := st.InsertChatMessage(ctx, other.ID, domain.ChatMessage{
		ID: "chat-pull-other-vessel", ReportID: "report-1", Sender: "reviewer1", Body: "for the other vessel",
		SentAt: time.Now().UTC(), Direction: domain.ChatFromOffice,
	}); err != nil {
		t.Fatalf("InsertChatMessage (other vessel): %v", err)
	}

	resp, err := client.PullInbox(ctx, connect.NewRequest(&syncv1.PullInboxRequest{Cursors: &syncv1.SyncCursors{}}))
	if err != nil {
		t.Fatalf("PullInbox: %v", err)
	}
	if len(resp.Msg.GetChatMessages()) != 1 || resp.Msg.GetChatMessages()[0].GetId() != "chat-pull-office" {
		t.Fatalf("ChatMessages = %+v, want exactly chat-pull-office", resp.Msg.GetChatMessages())
	}
	if resp.Msg.GetNextCursors().GetChatCursor() <= 0 {
		t.Error("NextCursors.ChatCursor <= 0 after pulling at least one message")
	}

	resp2, err := client.PullInbox(ctx, connect.NewRequest(&syncv1.PullInboxRequest{
		Cursors: &syncv1.SyncCursors{ChatCursor: resp.Msg.GetNextCursors().GetChatCursor()},
	}))
	if err != nil {
		t.Fatalf("PullInbox (second): %v", err)
	}
	if len(resp2.Msg.GetChatMessages()) != 0 {
		t.Errorf("second pull with the advanced cursor still returned messages: %+v", resp2.Msg.GetChatMessages())
	}
}

// TestPullInbox_InvalidationNoticesVesselScoped covers T4.2's office
// half: notices scoped to the calling vessel only, and the cursor must
// advance so a second pull doesn't return the same notice again.
func TestPullInbox_InvalidationNoticesVesselScoped(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 94)
	other := createTestVessel(t, st, 95)
	token := mintTestCredential(t, st, v)
	srv := newTestServer(t, st)
	client := newTestClient(srv, token)

	if err := st.InsertInvalidationNotice(ctx, v.ID, "report-inv-1", 1, []string{"continuity.timeChain"}, time.Now().UTC()); err != nil {
		t.Fatalf("InsertInvalidationNotice: %v", err)
	}
	if err := st.InsertInvalidationNotice(ctx, other.ID, "report-inv-2", 1, []string{"continuity.timeChain"}, time.Now().UTC()); err != nil {
		t.Fatalf("InsertInvalidationNotice (other vessel): %v", err)
	}

	resp, err := client.PullInbox(ctx, connect.NewRequest(&syncv1.PullInboxRequest{Cursors: &syncv1.SyncCursors{}}))
	if err != nil {
		t.Fatalf("PullInbox: %v", err)
	}
	if len(resp.Msg.GetInvalidationNotices()) != 1 || resp.Msg.GetInvalidationNotices()[0].GetReportId() != "report-inv-1" {
		t.Fatalf("InvalidationNotices = %+v, want exactly report-inv-1's notice", resp.Msg.GetInvalidationNotices())
	}
	if resp.Msg.GetNextCursors().GetInvalidationNoticeCursor() <= 0 {
		t.Error("NextCursors.InvalidationNoticeCursor <= 0 after pulling at least one notice")
	}

	resp2, err := client.PullInbox(ctx, connect.NewRequest(&syncv1.PullInboxRequest{
		Cursors: &syncv1.SyncCursors{InvalidationNoticeCursor: resp.Msg.GetNextCursors().GetInvalidationNoticeCursor()},
	}))
	if err != nil {
		t.Fatalf("PullInbox (second): %v", err)
	}
	if len(resp2.Msg.GetInvalidationNotices()) != 0 {
		t.Errorf("second pull with the advanced cursor still returned notices: %+v", resp2.Msg.GetInvalidationNotices())
	}
}

// TestPullInbox_RemarksVesselScoped covers T5.2's office pull half:
// remarks scoped to the calling vessel only, cursor advances so a
// second pull doesn't return the same remark again.
func TestPullInbox_RemarksVesselScoped(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 96)
	other := createTestVessel(t, st, 97)
	token := mintTestCredential(t, st, v)
	srv := newTestServer(t, st)
	client := newTestClient(srv, token)

	remark := domain.Remark{ID: "remark-pull-1", ReportID: "report-1", VersionNo: 1, FieldName: "Cargo_Mt", Body: "check", Author: "reviewer1", CreatedAt: time.Now().UTC()}
	if err := st.InsertRemarkSet(ctx, v.ID, []domain.Remark{remark}, "set-pull-1"); err != nil {
		t.Fatalf("InsertRemarkSet: %v", err)
	}
	otherRemark := domain.Remark{ID: "remark-pull-2", ReportID: "report-2", VersionNo: 1, FieldName: "Cargo_Mt", Body: "check", Author: "reviewer1", CreatedAt: time.Now().UTC()}
	if err := st.InsertRemarkSet(ctx, other.ID, []domain.Remark{otherRemark}, "set-pull-2"); err != nil {
		t.Fatalf("InsertRemarkSet (other vessel): %v", err)
	}

	resp, err := client.PullInbox(ctx, connect.NewRequest(&syncv1.PullInboxRequest{Cursors: &syncv1.SyncCursors{}}))
	if err != nil {
		t.Fatalf("PullInbox: %v", err)
	}
	if len(resp.Msg.GetRemarks()) != 1 || resp.Msg.GetRemarks()[0].GetId() != "remark-pull-1" {
		t.Fatalf("Remarks = %+v, want exactly remark-pull-1", resp.Msg.GetRemarks())
	}
	if resp.Msg.GetNextCursors().GetRemarkCursor() <= 0 {
		t.Error("NextCursors.RemarkCursor <= 0 after pulling at least one remark")
	}

	resp2, err := client.PullInbox(ctx, connect.NewRequest(&syncv1.PullInboxRequest{
		Cursors: &syncv1.SyncCursors{RemarkCursor: resp.Msg.GetNextCursors().GetRemarkCursor()},
	}))
	if err != nil {
		t.Fatalf("PullInbox (second): %v", err)
	}
	if len(resp2.Msg.GetRemarks()) != 0 {
		t.Errorf("second pull with the advanced cursor still returned remarks: %+v", resp2.Msg.GetRemarks())
	}
}

// TestPullInbox_RestoreCommands is architecture 12.5's DR push path
// exercised at PullInbox: a queued restore command is delivered to the
// vessel it was issued for (and not to another vessel), cursor-scoped
// like remarks/chat, with the cursor advancing so a second pull sees
// nothing new — office/store.QueueRestoreCommand is B2's DR tab "push to
// vessel" action; this proves the store.MarkRestoreCommandsApplied's
// ANY($3) array param (used elsewhere) isn't needed for the pull side,
// only for SyncStatus's ack path (office/syncservice_test.go).
func TestPullInbox_RestoreCommands(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 95)
	other := createTestVessel(t, st, 96)
	token := mintTestCredential(t, st, v)
	srv := newTestServer(t, st)
	client := newTestClient(srv, token)

	cmdID, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("uuid.NewV7: %v", err)
	}
	if err := st.QueueRestoreCommand(ctx, &store.RestoreCommand{
		ID: cmdID.String(), VesselID: v.ID, Reason: "power outage — vessel re-enrolled", IssuedBy: "admin", IssuedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("QueueRestoreCommand: %v", err)
	}
	otherCmdID, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("uuid.NewV7: %v", err)
	}
	if err := st.QueueRestoreCommand(ctx, &store.RestoreCommand{
		ID: otherCmdID.String(), VesselID: other.ID, Reason: "unrelated", IssuedBy: "admin", IssuedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("QueueRestoreCommand (other vessel): %v", err)
	}

	resp, err := client.PullInbox(ctx, connect.NewRequest(&syncv1.PullInboxRequest{Cursors: &syncv1.SyncCursors{}}))
	if err != nil {
		t.Fatalf("PullInbox: %v", err)
	}
	if len(resp.Msg.GetRestoreCommands()) != 1 || resp.Msg.GetRestoreCommands()[0].GetCommandId() != cmdID.String() {
		t.Fatalf("RestoreCommands = %+v, want exactly %s", resp.Msg.GetRestoreCommands(), cmdID.String())
	}
	if resp.Msg.GetNextCursors().GetRestoreCommandCursor() <= 0 {
		t.Error("NextCursors.RestoreCommandCursor <= 0 after pulling at least one restore command")
	}

	resp2, err := client.PullInbox(ctx, connect.NewRequest(&syncv1.PullInboxRequest{
		Cursors: &syncv1.SyncCursors{RestoreCommandCursor: resp.Msg.GetNextCursors().GetRestoreCommandCursor()},
	}))
	if err != nil {
		t.Fatalf("PullInbox (second): %v", err)
	}
	if len(resp2.Msg.GetRestoreCommands()) != 0 {
		t.Errorf("second pull with the advanced cursor still returned restore commands: %+v", resp2.Msg.GetRestoreCommands())
	}
}

// TestPullInbox_UserCommands is architecture 9.3/12.4's remote vessel-
// user-administration path exercised at PullInbox: a queued command is
// delivered to the vessel it was issued for (and not to another
// vessel), the full payload (including the plaintext temporary
// password) travels in this same response (no separate fetch step,
// unlike restore commands), and it's marked fetched immediately —
// office/store.QueueUserCommand is B2's Users tab action.
func TestPullInbox_UserCommands(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 89)
	other := createTestVessel(t, st, 87)
	token := mintTestCredential(t, st, v)
	srv := newTestServer(t, st)
	client := newTestClient(srv, token)

	if err := st.QueueUserCommand(ctx, &store.UserCommand{
		ID: "user-cmd-pull-1", VesselID: v.ID, Action: "resetPassword", Username: "second-officer",
		TemporaryPassword: "correct-horse-battery-staple", IssuedBy: "admin", IssuedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("QueueUserCommand: %v", err)
	}
	if err := st.QueueUserCommand(ctx, &store.UserCommand{
		ID: "user-cmd-pull-other", VesselID: other.ID, Action: "resetPassword", Username: "second-officer",
		TemporaryPassword: "unrelated", IssuedBy: "admin", IssuedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("QueueUserCommand (other vessel): %v", err)
	}

	resp, err := client.PullInbox(ctx, connect.NewRequest(&syncv1.PullInboxRequest{Cursors: &syncv1.SyncCursors{}}))
	if err != nil {
		t.Fatalf("PullInbox: %v", err)
	}
	got := resp.Msg.GetUserCommands()
	if len(got) != 1 || got[0].GetCommandId() != "user-cmd-pull-1" {
		t.Fatalf("UserCommands = %+v, want exactly user-cmd-pull-1", got)
	}
	if got[0].GetAction() != syncv1.UserCommandAction_USER_COMMAND_ACTION_RESET_PASSWORD {
		t.Errorf("Action = %v, want RESET_PASSWORD", got[0].GetAction())
	}
	if got[0].GetTemporaryPassword() != "correct-horse-battery-staple" {
		t.Errorf("TemporaryPassword = %q, want the plaintext to travel with the command", got[0].GetTemporaryPassword())
	}
	if resp.Msg.GetNextCursors().GetUserCommandCursor() <= 0 {
		t.Error("NextCursors.UserCommandCursor <= 0 after pulling at least one user command")
	}

	list, err := st.ListUserCommandsForVessel(ctx, v.ID)
	if err != nil {
		t.Fatalf("ListUserCommandsForVessel: %v", err)
	}
	if len(list) != 1 || list[0].FetchedAt == nil {
		t.Fatalf("list = %+v, want fetched_at set immediately after PullInbox delivers it", list)
	}

	resp2, err := client.PullInbox(ctx, connect.NewRequest(&syncv1.PullInboxRequest{
		Cursors: &syncv1.SyncCursors{UserCommandCursor: resp.Msg.GetNextCursors().GetUserCommandCursor()},
	}))
	if err != nil {
		t.Fatalf("PullInbox (second): %v", err)
	}
	if len(resp2.Msg.GetUserCommands()) != 0 {
		t.Errorf("second pull with the advanced cursor still returned user commands: %+v", resp2.Msg.GetUserCommands())
	}
}

// TestSyncStatus_ReportsUserRosterAndAcksAppliedCommands covers the
// other half of architecture 9.3/12.4's remote administration: the
// vessel's reported roster lands in office's mirror (replacing whatever
// was there before), and applied_user_command_ids marks the
// corresponding queued commands applied.
func TestSyncStatus_ReportsUserRosterAndAcksAppliedCommands(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 86)
	token := mintTestCredential(t, st, v)
	srv := newTestServer(t, st)
	client := newTestClient(srv, token)

	if err := st.QueueUserCommand(ctx, &store.UserCommand{
		ID: "user-cmd-status-1", VesselID: v.ID, Action: "resetPassword", Username: "second-officer",
		TemporaryPassword: "x", IssuedBy: "admin", IssuedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("QueueUserCommand: %v", err)
	}

	_, err := client.SyncStatus(ctx, connect.NewRequest(&syncv1.SyncStatusRequest{
		AppVersion:            "1.0.0",
		AppliedUserCommandIds: []string{"user-cmd-status-1"},
		Users: []*syncv1.VesselUserSummary{
			{Username: "master", Role: "master", Active: true, CanSubmit: true, UpdatedAt: timestamppb.New(time.Now().UTC())},
			{Username: "second-officer", Role: "secondOfficer", Active: true, CanSubmit: false, UpdatedAt: timestamppb.New(time.Now().UTC())},
		},
	}))
	if err != nil {
		t.Fatalf("SyncStatus: %v", err)
	}

	roster, err := st.ListVesselUsers(ctx, v.ID)
	if err != nil {
		t.Fatalf("ListVesselUsers: %v", err)
	}
	if len(roster) != 2 || roster[0].Username != "master" || roster[1].Username != "second-officer" {
		t.Fatalf("roster = %+v, want [master, second-officer]", roster)
	}

	commands, err := st.ListUserCommandsForVessel(ctx, v.ID)
	if err != nil {
		t.Fatalf("ListUserCommandsForVessel: %v", err)
	}
	if len(commands) != 1 || commands[0].AppliedAt == nil {
		t.Fatalf("commands = %+v, want applied_at set", commands)
	}
}

func TestPullInbox_MissingCredential(t *testing.T) {
	st := openTestStore(t)
	srv := newTestServer(t, st)
	client := newTestClient(srv, "")

	_, err := client.PullInbox(context.Background(), connect.NewRequest(&syncv1.PullInboxRequest{}))
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("PullInbox (no credential) error = %v, want CodeUnauthenticated", err)
	}
}
