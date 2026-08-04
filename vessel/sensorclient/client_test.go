// SPDX-License-Identifier: AGPL-3.0-only

package sensorclient

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClient_FetchReadings(t *testing.T) {
	t.Run("sends the expected request and parses readings", func(t *testing.T) {
		var gotPath, gotAuth, gotFrom, gotTo string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			gotAuth = r.Header.Get("Authorization")
			gotFrom = r.URL.Query().Get("from")
			gotTo = r.URL.Query().Get("to")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"readings": {"latitude": 12.34, "speed_kn": 14.2}}`))
		}))
		defer srv.Close()

		c := New(srv.URL, "test-key")
		from := time.Date(2026, 7, 18, 6, 0, 0, 0, time.UTC)
		to := time.Date(2026, 7, 19, 6, 0, 0, 0, time.UTC)
		readings, err := c.FetchReadings(t.Context(), from, to)
		if err != nil {
			t.Fatalf("FetchReadings: %v", err)
		}

		if gotPath != "/readings" {
			t.Errorf("path = %q, want /readings", gotPath)
		}
		if gotAuth != "Bearer test-key" {
			t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer test-key")
		}
		if gotFrom != "2026-07-18T06:00:00Z" || gotTo != "2026-07-19T06:00:00Z" {
			t.Errorf("from/to = %q/%q, want RFC3339 UTC timestamps", gotFrom, gotTo)
		}
		if readings["latitude"] != 12.34 || readings["speed_kn"] != 14.2 {
			t.Errorf("readings = %+v, want latitude=12.34 speed_kn=14.2", readings)
		}
	})

	t.Run("non-200 status is an error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer srv.Close()

		c := New(srv.URL, "bad-key")
		_, err := c.FetchReadings(t.Context(), time.Now(), time.Now())
		if err == nil {
			t.Fatal("FetchReadings: got nil error, want one for a 401 response")
		}
	})

	t.Run("malformed JSON is an error, not a panic", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`not json`))
		}))
		defer srv.Close()

		c := New(srv.URL, "key")
		_, err := c.FetchReadings(t.Context(), time.Now(), time.Now())
		if err == nil {
			t.Fatal("FetchReadings: got nil error, want one for malformed JSON")
		}
	})

	t.Run("trailing whitespace on the base URL is tolerated", func(t *testing.T) {
		// Regression: a base URL saved with a trailing space (e.g.
		// "http://127.0.0.1:8422 ") used to fail with an opaque
		// url.Parse error ("invalid port \":8422 \" after host") instead
		// of connecting — New trims both fields now, so a config saved
		// before that fix (or pasted with stray whitespace) still works.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"readings": {"latitude": 1}}`))
		}))
		defer srv.Close()

		c := New(srv.URL+" ", " test-key ")
		if _, err := c.FetchReadings(t.Context(), time.Now(), time.Now()); err != nil {
			t.Fatalf("FetchReadings with a whitespace-padded base URL/key: %v", err)
		}
	})

	t.Run("oversized response is rejected", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"readings": {"x": 1}, "padding": "` + strings.Repeat("a", maxResponseBytes) + `"}`))
		}))
		defer srv.Close()

		c := New(srv.URL, "key")
		_, err := c.FetchReadings(t.Context(), time.Now(), time.Now())
		if err == nil {
			t.Fatal("FetchReadings: got nil error, want one for an oversized response")
		}
	})
}
