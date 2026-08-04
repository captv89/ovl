// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"

	syncv1 "github.com/captv89/ovl/pkg/syncproto/gen/ovl/sync/v1"
	"github.com/captv89/ovl/pkg/syncproto/gen/ovl/sync/v1/syncv1connect"

	"github.com/captv89/ovl/pkg/backupcrypto"
	"github.com/captv89/ovl/pkg/configwire"
	"github.com/captv89/ovl/pkg/domain"
	"github.com/captv89/ovl/pkg/restorebundle"
	"github.com/captv89/ovl/pkg/syncproto"
	"github.com/captv89/ovl/pkg/validation"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// fakeOfficeSyncServer stands in for office/syncservice's real handler —
// vessel code never imports office code, even in tests (see
// vessel/sync/client_test.go's own fakeSyncServer for the same
// reasoning). This only needs to prove vessel/httpapi's sync endpoints
// drive vessel/sync.Client correctly end to end.
type fakeOfficeSyncServer struct {
	syncv1connect.UnimplementedSyncServiceHandler
	wantCredential string
	serverTime     time.Time
	callCount      int

	pushedItems []*syncv1.OutboxItem

	// pullResponse, if set, is returned verbatim by PullInbox instead of
	// the default cursor-echo — lets a test simulate the office having
	// something new to deliver.
	pullResponse *syncv1.PullInboxResponse

	receivedAttachmentChunks int

	// fetchRestoreBundleCiphertext, if set, is what FetchRestoreBundle
	// serves regardless of the requested command_id — architecture 12.5's
	// DR push path (2026-07-20).
	fetchRestoreBundleCiphertext []byte
	fetchedRestoreCommandIDs     []string
	gotAppliedRestoreCommandIDs  []string

	// gotAppliedUserCommandIDs/gotUsers capture architecture 9.3/12.4's
	// remote user administration ack/roster fields (2026-07-21) — same
	// reasoning as the restore-command fields above.
	gotAppliedUserCommandIDs []string
	gotUsers                 []*syncv1.VesselUserSummary
}

func (f *fakeOfficeSyncServer) SyncStatus(_ context.Context, req *connect.Request[syncv1.SyncStatusRequest]) (*connect.Response[syncv1.SyncStatusResponse], error) {
	f.callCount++
	if got := req.Header().Get("Authorization"); got != "Bearer "+f.wantCredential {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("wrong credential"))
	}
	f.gotAppliedRestoreCommandIDs = req.Msg.GetAppliedRestoreCommandIds()
	f.gotAppliedUserCommandIDs = req.Msg.GetAppliedUserCommandIds()
	f.gotUsers = req.Msg.GetUsers()
	return connect.NewResponse(&syncv1.SyncStatusResponse{ServerTime: timestamppb.New(f.serverTime)}), nil
}

// FetchRestoreBundle serves fetchRestoreBundleCiphertext verbatim — real
// bundle-building/encryption is office/syncservice's own job, already
// covered by its own tests; this only needs to prove vessel/httpapi's
// auto-fetch-on-sync path calls this RPC and applies whatever it gets
// back.
func (f *fakeOfficeSyncServer) FetchRestoreBundle(_ context.Context, req *connect.Request[syncv1.FetchRestoreBundleRequest]) (*connect.Response[syncv1.FetchRestoreBundleResponse], error) {
	if got := req.Header().Get("Authorization"); got != "Bearer "+f.wantCredential {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("wrong credential"))
	}
	f.fetchedRestoreCommandIDs = append(f.fetchedRestoreCommandIDs, req.Msg.GetCommandId())
	return connect.NewResponse(&syncv1.FetchRestoreBundleResponse{Ciphertext: f.fetchRestoreBundleCiphertext}), nil
}

// PushOutbox accepts every item unconditionally — real idempotency/
// landing behavior is office/syncservice's own responsibility, already
// covered by its own tests; this only needs to prove the vessel side
// builds and sends a batch, then acks what comes back accepted.
func (f *fakeOfficeSyncServer) PushOutbox(_ context.Context, req *connect.Request[syncv1.PushOutboxRequest]) (*connect.Response[syncv1.PushOutboxResponse], error) {
	if got := req.Header().Get("Authorization"); got != "Bearer "+f.wantCredential {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("wrong credential"))
	}
	f.pushedItems = append(f.pushedItems, req.Msg.GetItems()...)
	acks := make([]*syncv1.ItemAck, len(req.Msg.GetItems()))
	for i, item := range req.Msg.GetItems() {
		acks[i] = &syncv1.ItemAck{ItemId: item.GetItemId(), Accepted: true}
	}
	return connect.NewResponse(&syncv1.PushOutboxResponse{Acks: acks}), nil
}

// PullInbox returns nothing new and simply echoes the caller's cursors
// back unchanged — these sync tests only exercise PushOutbox/SyncStatus;
// office/syncservice's own tests already cover PullInbox's real
// behavior (landing content, cursor advancement).
func (f *fakeOfficeSyncServer) PullInbox(_ context.Context, req *connect.Request[syncv1.PullInboxRequest]) (*connect.Response[syncv1.PullInboxResponse], error) {
	if got := req.Header().Get("Authorization"); got != "Bearer "+f.wantCredential {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("wrong credential"))
	}
	if f.pullResponse != nil {
		return connect.NewResponse(f.pullResponse), nil
	}
	return connect.NewResponse(&syncv1.PullInboxResponse{NextCursors: req.Msg.GetCursors()}), nil
}

