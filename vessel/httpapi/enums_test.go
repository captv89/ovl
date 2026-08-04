// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"net/http"
	"testing"
)

func TestHandleGetEnum(t *testing.T) {
	_, c := newLoggedInTestServer(t)
	rec := c.do(http.MethodGet, "/api/enums/operational-modes", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/enums/operational-modes: status %d, body %s", rec.Code, rec.Body)
	}
	got := decodeBody[enumValuesView](t, rec)
	want := []string{"InPort", "AtSea", "Sailing"}
	if len(got.Values) != len(want) {
		t.Fatalf("values = %v, want %v", got.Values, want)
	}
	for i := range want {
		if got.Values[i] != want[i] {
			t.Fatalf("values[%d] = %q, want %q", i, got.Values[i], want[i])
		}
	}
}

func TestHandleGetEnum_UnknownEnumRef(t *testing.T) {
	_, c := newLoggedInTestServer(t)
	rec := c.do(http.MethodGet, "/api/enums/offshore-modes", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET /api/enums/offshore-modes: status %d, want 404", rec.Code)
	}
}
