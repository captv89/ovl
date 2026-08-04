// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/captv89/ovl/vessel/bootstrap"
)

// configuredTestServer builds a Server that has already completed wizard
// step 1 (mode + data directory), the precondition handleSetupEnrollment
// checks before doing anything.
func configuredTestServer(t *testing.T) (*Server, *testClient) {
	t.Helper()
	s := newTestServer(t)
	c := newTestClient(t, s)
	dataDir := filepath.Join(t.TempDir(), "data")
	rec := c.do(http.MethodPost, "/api/setup/mode", setupModeRequest{Mode: bootstrap.ModeStandalone, DataDir: dataDir})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/setup/mode: status %d, body %s", rec.Code, rec.Body)
	}
	return s, c
}

func TestHandleSetupEnrollment_SuccessfulRedemption(t *testing.T) {
	office := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/enroll" {
			// Enrollment now runs one sync cycle immediately after
			// redeeming (2026-07-14 manual-test feedback — see
			// runSyncScheduler/handleSetupEnrollment's own comments), so
			// this fake office also sees SyncService RPC traffic right
			// after /api/enroll. This test only cares about the
			// redemption/identity-storage outcome, not sync itself
			// (that's sync_test.go's job) — a plain 404 here is enough
			// for RunSyncCycle to record a harmless LastError.
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"credential": "a-long-lived-token",
			"vesselName": "MV Testship",
			"vesselIMO":  "9074729",
		})
	}))
	defer office.Close()

	s, c := configuredTestServer(t)
	rec := c.do(http.MethodPost, "/api/setup/enrollment", setupEnrollmentRequest{OfficeURL: office.URL, Code: "THE-CODE"})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/setup/enrollment: status %d, body %s", rec.Code, rec.Body)
	}
	status := decodeBody[setupStatusResponse](t, rec)
	if !status.Enrollment.Submitted {
		t.Error("Enrollment.Submitted = false after a successful redemption, want true")
	}
	if status.Enrollment.OfficeURL != office.URL {
		t.Errorf("Enrollment.OfficeURL = %q, want %q", status.Enrollment.OfficeURL, office.URL)
	}
	if status.Enrollment.Code != "" {
		t.Errorf("Enrollment.Code = %q, want empty (the one-time code should not be persisted)", status.Enrollment.Code)
	}

	cred, err := s.storeOrNil().GetSyncCredential(t.Context())
	if err != nil {
		t.Fatalf("GetSyncCredential: %v", err)
	}
	if cred.Token != "a-long-lived-token" {
		t.Errorf("stored credential token = %q, want %q", cred.Token, "a-long-lived-token")
	}

	identity, err := s.storeOrNil().GetVesselIdentity(t.Context())
	if err != nil {
		t.Fatalf("GetVesselIdentity: %v", err)
	}
	if identity.Name != "MV Testship" || identity.IMO != "9074729" {
		t.Errorf("GetVesselIdentity = %+v, want {Name: MV Testship, IMO: 9074729}", identity)
	}
}

func TestHandleSetupEnrollment_RejectedCode(t *testing.T) {
	office := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "code not recognized or already used"})
	}))
	defer office.Close()

	s, c := configuredTestServer(t)
	rec := c.do(http.MethodPost, "/api/setup/enrollment", setupEnrollmentRequest{OfficeURL: office.URL, Code: "WRONG-CODE"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("POST /api/setup/enrollment: status %d, body %s, want %d", rec.Code, rec.Body, http.StatusUnauthorized)
	}

	if _, err := s.storeOrNil().GetSyncCredential(t.Context()); err == nil {
		t.Error("a credential was stored despite the office rejecting the code")
	}
	cfg := s.config()
	if cfg.Enrollment.Submitted {
		t.Error("Enrollment.Submitted = true after a rejected code, want false")
	}
}

func TestHandleSetupEnrollment_UnreachableOffice(t *testing.T) {
	s, c := configuredTestServer(t)
	// Port 0 on localhost is never a listening server.
	rec := c.do(http.MethodPost, "/api/setup/enrollment", setupEnrollmentRequest{OfficeURL: "http://127.0.0.1:0", Code: "SOME-CODE"})
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("POST /api/setup/enrollment: status %d, body %s, want %d", rec.Code, rec.Body, http.StatusBadGateway)
	}
	if _, err := s.storeOrNil().GetSyncCredential(t.Context()); err == nil {
		t.Error("a credential was stored despite the office being unreachable")
	}
}

func TestHandleSetupEnrollment_RequiresModeFirst_NonSkip(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	rec := c.do(http.MethodPost, "/api/setup/enrollment", setupEnrollmentRequest{OfficeURL: "https://office.example.com", Code: "SOME-CODE"})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d (mode/data-directory setup not done yet)", rec.Code, http.StatusBadRequest)
	}
}
