// SPDX-License-Identifier: AGPL-3.0-only

package sync

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRedeem(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/enroll" || r.Method != http.MethodPost {
			t.Errorf("request = %s %s, want POST /api/enroll", r.Method, r.URL.Path)
		}
		var req redeemRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if req.Code != "THE-REAL-CODE" {
			t.Errorf("Code = %q, want %q", req.Code, "THE-REAL-CODE")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(redeemResponse{Credential: "a-long-lived-token"})
	}))
	defer srv.Close()

	result, err := Redeem(context.Background(), srv.Client(), srv.URL, "THE-REAL-CODE")
	if err != nil {
		t.Fatalf("Redeem: %v", err)
	}
	if result.Credential != "a-long-lived-token" {
		t.Errorf("Credential = %q, want %q", result.Credential, "a-long-lived-token")
	}
	if result.IssuedAt.IsZero() {
		t.Error("IssuedAt is zero")
	}
}

func TestRedeem_TrimsTrailingSlashFromOfficeURL(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(redeemResponse{Credential: "token"})
	}))
	defer srv.Close()

	if _, err := Redeem(context.Background(), srv.Client(), srv.URL+"/", "some-code"); err != nil {
		t.Fatalf("Redeem: %v", err)
	}
	if gotPath != "/api/enroll" {
		t.Errorf("request path = %q, want %q", gotPath, "/api/enroll")
	}
}

func TestRedeem_RejectedCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(errorResponse{Error: "code not recognized or already used"})
	}))
	defer srv.Close()

	_, err := Redeem(context.Background(), srv.Client(), srv.URL, "not-a-real-code")
	if !errors.Is(err, ErrRedeemRejected) {
		t.Errorf("Redeem error = %v, want ErrRedeemRejected", err)
	}
}

func TestRedeem_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := Redeem(context.Background(), srv.Client(), srv.URL, "some-code")
	if err == nil {
		t.Fatal("Redeem: got nil error, want a transport/server error")
	}
	if errors.Is(err, ErrRedeemRejected) {
		t.Error("Redeem(server 500) classified as ErrRedeemRejected, want a distinct error (not a code rejection)")
	}
}

func TestRedeem_EmptyOfficeURL(t *testing.T) {
	if _, err := Redeem(context.Background(), nil, "", "some-code"); err == nil {
		t.Fatal("Redeem(empty office URL) = nil error, want an error")
	}
}

func TestRedeem_EmptyCode(t *testing.T) {
	if _, err := Redeem(context.Background(), nil, "https://office.example.com", ""); err == nil {
		t.Fatal("Redeem(empty code) = nil error, want an error")
	}
}
