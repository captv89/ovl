// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/captv89/ovl/vessel/auth"
)

// TestBackupLifecycle_SnapshotListAndRestore exercises architecture 9.6's
// full local DR loop end to end: snapshot a report into existence,
// change it, restore, and confirm the change is gone — the same property
// vessel/store's own TestRestoreDatabase_RevertsToSnapshotPoint proves at
// the store layer, exercised here through the actual HTTP surface
// (including the write-lock close/reopen in restoreFromSnapshot).
func TestBackupLifecycle_SnapshotListAndRestore(t *testing.T) {
	_, c := newLoggedInTestServer(t)
	created := createTestReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), commercialPeriodFields())

	rec := c.do(http.MethodPost, "/api/admin/backups", nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("snapshot now: status %d, body %s", rec.Code, rec.Body)
	}
	snap := decodeBody[backupInfo](t, rec)
	if snap.ID == "" {
		t.Fatal("snapshot has no ID")
	}

	// Change the report after the snapshot.
	if rec := c.do(http.MethodPatch, "/api/reports/"+created.ReportID, saveSectionRequest{Section: "all", Changes: map[string]any{"Description": "post-snapshot edit"}}); rec.Code != http.StatusOK {
		t.Fatalf("save-section: status %d", rec.Code)
	}

	rec = c.do(http.MethodGet, "/api/admin/backups", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list backups: status %d, body %s", rec.Code, rec.Body)
	}
	list := decodeBody[[]backupInfo](t, rec)
	found := false
	for _, b := range list {
		if b.ID == snap.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("list backups = %+v, want it to include %s", list, snap.ID)
	}

	// Restore requires confirm=true.
	if rec := c.do(http.MethodPost, "/api/admin/backups/"+snap.ID+"/restore", restoreRequest{Confirm: false}); rec.Code != http.StatusBadRequest {
		t.Errorf("restore without confirm: status %d, want %d", rec.Code, http.StatusBadRequest)
	}

	rec = c.do(http.MethodPost, "/api/admin/backups/"+snap.ID+"/restore", restoreRequest{Confirm: true})
	if rec.Code != http.StatusOK {
		t.Fatalf("restore: status %d, body %s", rec.Code, rec.Body)
	}

	rec = c.do(http.MethodGet, "/api/reports/"+created.ReportID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get report after restore: status %d, body %s", rec.Code, rec.Body)
	}
	got := decodeBody[reportView](t, rec)
	if got.Fields["Description"] != nil {
		t.Errorf("Fields[Description] = %v after restore, want unset (reverted to pre-edit snapshot)", got.Fields["Description"])
	}
}

func TestBackupEndpoints_RequireSuperAdmin(t *testing.T) {
	s, _ := newLoggedInTestServer(t)
	st := s.storeOrNil()
	officer, err := auth.NewUser("second-officer", "another long password", auth.RoleSecondOfficer)
	if err != nil {
		t.Fatalf("auth.NewUser: %v", err)
	}
	if err := st.CreateUser(context.Background(), officer); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	c2 := newTestClient(t, s)
	if rec := c2.do(http.MethodPost, "/api/auth/login", loginRequest{Username: "second-officer", Password: "another long password"}); rec.Code != http.StatusOK {
		t.Fatalf("login as second-officer: status %d, body %s", rec.Code, rec.Body)
	}

	if rec := c2.do(http.MethodGet, "/api/admin/backups", nil); rec.Code != http.StatusForbidden {
		t.Errorf("list backups as non-Master: status %d, want %d", rec.Code, http.StatusForbidden)
	}
	if rec := c2.do(http.MethodPost, "/api/admin/backups", nil); rec.Code != http.StatusForbidden {
		t.Errorf("snapshot as non-Master: status %d, want %d", rec.Code, http.StatusForbidden)
	}
	if rec := c2.do(http.MethodPost, "/api/admin/backups/anything/restore", restoreRequest{Confirm: true}); rec.Code != http.StatusForbidden {
		t.Errorf("restore as non-Master: status %d, want %d", rec.Code, http.StatusForbidden)
	}
}
