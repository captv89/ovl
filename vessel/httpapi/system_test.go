// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"net/http"
	"testing"
)

func TestGetSystemInfo(t *testing.T) {
	_, c := newLoggedInTestServer(t)

	rec := c.do(http.MethodGet, "/api/system/info", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body)
	}
	info := decodeBody[systemInfoResponse](t, rec)
	if info.Version != "test" {
		t.Errorf("Version = %q, want %q", info.Version, "test")
	}
	if len(info.Schemas) == 0 {
		t.Error("Schemas is empty, want the curated OVD schemas")
	}
	var sawLogAbstract bool
	for _, sch := range info.Schemas {
		if sch.SchemaName == "log-abstract" {
			sawLogAbstract = true
			if sch.OvdVersion == "" {
				t.Error("log-abstract OvdVersion is empty")
			}
		}
	}
	if !sawLogAbstract {
		t.Errorf("Schemas = %+v, want it to include log-abstract", info.Schemas)
	}
}

func TestGetSystemInfo_RequiresAuth(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	rec := c.do(http.MethodGet, "/api/system/info", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