// QueryMissingAttachmentChunks always reports every chunk missing — this
// fake only needs to prove vessel/httpapi's push phase drives
// vessel/sync.Client.UploadAttachment at all; the real query-then-send-
// missing resumability contract is vessel/sync/attachments_test.go's own
// job to cover, against its own fakeAttachmentOffice.
func (f *fakeOfficeSyncServer) QueryMissingAttachmentChunks(_ context.Context, req *connect.Request[syncv1.QueryMissingAttachmentChunksRequest]) (*connect.Response[syncv1.QueryMissingAttachmentChunksResponse], error) {
	if got := req.Header().Get("Authorization"); got != "Bearer "+f.wantCredential {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("wrong credential"))
	}
	meta := req.Msg.GetAttachment()
	total := (meta.GetTotalSize() + int64(meta.GetChunkSize()) - 1) / int64(meta.GetChunkSize())
	missing := make([]int32, total)
	for i := range missing {
		missing[i] = int32(i)
	}
	return connect.NewResponse(&syncv1.QueryMissingAttachmentChunksResponse{MissingChunkIndices: missing}), nil
}

func (f *fakeOfficeSyncServer) UploadAttachmentChunk(_ context.Context, req *connect.Request[syncv1.UploadAttachmentChunkRequest]) (*connect.Response[syncv1.UploadAttachmentChunkResponse], error) {
	if got := req.Header().Get("Authorization"); got != "Bearer "+f.wantCredential {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("wrong credential"))
	}
	f.receivedAttachmentChunks++
	return connect.NewResponse(&syncv1.UploadAttachmentChunkResponse{Complete: true}), nil
}

// newFakeOffice serves both POST /api/enroll (mimicking
// office/httpapi.handleRedeemEnrollment's response shape) and the
// SyncService RPC, so wizard step 2 and RunSyncCycle can both be
// exercised against one stand-in office.
func newFakeOffice(t *testing.T, credential string) (*httptest.Server, *fakeOfficeSyncServer) {
	t.Helper()
	fake := &fakeOfficeSyncServer{wantCredential: credential, serverTime: time.Now().UTC()}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/enroll", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"credential": credential})
	})
	path, handler := syncv1connect.NewSyncServiceHandler(fake, syncproto.ServerOptions()...)
	mux.Handle(path, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, fake
}

func TestHandleSyncNow_EndToEnd(t *testing.T) {
	office, fake := newFakeOffice(t, "the-test-credential")

	_, c := newLoggedInTestServer(t) // skips enrollment; re-enroll below against the fake office
	rec := c.do(http.MethodPost, "/api/setup/enrollment", setupEnrollmentRequest{OfficeURL: office.URL, Code: "SOME-CODE"})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/setup/enrollment: status %d, body %s", rec.Code, rec.Body)
	}
	// Enrollment itself now runs one sync cycle immediately (2026-07-14
	// manual-test feedback: "Sync now" used to stay disabled for up to
	// syncInterval after enrolling, since syncStatusView.Enrolled only
	// flips on a completed cycle) — reset the counter so this assertion
	// isolates the explicit "/api/sync/now" call below.
	fake.callCount = 0

	rec = c.do(http.MethodPost, "/api/sync/now", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/sync/now: status %d, body %s", rec.Code, rec.Body)
	}
	status := decodeBody[syncStatusView](t, rec)
	if !status.Enrolled || status.LastError != "" || status.LastSuccess == nil {
		t.Errorf("status = %+v, want Enrolled=true LastError=\"\" LastSuccess set", status)
	}
	if fake.callCount != 1 {
		t.Errorf("office SyncStatus call count = %d, want 1", fake.callCount)
	}

	rec = c.do(http.MethodGet, "/api/sync/status", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/sync/status: status %d, body %s", rec.Code, rec.Body)
	}
	polled := decodeBody[syncStatusView](t, rec)
	if !polled.Enrolled || polled.LastSuccess == nil {
		t.Errorf("polled status = %+v, want Enrolled=true LastSuccess set", polled)
	}
}

