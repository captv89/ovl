// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"testing/fstest"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/captv89/ovl/office/auth"
	"github.com/captv89/ovl/office/store"
	"github.com/captv89/ovl/pkg/attachmentstore"
	"github.com/captv89/ovl/pkg/schema"
	ovlschemas "github.com/captv89/ovl/schemas"
)

// testDSN returns the Postgres connection string for integration tests,
// skipping the test if none is configured — same gating as
// office/store's own tests. See deploy/office/docker-compose.yml for a
// local instance to point OVL_TEST_DATABASE_URL at.
func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("OVL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("OVL_TEST_DATABASE_URL not set; skipping Postgres integration test")
	}
	return dsn
}

// newTestServer opens a real Store against the shared test Postgres
// (office/store, unlike vessel/store, has no "unconfigured" state to
// stand in for — a DSN is always required) and an in-memory placeholder
// SPA filesystem.
func newTestServer(t *testing.T) *Server {
	t.Helper()
	st, err := store.Open(testDSN(t))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	spa := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html>ovl-office</html>")}}
	validator, err := schema.NewValidator(ovlschemas.FS, "meta-schema.json")
	if err != nil {
		t.Fatalf("schema.NewValidator: %v", err)
	}
	attachments, err := attachmentstore.New(t.TempDir())
	if err != nil {
		t.Fatalf("attachmentstore.New: %v", err)
	}
	s := NewServer(st, spa, validator, attachments, t.TempDir(), "test", false)
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return s
}

// createTestUser inserts a user directly through the store (bypassing
// handleSetupAdmin, which only ever succeeds once against an empty
// users table — not a safe assumption in this shared, long-lived test
// database) and registers cleanup via a raw connection, since
// office/store exposes no DeleteUser and its own *sql.DB is unexported
// outside the store package.
func createTestUser(t *testing.T, s *Server, roles auth.Roles, password string) *auth.User {
	t.Helper()
	u, err := auth.NewUser(t.Name(), password, roles)
	if err != nil {
		t.Fatalf("auth.NewUser: %v", err)
	}
	u.MustChangePassword = false
	if err := s.st.CreateUser(context.Background(), u); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	t.Cleanup(func() {
		raw, err := sql.Open("pgx", testDSN(t))
		if err != nil {
			t.Errorf("cleanup: open raw connection: %v", err)
			return
		}
		defer func() { _ = raw.Close() }()
		if _, err := raw.ExecContext(context.Background(), `DELETE FROM users WHERE id = $1`, u.ID); err != nil {
			t.Errorf("cleanup: delete test user %s: %v", u.ID, err)
		}
	})
	return u
}

type testClient struct {
	t      *testing.T
	server *Server
	jar    []*http.Cookie
}

func newTestClient(t *testing.T, s *Server) *testClient {
	return &testClient{t: t, server: s}
}

func (c *testClient) do(method, path string, body any) *httptest.ResponseRecorder {
	c.t.Helper()
	var reader *bytes.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			c.t.Fatalf("marshal request body: %v", err)
		}
		reader = bytes.NewReader(data)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	for _, ck := range c.jar {
		req.AddCookie(ck)
	}
	rec := httptest.NewRecorder()
	c.server.Handler().ServeHTTP(rec, req)
	if cks := rec.Result().Cookies(); len(cks) > 0 {
		c.jar = cks
	}
	return rec
}

func decodeBody[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode response body %q: %v", rec.Body.String(), err)
	}
	return v
}

