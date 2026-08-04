// SPDX-License-Identifier: AGPL-3.0-only

package syncservice

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"connectrpc.com/connect"
	_ "github.com/jackc/pgx/v5/stdlib"

	syncv1 "github.com/captv89/ovl/pkg/syncproto/gen/ovl/sync/v1"
	"github.com/captv89/ovl/pkg/syncproto/gen/ovl/sync/v1/syncv1connect"

	"github.com/captv89/ovl/pkg/attachmentstore"

	"github.com/captv89/ovl/office/store"
	"github.com/captv89/ovl/office/synccred"
	"github.com/captv89/ovl/office/vessels"
)

// testDSN returns the Postgres connection string for integration tests,
// skipping the test if none is configured — same gating as
// office/store's and office/httpapi's own tests. See
// deploy/office/docker-compose.yml for a local instance to point
// OVL_TEST_DATABASE_URL at.
func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("OVL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("OVL_TEST_DATABASE_URL not set; skipping Postgres integration test")
	}
	return dsn
}

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(testDSN(t))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return st
}

// createTestVessel creates a vessel with a checksum-valid IMO distinct
// per caller (id in [0, 99]) and registers cleanup.
func createTestVessel(t *testing.T, st *store.Store, id int) *vessels.Vessel {
	t.Helper()
	digits := [6]int{9, 0, 7, (id / 10) % 10, id % 10, 2}
	sum := 0
	for i, weight := 0, 7; i < 6; i, weight = i+1, weight-1 {
		sum += digits[i] * weight
	}
	check := sum % 10
	imo := ""
	for _, d := range digits {
		imo += string(rune('0' + d))
	}
	imo += string(rune('0' + check))

	v, err := vessels.NewVessel(imo, t.Name(), "Bulk Carrier", nil)
	if err != nil {
		t.Fatalf("vessels.NewVessel: %v", err)
	}
	if err := st.CreateVessel(context.Background(), v); err != nil {
		t.Fatalf("CreateVessel: %v", err)
	}
	t.Cleanup(func() {
		raw, err := sql.Open("pgx", testDSN(t))
		if err != nil {
			return
		}
		defer func() { _ = raw.Close() }()
		_, _ = raw.ExecContext(context.Background(), `DELETE FROM vessels WHERE id = $1`, v.ID)
	})
	return v
}

// mintTestCredential mints and persists a fresh credential for v,
// returning the plaintext token to authenticate with.
func mintTestCredential(t *testing.T, st *store.Store, v *vessels.Vessel) string {
	t.Helper()
	result, err := synccred.Mint(v.ID)
	if err != nil {
		t.Fatalf("synccred.Mint: %v", err)
	}
	if err := st.UpsertVesselCredential(context.Background(), result.Credential); err != nil {
		t.Fatalf("UpsertVesselCredential: %v", err)
	}
	return result.Token
}

// newTestClient builds a SyncService client against srv, authenticating
// every request with token (unless empty, to exercise the missing-
// credential path).
func newTestClient(srv *httptest.Server, token string) syncv1connect.SyncServiceClient {
	var opts []connect.ClientOption
	if token != "" {
		opts = append(opts, connect.WithInterceptors(connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
			return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
				req.Header().Set("Authorization", "Bearer "+token)
				return next(ctx, req)
			}
		})))
	}
	return syncv1connect.NewSyncServiceClient(srv.Client(), srv.URL, opts...)
}

func newTestServer(t *testing.T, st *store.Store) *httptest.Server {
	t.Helper()
	attachments, err := attachmentstore.New(t.TempDir())
	if err != nil {
		t.Fatalf("attachmentstore.New: %v", err)
	}
	path, handler := syncv1connect.NewSyncServiceHandler(
		New(st, attachments, t.TempDir()),
		connect.WithInterceptors(AuthInterceptor(st)),
	)
	mux := http.NewServeMux()
	mux.Handle(path, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestSyncStatus_Authenticated(t *testing.T) {
	st := openTestStore(t)
	v := createTestVessel(t, st, 80)
	token := mintTestCredential(t, st, v)
	srv := newTestServer(t, st)
	client := newTestClient(srv, token)

	resp, err := client.SyncStatus(context.Background(), connect.NewRequest(&syncv1.SyncStatusRequest{AppVersion: "1.0.0"}))
	if err != nil {
		t.Fatalf("SyncStatus: %v", err)
	}
	if resp.Msg.GetServerTime() == nil {
		t.Error("ServerTime is nil")
	}

	status, err := st.GetVesselSyncStatus(context.Background(), v.ID)
	if err != nil {
		t.Fatalf("GetVesselSyncStatus: %v", err)
	}
	if status.AppVersion != "1.0.0" {
		t.Errorf("AppVersion = %q, want %q", status.AppVersion, "1.0.0")
	}
}

func TestSyncStatus_MissingCredential(t *testing.T) {
	st := openTestStore(t)
	srv := newTestServer(t, st)
	client := newTestClient(srv, "")

	_, err := client.SyncStatus(context.Background(), connect.NewRequest(&syncv1.SyncStatusRequest{}))
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("SyncStatus (no credential) error = %v, want CodeUnauthenticated", err)
	}
}

func TestSyncStatus_UnknownCredential(t *testing.T) {
	st := openTestStore(t)
	srv := newTestServer(t, st)
	client := newTestClient(srv, "not-a-real-token")

	_, err := client.SyncStatus(context.Background(), connect.NewRequest(&syncv1.SyncStatusRequest{}))
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("SyncStatus (unknown credential) error = %v, want CodeUnauthenticated", err)
	}
}

func TestSyncStatus_RevokedCredential(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 81)

	result, err := synccred.Mint(v.ID)
	if err != nil {
		t.Fatalf("synccred.Mint: %v", err)
	}
	if err := st.UpsertVesselCredential(ctx, result.Credential); err != nil {
		t.Fatalf("UpsertVesselCredential: %v", err)
	}
	// Revoke the same credential row in place (token unchanged, only
	// RevokedAt set) — the currently-assigned credential being revoked,
	// not a superseded/replaced one.
	result.Credential.Revoke()
	if err := st.UpsertVesselCredential(ctx, result.Credential); err != nil {
		t.Fatalf("UpsertVesselCredential (revoked): %v", err)
	}

	srv := newTestServer(t, st)
	client := newTestClient(srv, result.Token)
	_, err = client.SyncStatus(ctx, connect.NewRequest(&syncv1.SyncStatusRequest{}))
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("SyncStatus (revoked credential) error = %v, want CodeUnauthenticated", err)
	}
}