// TestHandleSyncNow_PullsSchemaVersionAndConfigBundle exercises
// architecture 11.2 step 3 end to end from the vessel's side: a sync
// cycle applies whatever the office's PullInbox returns and advances
// the local cursors atomically, and a second cycle with nothing new
// leaves everything unchanged.
func TestHandleSyncNow_PullsSchemaVersionAndConfigBundle(t *testing.T) {
	office, fake := newFakeOffice(t, "the-test-credential")
	fake.pullResponse = &syncv1.PullInboxResponse{
		SchemaVersions: []*syncv1.SchemaVersion{{
			SchemaKind:  syncv1.ReportSchemaKind_REPORT_SCHEMA_KIND_COMMERCIAL_PERIOD,
			Version:     "3.13",
			SchemaJson:  []byte(`{"schemaName":"commercial-period"}`),
			PublishedAt: timestamppb.New(time.Now().UTC()),
		}},
		ConfigBundles: []*syncv1.ConfigBundle{{
			BundleId:    "bundle-1",
			VersionNo:   7,
			ContentJson: []byte(`{"id":"bundle-1"}`),
			PublishedAt: timestamppb.New(time.Now().UTC()),
		}},
		NextCursors: &syncv1.SyncCursors{SchemaVersionCursor: 1, ConfigBundleCursor: 7},
	}

	s, c := newLoggedInTestServer(t) // skips enrollment; re-enroll below against the fake office
	rec := c.do(http.MethodPost, "/api/setup/enrollment", setupEnrollmentRequest{OfficeURL: office.URL, Code: "SOME-CODE"})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/setup/enrollment: status %d, body %s", rec.Code, rec.Body)
	}

	if rec := c.do(http.MethodPost, "/api/sync/now", nil); rec.Code != http.StatusOK {
		t.Fatalf("POST /api/sync/now: status %d, body %s", rec.Code, rec.Body)
	}

	cursors, err := s.storeOrNil().GetInboxCursors(context.Background())
	if err != nil {
		t.Fatalf("GetInboxCursors: %v", err)
	}
	if cursors.SchemaVersionCursor != 1 || cursors.ConfigBundleCursor != 7 {
		t.Errorf("cursors = %+v, want SchemaVersionCursor=1 ConfigBundleCursor=7", cursors)
	}

	// A second cycle with the office now echoing the advanced cursors
	// (nothing new) must not error and must leave state unchanged.
	fake.pullResponse = nil
	if rec := c.do(http.MethodPost, "/api/sync/now", nil); rec.Code != http.StatusOK {
		t.Fatalf("POST /api/sync/now (second cycle): status %d, body %s", rec.Code, rec.Body)
	}
	cursorsAfter, err := s.storeOrNil().GetInboxCursors(context.Background())
	if err != nil {
		t.Fatalf("GetInboxCursors (second): %v", err)
	}
	if cursorsAfter != cursors {
		t.Errorf("cursors changed on an empty pull: before=%+v after=%+v", cursors, cursorsAfter)
	}
}

// TestHandleSyncNow_AppliesPulledConfigBundle is the audit §8-item-1
// verification: a config bundle pulled over real HTTP is not merely
// stored (TestHandleSyncNow_PullsSchemaVersionAndConfigBundle already
// covers the cursor/store side) but becomes *authoritative on board*.
// After one sync cycle the vessel serves the bundle's resolved cadence,
// enabled regulatory profiles and field policy from its own HTTP
// endpoints — proving the full office→pull→store→appliedBundle→serve
// round trip that §1 built, which nothing exercised end to end before.
func TestHandleSyncNow_AppliesPulledConfigBundle(t *testing.T) {
	office, fake := newFakeOffice(t, "the-test-credential")

	// A real, tagged configwire document (not the dummy {"id":...} the
	// cursor-only test uses) so appliedBundle decodes it and the resolvers
	// treat it as authoritative rather than falling back to the stand-in.
	bundleContent, err := json.Marshal(configwire.Bundle{
		WireVersion:        configwire.WireVersion,
		BundleID:           "bundle-applied-1",
		VersionNo:          7,
		PublishedAt:        time.Now().UTC(),
		MaxGapHours:        8,               // not the 12h stand-in
		RegulatoryProfiles: []string{"mrv"}, // not all-profiles
		RuleSeverities:     map[string]string{validation.RuleROBContinuity: "error"},
		Schemas: []configwire.SchemaConfig{{
			SchemaName: "commercial-period",
			Version:    "3.13",
			Policy:     map[string]string{"Charterer": "companyMandatory"},
		}},
	})
	if err != nil {
		t.Fatalf("marshal configwire bundle: %v", err)
	}
	fake.pullResponse = &syncv1.PullInboxResponse{
		ConfigBundles: []*syncv1.ConfigBundle{{
			BundleId:    "bundle-applied-1",
			VersionNo:   7,
			ContentJson: bundleContent,
			PublishedAt: timestamppb.New(time.Now().UTC()),
		}},
		NextCursors: &syncv1.SyncCursors{ConfigBundleCursor: 7},
	}

	_, c := newLoggedInTestServer(t) // skips enrollment; re-enroll below against the fake office
	rec := c.do(http.MethodPost, "/api/setup/enrollment", setupEnrollmentRequest{OfficeURL: office.URL, Code: "SOME-CODE"})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/setup/enrollment: status %d, body %s", rec.Code, rec.Body)
	}
	if rec := c.do(http.MethodPost, "/api/sync/now", nil); rec.Code != http.StatusOK {
		t.Fatalf("POST /api/sync/now: status %d, body %s", rec.Code, rec.Body)
	}

	// Operational config endpoint (Home's overdue banner + readiness chips)
	// now reflects the bundle, not the built-in 12h / all-profiles stand-in.
	rec = c.do(http.MethodGet, "/api/vessel-config", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/vessel-config: status %d, body %s", rec.Code, rec.Body)
	}
	cfg := decodeBody[vesselConfigView](t, rec)
	if cfg.MaxGapHours != 8 {
		t.Errorf("MaxGapHours = %v, want 8 (from bundle, not the 12h stand-in)", cfg.MaxGapHours)
	}
	if len(cfg.RegulatoryProfiles) != 1 || cfg.RegulatoryProfiles[0] != "mrv" {
		t.Errorf("RegulatoryProfiles = %v, want [mrv] (from bundle, not all profiles)", cfg.RegulatoryProfiles)
	}
	if cfg.BundleID != "bundle-applied-1" || cfg.BundleVersionNo != 7 {
		t.Errorf("bundle id/version = %q/%d, want bundle-applied-1/7", cfg.BundleID, cfg.BundleVersionNo)
	}

	// Field policy on the schema fetch reflects the bundle too: its
	// companyMandatory Charterer wins, proving the applied-bundle path
	// (not fieldConfigFor's demo stand-in) drives the form.
	rec = c.do(http.MethodGet, "/api/schemas/commercial-period", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/schemas/commercial-period: status %d, body %s", rec.Code, rec.Body)
	}
	schemaResp := decodeBody[schemaConfigResponse](t, rec)
	if schemaResp.FieldPolicy["Charterer"] != "companyMandatory" {
		t.Errorf("Charterer policy = %q, want companyMandatory (from applied bundle)", schemaResp.FieldPolicy["Charterer"])
	}
}

