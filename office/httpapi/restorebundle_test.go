// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/captv89/ovl/office/auth"
	"github.com/captv89/ovl/office/compliance"
	"github.com/captv89/ovl/office/configbundle"
	"github.com/captv89/ovl/pkg/backupcrypto"
	"github.com/captv89/ovl/pkg/configwire"
	"github.com/captv89/ovl/pkg/domain"
	"github.com/captv89/ovl/pkg/restorebundle"
)

// createTestVesselWithEnrollment mirrors enroll_test.go's
// issueTestEnrollment, but as a helper the caller's own admin client
// drives — issueTestEnrollment creates its own throwaway admin (also
// named t.Name()), which collides with a test that also needs to call
// createTestUser itself.
func createTestVesselWithEnrollment(t *testing.T, c *testClient, first int) (vesselID, code string) {
	t.Helper()
	rec := c.do(http.MethodPost, "/api/vessels", createVesselRequest{
		IMO: distinctIMOForHTTP(t, first), Name: "MV DR Test", Type: "Bulk Carrier",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/vessels: status %d, body %s", rec.Code, rec.Body)
	}
	created := decodeBody[vesselView](t, rec)
	t.Cleanup(func() {
		raw, err := sql.Open("pgx", testDSN(t))
		if err != nil {
			return
		}
		defer func() { _ = raw.Close() }()
		_, _ = raw.ExecContext(context.Background(), `DELETE FROM vessels WHERE id = $1`, created.ID)
	})

	rec = c.do(http.MethodPost, "/api/vessels/"+created.ID+"/enrollment/issue", issueEnrollmentRequest{})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST .../enrollment/issue: status %d, body %s", rec.Code, rec.Body)
	}
	return created.ID, decodeBody[issueResultView](t, rec).Code
}

// TestHandleGenerateRestoreBundle_EncryptsAgainstDRPublicKey is
// architecture 12.5's DR exit criterion exercised at office's own HTTP
// surface: once a vessel has redeemed enrollment with a DR public key on
// file, an Admin's download decrypts (with that vessel's matching
// private key) to a Bundle carrying exactly the report/chat data landed
// for it — the same shape vessel/httpapi's own
// TestHandleImportRestoreBundle_Roundtrip proves importable on the other
// side.
func TestHandleGenerateRestoreBundle_EncryptsAgainstDRPublicKey(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	admin := createTestUser(t, s, auth.Roles{auth.RoleAdmin}, "correct horse battery staple")
	loginAs(t, c, admin, "correct horse battery staple")
	vesselID, code := createTestVesselWithEnrollment(t, c, 80)

	identity, err := backupcrypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}
	anon := newTestClient(t, s)
	rec := anon.do(http.MethodPost, "/api/enroll", redeemEnrollmentRequest{Code: code, DRPublicKey: identity.PublicKey})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/enroll: status %d, body %s", rec.Code, rec.Body)
	}

	landTestReport(t, s, vesselID, "report-dr-1", 1, time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), domain.StateSubmitted)

	// Architecture 12.5: "all reports, config, chat and attachments" — a
	// resolved config bundle assignment must ride along inside the
	// restore bundle itself, same as office/syncservice's own
	// TestPullInbox_AssignedConfigBundle sets one up.
	cb := configbundle.Compose(nil, nil, nil, nil, nil, []string{"master"})
	published, err := configbundle.Publish(cb, "DR Test Bundle", "admin")
	if err != nil {
		t.Fatalf("configbundle.Publish: %v", err)
	}
	if err := s.st.CreateConfigBundle(context.Background(), published); err != nil {
		t.Fatalf("CreateConfigBundle: %v", err)
	}
	vesselScope, err := compliance.VesselScope(vesselID)
	if err != nil {
		t.Fatalf("VesselScope: %v", err)
	}
	assignment, err := configbundle.NewBundleAssignment(vesselScope, published.ID)
	if err != nil {
		t.Fatalf("NewBundleAssignment: %v", err)
	}
	if err := s.st.SaveBundleAssignment(context.Background(), assignment); err != nil {
		t.Fatalf("SaveBundleAssignment: %v", err)
	}

	rec = c.do(http.MethodGet, "/api/vessels/"+vesselID+"/restore-bundle", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET restore-bundle: status %d, body %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/octet-stream" {
		t.Errorf("Content-Type = %q, want application/octet-stream", ct)
	}

	plaintext, err := backupcrypto.Decrypt(rec.Body.Bytes(), identity.PrivateKey)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	var bundle restorebundle.Bundle
	if err := json.Unmarshal(plaintext, &bundle); err != nil {
		t.Fatalf("unmarshal bundle: %v", err)
	}
	if bundle.VesselID != vesselID {
		t.Errorf("VesselID = %q, want %q", bundle.VesselID, vesselID)
	}
	if len(bundle.Reports) != 1 || bundle.Reports[0].ReportID != "report-dr-1" {
		t.Fatalf("Reports = %+v, want exactly report-dr-1", bundle.Reports)
	}
	if len(bundle.Reports[0].Versions) != 1 || bundle.Reports[0].Versions[0].Fields["IMO"] != float64(9074729) {
		t.Errorf("Versions = %+v, want one version with IMO=9074729", bundle.Reports[0].Versions)
	}
	if bundle.ConfigBundle == nil || bundle.ConfigBundle.BundleID != published.ID {
		t.Fatalf("ConfigBundle = %+v, want the resolved bundle %q embedded", bundle.ConfigBundle, published.ID)
	}
	decodedConfig, err := configwire.Decode(bundle.ConfigBundle.ContentJSON)
	if err != nil {
		t.Fatalf("configwire.Decode ConfigBundle.ContentJSON: %v", err)
	}
	if len(decodedConfig.DefaultRoleNames) != 1 || decodedConfig.DefaultRoleNames[0] != "master" {
		t.Errorf("decoded config bundle = %+v, want DefaultRoleNames=[master]", decodedConfig)
	}
}

