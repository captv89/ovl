// SPDX-License-Identifier: AGPL-3.0-only

package sync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"

	syncv1 "github.com/captv89/ovl/pkg/syncproto/gen/ovl/sync/v1"
	"github.com/captv89/ovl/pkg/syncproto/gen/ovl/sync/v1/syncv1connect"

	"github.com/captv89/ovl/pkg/syncproto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// fakeSyncServer is a minimal, local stand-in for office/syncservice's
// real handler — deliberately not importing office code here: vessel and
// office share only pkg/ and proto/, never each other, and this is the
// first client test that would otherwise be tempted to cross that line.
// office/syncservice's own tests already cover the auth interceptor;
// this only needs to prove vessel/sync.Client speaks the wire protocol
// (including the Authorization header and zstd negotiation) correctly.
type fakeSyncServer struct {
	syncv1connect.UnimplementedSyncServiceHandler
	gotAuthHeader        string
	gotAppVersion        string
	gotAppliedBundleID   string
	gotAppliedBundleVers int64
	serverTime           time.Time
}

func (f *fakeSyncServer) SyncStatus(_ context.Context, req *connect.Request[syncv1.SyncStatusRequest]) (*connect.Response[syncv1.SyncStatusResponse], error) {
	f.gotAuthHeader = req.Header().Get("Authorization")
	f.gotAppVersion = req.Msg.GetAppVersion()
	f.gotAppliedBundleID = req.Msg.GetAppliedBundleId()
	f.gotAppliedBundleVers = req.Msg.GetAppliedBundleVersion()
	return connect.NewResponse(&syncv1.SyncStatusResponse{ServerTime: timestamppb.New(f.serverTime)}), nil
}

func (f *fakeSyncServer) PullInbox(_ context.Context, req *connect.Request[syncv1.PullInboxRequest]) (*connect.Response[syncv1.PullInboxResponse], error) {
	return connect.NewResponse(&syncv1.PullInboxResponse{
		SchemaVersions: []*syncv1.SchemaVersion{{Version: "3.13", SchemaJson: []byte(`{}`)}},
		NextCursors:    &syncv1.SyncCursors{SchemaVersionCursor: req.Msg.GetCursors().GetSchemaVersionCursor() + 1},
	}), nil
}

func newFakeSyncServer(t *testing.T, f *fakeSyncServer) *httptest.Server {
	t.Helper()
	path, handler := syncv1connect.NewSyncServiceHandler(f, syncproto.ServerOptions()...)
	mux := http.NewServeMux()
	mux.Handle(path, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestClient_SyncStatus(t *testing.T) {
	wantServerTime := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	fake := &fakeSyncServer{serverTime: wantServerTime}
	srv := newFakeSyncServer(t, fake)

	client := NewClient(srv.Client(), srv.URL, "the-vessel-credential")
	got, err := client.SyncStatus(context.Background(), "1.2.3", nil, nil, nil, "bundle-42", 42)
	if err != nil {
		t.Fatalf("SyncStatus: %v", err)
	}
	if !got.Equal(wantServerTime) {
		t.Errorf("SyncStatus() = %v, want %v", got, wantServerTime)
	}
	if fake.gotAuthHeader != "Bearer the-vessel-credential" {
		t.Errorf("Authorization header = %q, want %q", fake.gotAuthHeader, "Bearer the-vessel-credential")
	}
	if fake.gotAppVersion != "1.2.3" {
		t.Errorf("AppVersion = %q, want %q", fake.gotAppVersion, "1.2.3")
	}
	if fake.gotAppliedBundleID != "bundle-42" || fake.gotAppliedBundleVers != 42 {
		t.Errorf("applied bundle = %q v%d, want bundle-42 v42", fake.gotAppliedBundleID, fake.gotAppliedBundleVers)
	}
}

func TestClient_PullInbox(t *testing.T) {
	fake := &fakeSyncServer{}
	srv := newFakeSyncServer(t, fake)

	client := NewClient(srv.Client(), srv.URL, "the-vessel-credential")
	resp, err := client.PullInbox(context.Background(), &syncv1.SyncCursors{SchemaVersionCursor: 5})
	if err != nil {
		t.Fatalf("PullInbox: %v", err)
	}
	if len(resp.GetSchemaVersions()) != 1 || resp.GetSchemaVersions()[0].GetVersion() != "3.13" {
		t.Errorf("SchemaVersions = %v, want one item with Version=3.13", resp.GetSchemaVersions())
	}
	if resp.GetNextCursors().GetSchemaVersionCursor() != 6 {
		t.Errorf("NextCursors.SchemaVersionCursor = %d, want 6", resp.GetNextCursors().GetSchemaVersionCursor())
	}
}

func TestClient_SyncStatus_TrimsTrailingSlashFromOfficeURL(t *testing.T) {
	fake := &fakeSyncServer{serverTime: time.Now().UTC()}
	srv := newFakeSyncServer(t, fake)

	client := NewClient(srv.Client(), srv.URL+"/", "some-credential")
	if _, err := client.SyncStatus(context.Background(), "1.0.0", nil, nil, nil, "", 0); err != nil {
		t.Fatalf("SyncStatus: %v", err)
	}
}