// TestHandleSyncNow_PullsChatMessage covers T3.4 end to end (not just
// the store-level unit test): a chat message the office returns from
// PullInbox must land locally as direction=office and advance ChatCursor.
func TestHandleSyncNow_PullsChatMessage(t *testing.T) {
	office, fake := newFakeOffice(t, "the-test-credential")
	sentAt := time.Now().UTC()
	fake.pullResponse = &syncv1.PullInboxResponse{
		ChatMessages: []*syncv1.ChatMessage{{
			Id: "chat-pulled-sync-1", ReportId: "report-1", Sender: "reviewer1",
			Body: "please double-check this figure", SentAt: timestamppb.New(sentAt),
		}},
		NextCursors: &syncv1.SyncCursors{ChatCursor: 3},
	}

	s, c := newLoggedInTestServer(t)
	rec := c.do(http.MethodPost, "/api/setup/enrollment", setupEnrollmentRequest{OfficeURL: office.URL, Code: "SOME-CODE"})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/setup/enrollment: status %d, body %s", rec.Code, rec.Body)
	}

	if rec := c.do(http.MethodPost, "/api/sync/now", nil); rec.Code != http.StatusOK {
		t.Fatalf("POST /api/sync/now: status %d, body %s", rec.Code, rec.Body)
	}

	cursors, err := s.storeOrNil().GetInboxCursors(context.Background())
	if err != nil {
		t.Fatalf("GetInboxCursors: %v", err)
	}
	if cursors.ChatCursor != 3 {
		t.Errorf("ChatCursor = %d, want 3", cursors.ChatCursor)
	}

	messages, err := s.storeOrNil().ListChatMessages(context.Background(), "report-1")
	if err != nil {
		t.Fatalf("ListChatMessages: %v", err)
	}
	if len(messages) != 1 || messages[0].ID != "chat-pulled-sync-1" || messages[0].Direction != domain.ChatFromOffice {
		t.Errorf("messages = %+v, want the pulled office message", messages)
	}
}

// TestHandleSyncNow_PullsInvalidationNotice covers T4.2 end to end: an
// invalidation notice the office returns from PullInbox must drive the
// affected local report through Invalidate (real audit event, not a
// silent state flip) and advance InvalidationNoticeCursor.
func TestHandleSyncNow_PullsInvalidationNotice(t *testing.T) {
	office, fake := newFakeOffice(t, "the-test-credential")

	s, c := newLoggedInTestServer(t)
	rec := c.do(http.MethodPost, "/api/setup/enrollment", setupEnrollmentRequest{OfficeURL: office.URL, Code: "SOME-CODE"})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/setup/enrollment: status %d, body %s", rec.Code, rec.Body)
	}
	created := createTestReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), commercialPeriodFields())
	c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/check", nil)
	if rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/submit", nil); rec.Code != http.StatusOK {
		t.Fatalf("submit: status %d, body %s", rec.Code, rec.Body)
	}
	// The submit above enqueues outbox items the fake office's PushOutbox
	// accepts unconditionally; drain them via a first sync cycle before
	// exercising the pull side, so this test only measures the pull.
	if rec := c.do(http.MethodPost, "/api/sync/now", nil); rec.Code != http.StatusOK {
		t.Fatalf("POST /api/sync/now (drain outbox): status %d, body %s", rec.Code, rec.Body)
	}

	fake.pullResponse = &syncv1.PullInboxResponse{
		InvalidationNotices: []*syncv1.InvalidationNotice{{
			ReportId: created.ReportID, VersionNo: 1, BrokenRules: []string{"continuity.timeChain"},
			ComputedAt: timestamppb.New(time.Now().UTC()),
		}},
		NextCursors: &syncv1.SyncCursors{InvalidationNoticeCursor: 9},
	}
	if rec := c.do(http.MethodPost, "/api/sync/now", nil); rec.Code != http.StatusOK {
		t.Fatalf("POST /api/sync/now (pull notice): status %d, body %s", rec.Code, rec.Body)
	}

	cursors, err := s.storeOrNil().GetInboxCursors(context.Background())
	if err != nil {
		t.Fatalf("GetInboxCursors: %v", err)
	}
	if cursors.InvalidationNoticeCursor != 9 {
		t.Errorf("InvalidationNoticeCursor = %d, want 9", cursors.InvalidationNoticeCursor)
	}

	got, err := s.storeOrNil().GetLatestVersion(context.Background(), created.ReportID)
	if err != nil {
		t.Fatalf("GetLatestVersion: %v", err)
	}
	if got.State != "invalidated" {
		t.Errorf("State = %q, want invalidated", got.State)
	}

	events, err := s.storeOrNil().ListEvents(context.Background(), created.ReportID)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	found := false
	for _, e := range events {
		if e.Type == "invalidated" {
			found = true
		}
	}
	if !found {
		t.Errorf("events = %+v, want an invalidated event", events)
	}
}

