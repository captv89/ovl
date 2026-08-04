// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/captv89/ovl/office/auth"
)

// TestHandleAPIKeys_CreateListRevoke exercises the Administration > API
// Access management surface end to end: an Admin issues a key (reveal-
// once, matching handleCreateUser's own temporary-password contract), a
// non-admin is forbidden, the list includes it, and revoking it is
// reflected back.
func TestHandleAPIKeys_CreateListRevoke(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	admin := createTestUser(t, s, auth.Roles{auth.RoleAdmin}, "correct horse battery staple")
	loginAs(t, c, admin, "correct horse battery staple")

	t.Run("non-admin is forbidden", func(t *testing.T) {
		viewer := createTestUser2(t, s, auth.Roles{auth.RoleViewer}, "correct horse battery staple")
		vc := newTestClient(t, s)
		loginAs(t, vc, viewer, "correct horse battery staple")
		rec := vc.do(http.MethodPost, "/api/api-keys", createAPIKeyRequest{Label: "Should be forbidden"})
		if rec.Code != http.StatusForbidden {
			t.Errorf("POST /api/api-keys as viewer: status = %d, want %d", rec.Code, http.StatusForbidden)
		}
	})

	rec := c.do(http.MethodPost, "/api/api-keys", createAPIKeyRequest{Label: "Acme Verifier"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/api-keys: status = %d, want %d, body %s", rec.Code, http.StatusCreated, rec.Body)
	}
	created := decodeBody[createAPIKeyResponse](t, rec)
	if created.Token == "" {
		t.Fatal("Token is empty, want a generated value")
	}
	if created.APIKey.Label != "Acme Verifier" {
		t.Errorf("Label = %q, want %q", created.APIKey.Label, "Acme Verifier")
	}
	if created.APIKey.CreatedBy != admin.Username {
		t.Errorf("CreatedBy = %q, want %q", created.APIKey.CreatedBy, admin.Username)
	}
	if created.APIKey.RevokedAt != nil {
		t.Error("RevokedAt is set on a freshly created key, want nil")
	}

	listRec := c.do(http.MethodGet, "/api/api-keys", nil)
	list := decodeBody[[]apiKeyView](t, listRec)
	var found bool
	for _, k := range list {
		if k.ID == created.APIKey.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("GET /api/api-keys = %+v, want the newly created key listed", list)
	}

	revokeRec := c.do(http.MethodPost, "/api/api-keys/"+created.APIKey.ID+"/revoke", nil)
	if revokeRec.Code != http.StatusOK {
		t.Fatalf("POST revoke: status = %d, want %d, body %s", revokeRec.Code, http.StatusOK, revokeRec.Body)
	}

	afterRec := c.do(http.MethodGet, "/api/api-keys", nil)
	after := decodeBody[[]apiKeyView](t, afterRec)
	for _, k := range after {
		if k.ID == created.APIKey.ID {
			if k.RevokedAt == nil {
				t.Error("RevokedAt is nil after revoking, want a timestamp")
			}
		}
	}
}

// TestAuthenticatedAPIKey exercises the bearer-token auth check the data
// API (Phase 3) will gate its routes with — separate from and parallel
// to authenticatedUser's session-cookie check.
func TestAuthenticatedAPIKey(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	admin := createTestUser(t, s, auth.Roles{auth.RoleAdmin}, "correct horse battery staple")
	loginAs(t, c, admin, "correct horse battery staple")

	rec := c.do(http.MethodPost, "/api/api-keys", createAPIKeyRequest{Label: "Acme Verifier"})
	created := decodeBody[createAPIKeyResponse](t, rec)

	t.Run("valid bearer token authenticates", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/graphql", nil)
		req.Header.Set("Authorization", "Bearer "+created.Token)
		w := httptest.NewRecorder()
		k, ok := s.authenticatedAPIKey(w, req)
		if !ok {
			t.Fatalf("authenticatedAPIKey(valid token) ok = false, body %s", w.Body)
		}
		if k.ID != created.APIKey.ID {
			t.Errorf("authenticated key ID = %q, want %q", k.ID, created.APIKey.ID)
		}
	})

	t.Run("missing header rejected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/graphql", nil)
		w := httptest.NewRecorder()
		if _, ok := s.authenticatedAPIKey(w, req); ok {
			t.Error("authenticatedAPIKey(no header) ok = true, want false")
		}
		if w.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
		}
	})

	t.Run("garbage token rejected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/graphql", nil)
		req.Header.Set("Authorization", "Bearer not-a-real-key")
		w := httptest.NewRecorder()
		if _, ok := s.authenticatedAPIKey(w, req); ok {
			t.Error("authenticatedAPIKey(garbage token) ok = true, want false")
		}
		if w.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
		}
	})

	t.Run("revoked key rejected", func(t *testing.T) {
		revokeRec := c.do(http.MethodPost, "/api/api-keys/"+created.APIKey.ID+"/revoke", nil)
		if revokeRec.Code != http.StatusOK {
			t.Fatalf("revoke: status = %d", revokeRec.Code)
		}
		req := httptest.NewRequest(http.MethodGet, "/api/v1/graphql", nil)
		req.Header.Set("Authorization", "Bearer "+created.Token)
		w := httptest.NewRecorder()
		if _, ok := s.authenticatedAPIKey(w, req); ok {
			t.Error("authenticatedAPIKey(revoked token) ok = true, want false")
		}
	})
}