// TestHandlePushRestoreBundle_QueuesAndSurfacesStatus is design handoff
// B2's DR tab "push to vessel" action (architecture 12.5/11.2's DR push
// path): queuing a command shows up in the same vessel detail fetch's
// RestoreCommands with a default reason when none was given, and an
// explicit reason when one was. The actual fetched/applied lifecycle
// (office/syncservice's FetchRestoreBundle + SyncStatus ack) is covered
// by office/syncservice's own tests — this only needs to prove the DR
// tab's own queue+status surface.
func TestHandlePushRestoreBundle_QueuesAndSurfacesStatus(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	admin := createTestUser(t, s, auth.Roles{auth.RoleAdmin}, "correct horse battery staple")
	loginAs(t, c, admin, "correct horse battery staple")
	vesselID, code := createTestVesselWithEnrollment(t, c, 86)

	identity, err := backupcrypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}
	anon := newTestClient(t, s)
	rec := anon.do(http.MethodPost, "/api/enroll", redeemEnrollmentRequest{Code: code, DRPublicKey: identity.PublicKey})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/enroll: status %d, body %s", rec.Code, rec.Body)
	}

	rec = c.do(http.MethodPost, "/api/vessels/"+vesselID+"/restore-bundle/push", nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("push (no reason): status %d, body %s", rec.Code, rec.Body)
	}
	queued := decodeBody[restoreCommandView](t, rec)
	if queued.Reason != defaultPushRestoreBundleReason {
		t.Errorf("Reason = %q, want default %q", queued.Reason, defaultPushRestoreBundleReason)
	}
	if queued.IssuedBy != admin.Username {
		t.Errorf("IssuedBy = %q, want %q", queued.IssuedBy, admin.Username)
	}

	rec = c.do(http.MethodPost, "/api/vessels/"+vesselID+"/restore-bundle/push", pushRestoreBundleRequest{Reason: "power outage"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("push (with reason): status %d, body %s", rec.Code, rec.Body)
	}
	queued2 := decodeBody[restoreCommandView](t, rec)
	if queued2.Reason != "power outage" {
		t.Errorf("Reason = %q, want %q", queued2.Reason, "power outage")
	}

	rec = c.do(http.MethodGet, "/api/vessels/"+vesselID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET vessel: status %d, body %s", rec.Code, rec.Body)
	}
	detail := decodeBody[vesselDetailView](t, rec)
	if len(detail.RestoreCommands) != 2 {
		t.Fatalf("RestoreCommands = %+v, want 2 (newest first)", detail.RestoreCommands)
	}
	if detail.RestoreCommands[0].ID != queued2.ID || detail.RestoreCommands[1].ID != queued.ID {
		t.Errorf("RestoreCommands order = %+v, want newest (%s) first", detail.RestoreCommands, queued2.ID)
	}
	if detail.RestoreCommands[0].FetchedAt != nil || detail.RestoreCommands[0].AppliedAt != nil {
		t.Errorf("RestoreCommands[0] = %+v, want neither fetched nor applied yet", detail.RestoreCommands[0])
	}
}

func TestHandlePushRestoreBundle_RequiresDRPublicKey(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	admin := createTestUser(t, s, auth.Roles{auth.RoleAdmin}, "correct horse battery staple")
	loginAs(t, c, admin, "correct horse battery staple")
	vesselID, _ := createTestVesselWithEnrollment(t, c, 87)

	rec := c.do(http.MethodPost, "/api/vessels/"+vesselID+"/restore-bundle/push", nil)
	if rec.Code != http.StatusConflict {
		t.Errorf("push before enrollment redeemed a DR key: status %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestHandlePushRestoreBundle_RequiresAdmin(t *testing.T) {
	s := newTestServer(t)
	admin := newTestClient(t, s)
	loginAs(t, admin, createTestUser(t, s, auth.Roles{auth.RoleAdmin}, "correct horse battery staple"), "correct horse battery staple")
	vesselID, _ := createTestVesselWithEnrollment(t, admin, 88)

	c := newTestClient(t, s)
	viewer := createTestUser2(t, s, auth.Roles{auth.RoleViewer}, "correct horse battery staple")
	loginAs(t, c, viewer, "correct horse battery staple")

	rec := c.do(http.MethodPost, "/api/vessels/"+vesselID+"/restore-bundle/push", nil)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestHandleGenerateRestoreBundle_RequiresDRPublicKey(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	admin := createTestUser(t, s, auth.Roles{auth.RoleAdmin}, "correct horse battery staple")
	loginAs(t, c, admin, "correct horse battery staple")
	vesselID, _ := createTestVesselWithEnrollment(t, c, 81)

	rec := c.do(http.MethodGet, "/api/vessels/"+vesselID+"/restore-bundle", nil)
	if rec.Code != http.StatusConflict {
		t.Errorf("GET restore-bundle before enrollment redeemed a DR key: status %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestHandleGenerateRestoreBundle_RequiresAdmin(t *testing.T) {
	s := newTestServer(t)
	admin := newTestClient(t, s)
	loginAs(t, admin, createTestUser(t, s, auth.Roles{auth.RoleAdmin}, "correct horse battery staple"), "correct horse battery staple")
	vesselID, _ := createTestVesselWithEnrollment(t, admin, 82)

	c := newTestClient(t, s)
	viewer := createTestUser2(t, s, auth.Roles{auth.RoleViewer}, "correct horse battery staple")
	loginAs(t, c, viewer, "correct horse battery staple")

	rec := c.do(http.MethodGet, "/api/vessels/"+vesselID+"/restore-bundle", nil)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}