// TestHandleSyncNow_PullsRemark covers T5.4 end to end: a remark the
// office returns from PullInbox must drive the affected local report
// through MarkRemarked (real audit event), land in the remarks table,
// and advance RemarkCursor.
func TestHandleSyncNow_PullsRemark(t *testing.T) {
	office, fake := newFakeOffice(t, "the-test-credential")

	s, c := newLoggedInTestServer(t)
	rec := c.do(http.MethodPost, "/api/setup/enrollment", setupEnrollmentRequest{OfficeURL: office.URL, Code: "SOME-CODE"})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/setup/enrollment: status %d, body %s", rec.Code, rec.Body)
	}
	created := createTestReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), commercialPeriodFields())
	c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/check", nil)
	if rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/submit", nil); rec.Code != http.StatusOK {
		t.Fatalf("submit: status %d, body %s", rec.Code, rec.Body)
	}
	if rec := c.do(http.MethodPost, "/api/sync/now", nil); rec.Code != http.StatusOK {
		t.Fatalf("POST /api/sync/now (drain outbox): status %d, body %s", rec.Code, rec.Body)
	}

	fake.pullResponse = &syncv1.PullInboxResponse{
		Remarks: []*syncv1.Remark{{
			Id: "remark-pulled-sync-1", ReportId: created.ReportID, VersionNo: 1, FieldName: "Charterer",
			Body: "please double-check this figure", Author: "reviewer1", CreatedAt: timestamppb.New(time.Now().UTC()),
		}},
		NextCursors: &syncv1.SyncCursors{RemarkCursor: 6},
	}
	if rec := c.do(http.MethodPost, "/api/sync/now", nil); rec.Code != http.StatusOK {
		t.Fatalf("POST /api/sync/now (pull remark): status %d, body %s", rec.Code, rec.Body)
	}

	cursors, err := s.storeOrNil().GetInboxCursors(context.Background())
	if err != nil {
		t.Fatalf("GetInboxCursors: %v", err)
	}
	if cursors.RemarkCursor != 6 {
		t.Errorf("RemarkCursor = %d, want 6", cursors.RemarkCursor)
	}

	got, err := s.storeOrNil().GetLatestVersion(context.Background(), created.ReportID)
	if err != nil {
		t.Fatalf("GetLatestVersion: %v", err)
	}
	if got.State != "remarked" {
		t.Errorf("State = %q, want remarked", got.State)
	}

	events, err := s.storeOrNil().ListEvents(context.Background(), created.ReportID)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	found := false
	for _, e := range events {
		if e.Type == domain.EventRemarked {
			found = true
		}
	}
	if !found {
		t.Errorf("events = %+v, want a remarked event", events)
	}

	remarks, err := s.storeOrNil().ListRemarks(context.Background(), created.ReportID)
	if err != nil {
		t.Fatalf("ListRemarks: %v", err)
	}
	if len(remarks) != 1 || remarks[0].FieldName != "Charterer" {
		t.Errorf("remarks = %+v, want one Charterer remark", remarks)
	}
}

