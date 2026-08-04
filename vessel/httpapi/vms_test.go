// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHandleVMSSource_GetSaveRoundtrip(t *testing.T) {
	s, c := newLoggedInTestServer(t)

	t.Run("unconfigured returns a zero view", func(t *testing.T) {
		rec := c.do(http.MethodGet, "/api/settings/vms-source", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d, body %s", rec.Code, rec.Body)
		}
		got := decodeBody[vmsSourceView](t, rec)
		if got.Configured {
			t.Errorf("Configured = true before any save, want false")
		}
	})

	t.Run("non-master is forbidden", func(t *testing.T) {
		c2 := newSecondOfficerClient(t, s)
		rec := c2.do(http.MethodPut, "/api/settings/vms-source", saveVMSSourceRequest{BaseURL: "http://example.com", APIKey: "secret-key-1234", Enabled: true})
		if rec.Code != http.StatusForbidden {
			t.Errorf("status %d, want %d", rec.Code, http.StatusForbidden)
		}
	})

	rec := c.do(http.MethodPut, "/api/settings/vms-source", saveVMSSourceRequest{BaseURL: "http://vms.example.com", APIKey: "secret-key-1234", Enabled: true})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT: status %d, body %s", rec.Code, rec.Body)
	}
	saved := decodeBody[vmsSourceView](t, rec)
	if !saved.Configured || saved.BaseURL != "http://vms.example.com" || saved.APIKey == "secret-key-1234" {
		t.Errorf("saved view = %+v, want Configured=true, BaseURL unmasked, APIKey masked", saved)
	}

	getRec := c.do(http.MethodGet, "/api/settings/vms-source", nil)
	got := decodeBody[vmsSourceView](t, getRec)
	if !got.Configured || got.BaseURL != "http://vms.example.com" {
		t.Errorf("GET after save = %+v, want Configured=true, BaseURL=http://vms.example.com", got)
	}
}

func TestHandleFetchVMSData(t *testing.T) {
	vmsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-vms-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"voyageData": {
			"voyage_number": "V.1", "voyage_type": "One way", "crew": 21, "cargo_weight_mt": 45000,
			"eta": "2026-08-09T12:00:00Z"
		}}`))
	}))
	defer vmsSrv.Close()

	_, c := newLoggedInTestServer(t)

	t.Run("no source configured is a conflict", func(t *testing.T) {
		rec := c.do(http.MethodPost, "/api/schemas/log-abstract/fetch-vms-data", fetchVMSDataRequest{EventTime: time.Now().UTC()})
		if rec.Code != http.StatusConflict {
			t.Errorf("status %d, want %d", rec.Code, http.StatusConflict)
		}
	})

	if rec := c.do(http.MethodPut, "/api/settings/vms-source", saveVMSSourceRequest{BaseURL: vmsSrv.URL, APIKey: "test-vms-key", Enabled: true}); rec.Code != http.StatusOK {
		t.Fatalf("save vms source: status %d, body %s", rec.Code, rec.Body)
	}

	t.Run("unsupported schema has no mapping", func(t *testing.T) {
		rec := c.do(http.MethodPost, "/api/schemas/bunker-report/fetch-vms-data", fetchVMSDataRequest{EventTime: time.Now().UTC()})
		if rec.Code != http.StatusNotFound {
			t.Errorf("status %d, want %d", rec.Code, http.StatusNotFound)
		}
	})

	t.Run("fetches and maps voyage/cargo fields", func(t *testing.T) {
		rec := c.do(http.MethodPost, "/api/schemas/log-abstract/fetch-vms-data", fetchVMSDataRequest{EventTime: time.Now().UTC()})
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d, body %s", rec.Code, rec.Body)
		}
		got := decodeBody[fetchVMSDataResponse](t, rec)

		if got.Fields["Voyage_Number"] != "V.1" {
			t.Errorf("Fields[Voyage_Number] = %v, want V.1", got.Fields["Voyage_Number"])
		}
		if got.Fields["Voyage_Type"] != "One way" {
			t.Errorf("Fields[Voyage_Type] = %v, want \"One way\"", got.Fields["Voyage_Type"])
		}
		if got.Fields["Crew"] != float64(21) {
			t.Errorf("Fields[Crew] = %v, want 21", got.Fields["Crew"])
		}
		// rta/carrier_code/etc. were never in the fake VMS's response —
		// must not appear as spurious fields.
		if _, ok := got.Fields["RTA"]; ok {
			t.Error("Fields[RTA] present, want absent (not in the VMS's response)")
		}
		// C3 fix: the VMS sends eta as RFC3339 ("2026-08-09T12:00:00Z"),
		// but this app's canonical stored dateTime field format is
		// dateTimeFieldLayout ("2006-01-02 15:04", voyage.go) — the
		// mapping boundary in vms.go must convert, not pass the raw
		// RFC3339 string through, or Check/Submit's field.format rule
		// and computeVoyageSummary's own ETA parse both break.
		wantETA := "2026-08-09 12:00"
		if got.Fields["ETA"] != wantETA {
			t.Errorf("Fields[ETA] = %v, want %q (dateTimeFieldLayout, converted from RFC3339)", got.Fields["ETA"], wantETA)
		}
	})
}

func TestHandleTestVMSSource(t *testing.T) {
	vmsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"voyageData": {"voyage_number": "V.1"}}`))
	}))
	defer vmsSrv.Close()

	_, c := newLoggedInTestServer(t)

	t.Run("successful probe", func(t *testing.T) {
		rec := c.do(http.MethodPost, "/api/settings/vms-source/test", testVMSSourceRequest{BaseURL: vmsSrv.URL, APIKey: "good-key"})
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d, body %s", rec.Code, rec.Body)
		}
		got := decodeBody[testVMSSourceResponse](t, rec)
		if !got.OK {
			t.Errorf("OK = false, message %q", got.Message)
		}
	})

	t.Run("wrong key reports failure without a 4xx/5xx", func(t *testing.T) {
		rec := c.do(http.MethodPost, "/api/settings/vms-source/test", testVMSSourceRequest{BaseURL: vmsSrv.URL, APIKey: "wrong-key"})
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d, want %d", rec.Code, http.StatusOK)
		}
		got := decodeBody[testVMSSourceResponse](t, rec)
		if got.OK {
			t.Error("OK = true for a wrong key, want false")
		}
	})
}
