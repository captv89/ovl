// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"net/http"
	"testing"
)

func TestHandleSyncSettings_GetSaveRoundtrip(t *testing.T) {
	s, c := newLoggedInTestServer(t)

	t.Run("default before any save", func(t *testing.T) {
		rec := c.do(http.MethodGet, "/api/settings/sync", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d, body %s", rec.Code, rec.Body)
		}
		got := decodeBody[syncSettingsView](t, rec)
		if got.SyncIntervalSeconds != defaultSyncIntervalConst {
			t.Errorf("SyncIntervalSeconds = %d, want %d (migration-seeded default)", got.SyncIntervalSeconds, defaultSyncIntervalConst)
		}
	})

	t.Run("non-master is forbidden", func(t *testing.T) {
		c2 := newSecondOfficerClient(t, s)
		rec := c2.do(http.MethodPut, "/api/settings/sync", saveSyncSettingsRequest{SyncIntervalSeconds: 60})
		if rec.Code != http.StatusForbidden {
			t.Errorf("status %d, want %d", rec.Code, http.StatusForbidden)
		}
	})

	t.Run("out of range is rejected", func(t *testing.T) {
		rec := c.do(http.MethodPut, "/api/settings/sync", saveSyncSettingsRequest{SyncIntervalSeconds: 5})
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status %d, want %d (below min)", rec.Code, http.StatusBadRequest)
		}
		rec = c.do(http.MethodPut, "/api/settings/sync", saveSyncSettingsRequest{SyncIntervalSeconds: 100000000})
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status %d, want %d (above max)", rec.Code, http.StatusBadRequest)
		}
	})

	rec := c.do(http.MethodPut, "/api/settings/sync", saveSyncSettingsRequest{SyncIntervalSeconds: 60})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT: status %d, body %s", rec.Code, rec.Body)
	}
	saved := decodeBody[syncSettingsView](t, rec)
	if saved.SyncIntervalSeconds != 60 {
		t.Errorf("SyncIntervalSeconds = %d, want 60", saved.SyncIntervalSeconds)
	}

	rec = c.do(http.MethodGet, "/api/settings/sync", nil)
	got := decodeBody[syncSettingsView](t, rec)
	if got.SyncIntervalSeconds != 60 {
		t.Errorf("SyncIntervalSeconds after reload = %d, want 60", got.SyncIntervalSeconds)
	}

	if got := s.SyncIntervalDuration(); got.Seconds() != 60 {
		t.Errorf("SyncIntervalDuration() = %v, want 60s", got)
	}
}

const defaultSyncIntervalConst = 300