// TestHandleSyncNow_PushesSubmittedReportOutbox exercises architecture
// 11.2 step 1 end to end from the vessel's side: submitting a report
// enqueues outbox items (vessel/httpapi.handleSubmitReport), and a sync
// cycle sends them to the office and acks (removes) whatever it accepts.
func TestHandleSyncNow_PushesSubmittedReportOutbox(t *testing.T) {
	office, fake := newFakeOffice(t, "the-test-credential")

	s, c := newLoggedInTestServer(t) // skips enrollment; re-enroll below
	rec := c.do(http.MethodPost, "/api/setup/enrollment", setupEnrollmentRequest{OfficeURL: office.URL, Code: "SOME-CODE"})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/setup/enrollment: status %d, body %s", rec.Code, rec.Body)
	}

	created := createTestReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), commercialPeriodFields())
	c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/check", nil)
	if rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/submit", nil); rec.Code != http.StatusOK {
		t.Fatalf("submit: status %d, body %s", rec.Code, rec.Body)
	}

	pendingBefore, err := s.storeOrNil().ListOutboxItems(context.Background())
	if err != nil {
		t.Fatalf("ListOutboxItems: %v", err)
	}
	// created (handleCreateReport) + health_check_result (the /check
	// call above) + report version + submitted event — architecture 14's
	// audit trail "spans vessel and office" for every event type it
	// lists, not just submit/resubmit (see reports.go's handleCreateReport
	// comment on this fix).
	if len(pendingBefore) != 4 {
		t.Fatalf("pending outbox items before sync = %d, want 4 (created event + health_check_result event + report version + submitted event)", len(pendingBefore))
	}

	rec = c.do(http.MethodPost, "/api/sync/now", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/sync/now: status %d, body %s", rec.Code, rec.Body)
	}

	if len(fake.pushedItems) != 4 {
		t.Fatalf("office received %d items, want 4", len(fake.pushedItems))
	}
	pendingAfter, err := s.storeOrNil().ListOutboxItems(context.Background())
	if err != nil {
		t.Fatalf("ListOutboxItems: %v", err)
	}
	if len(pendingAfter) != 0 {
		t.Errorf("pending outbox items after a fully-accepted sync = %d, want 0", len(pendingAfter))
	}

	// 18.07.26 manual-test item 2: a reportVersion outbox ack must flip
	// the report to synced (domain.Report.MarkSynced), not leave it
	// showing "Submitted" forever.
	got, err := s.storeOrNil().GetReport(context.Background(), created.ReportID, 1)
	if err != nil {
		t.Fatalf("GetReport: %v", err)
	}
	if got.State != domain.StateSynced {
		t.Errorf("report state after a fully-accepted sync = %q, want %q", got.State, domain.StateSynced)
	}
}

// TestHandleSyncNow_PushesChatMessage covers T3.2's buildOutboxItem
// extension: an enqueued chat message must hydrate to a real
// OutboxItem_ChatMessage payload and get pushed like any other kind.
func TestHandleSyncNow_PushesChatMessage(t *testing.T) {
	office, fake := newFakeOffice(t, "the-test-credential")

	s, c := newLoggedInTestServer(t)
	rec := c.do(http.MethodPost, "/api/setup/enrollment", setupEnrollmentRequest{OfficeURL: office.URL, Code: "SOME-CODE"})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/setup/enrollment: status %d, body %s", rec.Code, rec.Body)
	}

	st := s.storeOrNil()
	msg := domain.ChatMessage{
		ID: "chat-sync-1", ReportID: "report-1", Sender: "master", Body: "corrected version pushed",
		SentAt: time.Now().UTC(), Direction: domain.ChatFromVessel,
	}
	if err := st.InsertChatMessage(context.Background(), msg); err != nil {
		t.Fatalf("InsertChatMessage: %v", err)
	}
	if _, err := st.EnqueueChatMessage(context.Background(), msg.ReportID, msg.ID); err != nil {
		t.Fatalf("EnqueueChatMessage: %v", err)
	}

	if rec := c.do(http.MethodPost, "/api/sync/now", nil); rec.Code != http.StatusOK {
		t.Fatalf("POST /api/sync/now: status %d, body %s", rec.Code, rec.Body)
	}

	if len(fake.pushedItems) != 1 {
		t.Fatalf("office received %d items, want 1", len(fake.pushedItems))
	}
	chatPayload, ok := fake.pushedItems[0].GetPayload().(*syncv1.OutboxItem_ChatMessage)
	if !ok {
		t.Fatalf("pushed item payload = %T, want *OutboxItem_ChatMessage", fake.pushedItems[0].GetPayload())
	}
	if chatPayload.ChatMessage.GetId() != "chat-sync-1" || chatPayload.ChatMessage.GetBody() != "corrected version pushed" {
		t.Errorf("chat payload = %+v, want id=chat-sync-1 body='corrected version pushed'", chatPayload.ChatMessage)
	}
}

// TestHandleSyncNow_PushesAttachment covers Phase 6's pushAttachmentsBatch:
// an uploaded attachment not yet synced_at must get its chunks pushed via
// vessel/sync.Client.UploadAttachment (already unit-tested on its own in
// vessel/sync/attachments_test.go) and end up marked synced.
func TestHandleSyncNow_PushesAttachment(t *testing.T) {
	office, fake := newFakeOffice(t, "the-test-credential")

	s, c := newLoggedInTestServer(t)
	rec := c.do(http.MethodPost, "/api/setup/enrollment", setupEnrollmentRequest{OfficeURL: office.URL, Code: "SOME-CODE"})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/setup/enrollment: status %d, body %s", rec.Code, rec.Body)
	}

	created := createTestReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), commercialPeriodFields())
	uploaded := decodeBody[attachmentView](t, uploadAttachment(t, c, created.ReportID, "bdn.jpg", "image/jpeg", []byte("bunker delivery note bytes")))
	if uploaded.Synced {
		t.Fatal("freshly uploaded attachment should not be synced yet")
	}

	if rec := c.do(http.MethodPost, "/api/sync/now", nil); rec.Code != http.StatusOK {
		t.Fatalf("POST /api/sync/now: status %d, body %s", rec.Code, rec.Body)
	}
	if fake.receivedAttachmentChunks == 0 {
		t.Error("office received no attachment chunks")
	}

	pending, err := s.storeOrNil().ListPendingAttachments(context.Background())
	if err != nil {
		t.Fatalf("ListPendingAttachments: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("pending attachments after a fully-synced push = %d, want 0", len(pending))
	}
}