func TestFullLoginFlow(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	u := createTestUser(t, s, auth.Roles{auth.RoleAdmin}, "correct horse battery staple")

	// 1. Login.
	rec := c.do(http.MethodPost, "/api/auth/login", loginRequest{Username: u.Username, Password: "correct horse battery staple"})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/auth/login: status %d, body %s", rec.Code, rec.Body)
	}
	logged := decodeBody[userView](t, rec)
	if logged.Username != u.Username || len(logged.Roles) != 1 || logged.Roles[0] != auth.RoleAdmin {
		t.Errorf("login response = %+v, want username=%q roles=[admin]", logged, u.Username)
	}

	// 2. The session should already be authenticated.
	rec = c.do(http.MethodGet, "/api/auth/me", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/auth/me: status %d, body %s", rec.Code, rec.Body)
	}
	me := decodeBody[userView](t, rec)
	if me.Username != u.Username {
		t.Errorf("me = %+v, want username=%q", me, u.Username)
	}

	// 3. Logout clears the session.
	rec = c.do(http.MethodPost, "/api/auth/logout", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("POST /api/auth/logout: status %d", rec.Code)
	}
	rec = c.do(http.MethodGet, "/api/auth/me", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("GET /api/auth/me after logout: status %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	// 4. Login again, then change password, then confirm the old one no
	// longer works and the new one does.
	rec = c.do(http.MethodPost, "/api/auth/login", loginRequest{Username: u.Username, Password: "correct horse battery staple"})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/auth/login (again): status %d, body %s", rec.Code, rec.Body)
	}
	rec = c.do(http.MethodPost, "/api/auth/change-password", changePasswordRequest{
		CurrentPassword: "correct horse battery staple",
		NewPassword:     "a brand new chosen password",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/auth/change-password: status %d, body %s", rec.Code, rec.Body)
	}
	c.jar = nil // simulate a fresh browser session
	rec = c.do(http.MethodPost, "/api/auth/login", loginRequest{Username: u.Username, Password: "correct horse battery staple"})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("login with old password after change: status %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	rec = c.do(http.MethodPost, "/api/auth/login", loginRequest{Username: u.Username, Password: "a brand new chosen password"})
	if rec.Code != http.StatusOK {
		t.Errorf("login with new password: status %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestHandleLogin_UnknownUserAndWrongPasswordLookTheSame(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	u := createTestUser(t, s, auth.Roles{auth.RoleViewer}, "correct horse battery staple")

	recUnknown := c.do(http.MethodPost, "/api/auth/login", loginRequest{Username: "nobody-" + u.Username, Password: "whatever12345"})
	recWrong := c.do(http.MethodPost, "/api/auth/login", loginRequest{Username: u.Username, Password: "wrong-password-here"})

	if recUnknown.Code != http.StatusUnauthorized || recWrong.Code != http.StatusUnauthorized {
		t.Fatalf("status codes = (%d, %d), want both %d", recUnknown.Code, recWrong.Code, http.StatusUnauthorized)
	}
	if recUnknown.Body.String() != recWrong.Body.String() {
		t.Errorf("unknown-user body %q != wrong-password body %q, want identical (avoid username enumeration)", recUnknown.Body.String(), recWrong.Body.String())
	}
}

func TestHandleSetupAdmin_RejectsWhenAUserAlreadyExists(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	createTestUser(t, s, auth.Roles{auth.RoleAdmin}, "correct horse battery staple")

	rec := c.do(http.MethodPost, "/api/setup/admin", setupAdminRequest{Username: "someone-else", Password: "another long password"})
	if rec.Code != http.StatusConflict {
		t.Errorf("POST /api/setup/admin with an existing user: status %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestHandleSetupStatus_ReportsHasAnyUser(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	createTestUser(t, s, auth.Roles{auth.RoleAdmin}, "correct horse battery staple")

	rec := c.do(http.MethodGet, "/api/setup/status", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/setup/status: status %d, body %s", rec.Code, rec.Body)
	}
	status := decodeBody[setupStatusResponse](t, rec)
	if !status.HasAnyUser {
		t.Error("HasAnyUser = false with at least one user in the store, want true")
	}
}

func TestSPAFallback(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	rec := c.do(http.MethodGet, "/some/client/side/route", nil)
	if rec.Code != http.StatusOK || rec.Body.String() != "<html>ovl-office</html>" {
		t.Errorf("unmatched route: status %d, body %q, want the embedded index.html", rec.Code, rec.Body.String())
	}
}