// TestHandleAPIKeys_DeleteRequiresRevoked exercises the redesigned API
// Access screen's Delete action (2026-07-25 redesign): a live key can't
// be deleted (409, would silently break whatever integration still
// holds it), a non-admin is forbidden, and once revoked the delete
// actually removes it.
func TestHandleAPIKeys_DeleteRequiresRevoked(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	admin := createTestUser(t, s, auth.Roles{auth.RoleAdmin}, "correct horse battery staple")
	loginAs(t, c, admin, "correct horse battery staple")

	keyRec := c.do(http.MethodPost, "/api/api-keys", createAPIKeyRequest{Label: "Delete test key"})
	created := decodeBody[createAPIKeyResponse](t, keyRec)

	t.Run("non-admin is forbidden", func(t *testing.T) {
		viewer := createTestUser2(t, s, auth.Roles{auth.RoleViewer}, "correct horse battery staple")
		vc := newTestClient(t, s)
		loginAs(t, vc, viewer, "correct horse battery staple")
		rec := vc.do(http.MethodDelete, "/api/api-keys/"+created.APIKey.ID, nil)
		if rec.Code != http.StatusForbidden {
			t.Errorf("DELETE as viewer: status = %d, want %d", rec.Code, http.StatusForbidden)
		}
	})

	t.Run("still active is a 409", func(t *testing.T) {
		rec := c.do(http.MethodDelete, "/api/api-keys/"+created.APIKey.ID, nil)
		if rec.Code != http.StatusConflict {
			t.Errorf("DELETE (active key): status = %d, want %d, body %s", rec.Code, http.StatusConflict, rec.Body)
		}
	})

	revokeRec := c.do(http.MethodPost, "/api/api-keys/"+created.APIKey.ID+"/revoke", nil)
	if revokeRec.Code != http.StatusOK {
		t.Fatalf("revoke: status = %d", revokeRec.Code)
	}

	t.Run("revoked key deletes", func(t *testing.T) {
		rec := c.do(http.MethodDelete, "/api/api-keys/"+created.APIKey.ID, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("DELETE (revoked key): status = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body)
		}
		listRec := c.do(http.MethodGet, "/api/api-keys", nil)
		list := decodeBody[[]apiKeyView](t, listRec)
		for _, k := range list {
			if k.ID == created.APIKey.ID {
				t.Error("GET /api/api-keys still lists the deleted key")
			}
		}
	})

	t.Run("unknown id is a 404", func(t *testing.T) {
		rec := c.do(http.MethodDelete, "/api/api-keys/"+created.APIKey.ID, nil)
		if rec.Code != http.StatusNotFound {
			t.Errorf("DELETE (already deleted): status = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})
}

// TestHandleAPIKeys_Events covers the per-key activity-log panel: created
// and revoked land as events (newest first), a data-API call adds a
// usedGraphQL event, and an unknown key id is a 404 rather than an empty
// list.
func TestHandleAPIKeys_Events(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	admin := createTestUser(t, s, auth.Roles{auth.RoleAdmin}, "correct horse battery staple")
	loginAs(t, c, admin, "correct horse battery staple")

	keyRec := c.do(http.MethodPost, "/api/api-keys", createAPIKeyRequest{Label: "Events test key"})
	created := decodeBody[createAPIKeyResponse](t, keyRec)

	eventsRec := c.do(http.MethodGet, "/api/api-keys/"+created.APIKey.ID+"/events", nil)
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("GET events (after create): status = %d, body %s", eventsRec.Code, eventsRec.Body)
	}
	events := decodeBody[[]apiKeyEventView](t, eventsRec)
	if len(events) != 1 || events[0].Kind != "created" {
		t.Fatalf("events after create = %+v, want exactly one \"created\" event", events)
	}

	doGraphQL(t, s, created.Token, `{ reports { items { reportId } } }`, nil)

	revokeRec := c.do(http.MethodPost, "/api/api-keys/"+created.APIKey.ID+"/revoke", nil)
	if revokeRec.Code != http.StatusOK {
		t.Fatalf("revoke: status = %d", revokeRec.Code)
	}

	eventsRec = c.do(http.MethodGet, "/api/api-keys/"+created.APIKey.ID+"/events", nil)
	events = decodeBody[[]apiKeyEventView](t, eventsRec)
	if len(events) != 3 {
		t.Fatalf("events after graphql use + revoke = %d, want 3, got %+v", len(events), events)
	}
	// Newest first: revoked, then usedGraphQL, then created.
	if events[0].Kind != "revoked" || events[1].Kind != "usedGraphQL" || events[2].Kind != "created" {
		t.Errorf("event kinds = [%q, %q, %q], want [revoked, usedGraphQL, created]", events[0].Kind, events[1].Kind, events[2].Kind)
	}

	t.Run("unknown id is a 404", func(t *testing.T) {
		rec := c.do(http.MethodGet, "/api/api-keys/00000000-0000-0000-0000-000000000000/events", nil)
		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})

	t.Run("non-admin is forbidden", func(t *testing.T) {
		viewer := createTestUser2(t, s, auth.Roles{auth.RoleViewer}, "correct horse battery staple")
		vc := newTestClient(t, s)
		loginAs(t, vc, viewer, "correct horse battery staple")
		rec := vc.do(http.MethodGet, "/api/api-keys/"+created.APIKey.ID+"/events", nil)
		if rec.Code != http.StatusForbidden {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
		}
	})
}