// TestHandleSyncNow_AutoFetchesAndAppliesRestoreCommand is architecture
// 12.5's DR push path exercised end to end from the vessel's side: a
// restore command delivered via PullInbox gets auto-fetched
// (FetchRestoreBundle), decrypted with this vessel's own DR key (minted
// for real by the enrollment redemption below, not stubbed), applied to
// the local store (including the bundle's embedded config bundle), and
// the applied command id is reported back on the same cycle's
// SyncStatus call — the "confirming it's pushed" signal office's DR tab
// surfaces.
func TestHandleSyncNow_AutoFetchesAndAppliesRestoreCommand(t *testing.T) {
	office, fake := newFakeOffice(t, "the-test-credential")

	s, c := newLoggedInTestServer(t) // skips enrollment; re-enroll below against the fake office
	rec := c.do(http.MethodPost, "/api/setup/enrollment", setupEnrollmentRequest{OfficeURL: office.URL, Code: "SOME-CODE"})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/setup/enrollment: status %d, body %s", rec.Code, rec.Body)
	}
	fake.callCount = 0 // enrollment itself already ran one sync cycle; isolate the cycle under test

	st := s.storeOrNil()
	identity, err := st.GetDRIdentity(context.Background())
	if err != nil {
		t.Fatalf("GetDRIdentity (should have been generated by enrollment redemption): %v", err)
	}

	eventTime := time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)
	bundle := restorebundle.Bundle{
		VesselID: "vessel-1", VesselName: "MV Test", VesselIMO: "9074729",
		GeneratedAt: time.Now().UTC(),
		Reports: []restorebundle.BundleReport{{
			ReportID: "restored-report-auto-1",
			Versions: []*domain.Report{{
				ReportID: "restored-report-auto-1", VersionNo: 1,
				SchemaName: "log-abstract", EventType: "Departure", EventTime: eventTime,
				Fields: map[string]any{"IMO": 9074729.0}, State: domain.StateSubmitted,
				CreatedAt: eventTime, CreatedBy: "master", UpdatedAt: eventTime,
				SubmittedAt: eventTime, SubmittedBy: "master",
			}},
		}},
		ConfigBundle: &restorebundle.ConfigBundle{
			BundleID: "bundle-from-restore", VersionNo: 3, ContentJSON: []byte(`{"id":"bundle-from-restore"}`), PublishedAt: eventTime,
		},
	}
	plaintext, err := json.Marshal(bundle)
	if err != nil {
		t.Fatalf("marshal bundle: %v", err)
	}
	ciphertext, err := backupcrypto.Encrypt(plaintext, identity.PublicKey)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	fake.fetchRestoreBundleCiphertext = ciphertext
	fake.pullResponse = &syncv1.PullInboxResponse{
		RestoreCommands: []*syncv1.RestoreCommand{{CommandId: "restore-cmd-auto-1", Reason: "power outage", IssuedAt: timestamppb.New(eventTime)}},
		NextCursors:     &syncv1.SyncCursors{RestoreCommandCursor: 1},
	}

	if rec := c.do(http.MethodPost, "/api/sync/now", nil); rec.Code != http.StatusOK {
		t.Fatalf("POST /api/sync/now: status %d, body %s", rec.Code, rec.Body)
	}

	if len(fake.fetchedRestoreCommandIDs) != 1 || fake.fetchedRestoreCommandIDs[0] != "restore-cmd-auto-1" {
		t.Errorf("fetchedRestoreCommandIDs = %v, want exactly [restore-cmd-auto-1]", fake.fetchedRestoreCommandIDs)
	}
	if len(fake.gotAppliedRestoreCommandIDs) != 1 || fake.gotAppliedRestoreCommandIDs[0] != "restore-cmd-auto-1" {
		t.Errorf("SyncStatus AppliedRestoreCommandIds = %v, want exactly [restore-cmd-auto-1]", fake.gotAppliedRestoreCommandIDs)
	}

	rec = c.do(http.MethodGet, "/api/reports/restored-report-auto-1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get auto-applied report: status %d, body %s", rec.Code, rec.Body)
	}
	got := decodeBody[reportView](t, rec)
	if got.Fields["IMO"] != 9074729.0 {
		t.Errorf("Fields[IMO] = %v, want 9074729", got.Fields["IMO"])
	}

	cursors, err := st.GetInboxCursors(context.Background())
	if err != nil {
		t.Fatalf("GetInboxCursors: %v", err)
	}
	if cursors.RestoreCommandCursor != 1 {
		t.Errorf("RestoreCommandCursor = %d, want 1 (advanced after successful apply)", cursors.RestoreCommandCursor)
	}
}

