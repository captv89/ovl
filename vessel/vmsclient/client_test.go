// SPDX-License-Identifier: AGPL-3.0-only

package vmsclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFetchVoyageData(t *testing.T) {
	t.Run("sends bearer auth and the at param, returns the parsed map", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/voyage-data" {
				t.Errorf("path = %s, want /voyage-data", r.URL.Path)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
				t.Errorf("Authorization = %q, want %q", got, "Bearer test-key")
			}
			if at := r.URL.Query().Get("at"); at != "2026-07-05T12:00:00Z" {
				t.Errorf("at = %q, want 2026-07-05T12:00:00Z", at)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"voyageData": {"voyage_number": "V.1", "crew": 21, "cargo_weight_mt": 45000}}`))
		}))
		defer srv.Close()

		c := New(srv.URL, "test-key")
		got, err := c.FetchVoyageData(context.Background(), time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
		if err != nil {
			t.Fatalf("FetchVoyageData: %v", err)
		}
		if got["voyage_number"] != "V.1" {
			t.Errorf("voyage_number = %v, want V.1", got["voyage_number"])
		}
		if got["crew"] != float64(21) {
			t.Errorf("crew = %v, want 21", got["crew"])
		}
	})

	t.Run("non-200 status is an error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer srv.Close()

		c := New(srv.URL, "wrong-key")
		if _, err := c.FetchVoyageData(context.Background(), time.Now()); err == nil {
			t.Error("expected an error for a 401 response, got nil")
		}
	})

	t.Run("oversized response is rejected", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"voyageData": {"x": "` + strings.Repeat("a", maxResponseBytes+1) + `"}}`))
		}))
		defer srv.Close()

		c := New(srv.URL, "test-key")
		if _, err := c.FetchVoyageData(context.Background(), time.Now()); err == nil {
			t.Error("expected an error for an oversized response, got nil")
		}
	})

	t.Run("trims whitespace off baseURL and apiKey", func(t *testing.T) {
		c := New("  http://example.com  ", "  key  ")
		if c.BaseURL != "http://example.com" {
			t.Errorf("BaseURL = %q, want trimmed", c.BaseURL)
		}
		if c.APIKey != "key" {
			t.Errorf("APIKey = %q, want trimmed", c.APIKey)
		}
	})
}