// TestHandleSyncNow_AppliesUserCommandAndReportsRoster is architecture
// 9.3/12.4's remote vessel-user-administration path exercised end to
// end from the vessel's side: a pulled UserCommand (resetPassword on
// the Master account — the forgot-their-password recovery path) is
// applied locally, the applied id is reported back via SyncStatus, and
// the vessel's current roster is reported on the same call.
func TestHandleSyncNow_AppliesUserCommandAndReportsRoster(t *testing.T) {
	office, fake := newFakeOffice(t, "the-test-credential")

	s, c := newLoggedInTestServer(t) // creates the "master" account; skips enrollment, re-enrolled below
	rec := c.do(http.MethodPost, "/api/setup/enrollment", setupEnrollmentRequest{OfficeURL: office.URL, Code: "SOME-CODE"})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/setup/enrollment: status %d, body %s", rec.Code, rec.Body)
	}
	fake.callCount = 0

	fake.pullResponse = &syncv1.PullInboxResponse{
		UserCommands: []*syncv1.UserCommand{{
			CommandId: "user-cmd-auto-1", Action: syncv1.UserCommandAction_USER_COMMAND_ACTION_RESET_PASSWORD,
			Username: "master", TemporaryPassword: "a-new-temporary-password", IssuedBy: "admin", IssuedAt: timestamppb.New(time.Now().UTC()),
		}},
		NextCursors: &syncv1.SyncCursors{UserCommandCursor: 1},
	}

	if rec := c.do(http.MethodPost, "/api/sync/now", nil); rec.Code != http.StatusOK {
		t.Fatalf("POST /api/sync/now: status %d, body %s", rec.Code, rec.Body)
	}

	if len(fake.gotAppliedUserCommandIDs) != 1 || fake.gotAppliedUserCommandIDs[0] != "user-cmd-auto-1" {
		t.Fatalf("gotAppliedUserCommandIDs = %v, want exactly [user-cmd-auto-1]", fake.gotAppliedUserCommandIDs)
	}
	var foundMaster bool
	for _, u := range fake.gotUsers {
		if u.GetUsername() == "master" {
			foundMaster = true
			if u.GetRole() != "master" || !u.GetActive() {
				t.Errorf("reported master user = %+v, want role=master active=true", u)
			}
		}
	}
	if !foundMaster {
		t.Fatalf("gotUsers = %+v, want the master account reported in the roster", fake.gotUsers)
	}

	cursors, err := s.storeOrNil().GetInboxCursors(context.Background())
	if err != nil {
		t.Fatalf("GetInboxCursors: %v", err)
	}
	if cursors.UserCommandCursor != 1 {
		t.Errorf("UserCommandCursor = %d, want 1 (advanced after successful apply)", cursors.UserCommandCursor)
	}

	// The reset must actually have taken effect locally: the vessel's own
	// login now accepts the new temporary password.
	anon := newTestClient(t, s)
	rec = anon.do(http.MethodPost, "/api/auth/login", map[string]string{"username": "master", "password": "a-new-temporary-password"})
	if rec.Code != http.StatusOK {
		t.Fatalf("login with the remotely-reset password: status %d, body %s", rec.Code, rec.Body)
	}
}

// TestApplyUserCommand_RefusesRemoteMasterCreationAndDeactivation is the
// vessel-side half of the same guardrails office/httpapi/vesselusers.go
// already enforces at queue time — the vessel is the final authority
// over its own accounts and must not trust office alone.
func TestApplyUserCommand_RefusesRemoteMasterCreationAndDeactivation(t *testing.T) {
	s, _ := newLoggedInTestServer(t)
	st := s.storeOrNil()
	ctx := context.Background()

	if err := applyUserCommand(ctx, st, &syncv1.UserCommand{
		Action: syncv1.UserCommandAction_USER_COMMAND_ACTION_CREATE, Username: "second-master", Role: "master", TemporaryPassword: "x",
	}); err == nil {
		t.Error("applyUserCommand(create role=master) = nil error, want a refusal")
	}

	if err := applyUserCommand(ctx, st, &syncv1.UserCommand{
		Action: syncv1.UserCommandAction_USER_COMMAND_ACTION_SET_ACTIVE, Username: "master", Active: false,
	}); err == nil {
		t.Error("applyUserCommand(deactivate master) = nil error, want a refusal")
	}

	master, err := st.GetUserByUsername(ctx, "master")
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}
	if !master.Active {
		t.Error("master.Active = false after a refused deactivate command, want still true")
	}
}

func TestHandleSyncNow_NotEnrolled(t *testing.T) {
	_, c := newLoggedInTestServer(t) // skips enrollment
	rec := c.do(http.MethodPost, "/api/sync/now", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleSyncStatus_RequiresAuth(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	rec := c.do(http.MethodGet, "/api/sync/status", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
