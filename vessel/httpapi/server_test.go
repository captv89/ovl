// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/captv89/ovl/vessel/bootstrap"
)

// newTestServer builds a Server with a scratch bootstrap config path and
// an in-memory placeholder SPA filesystem, unconfigured (as if this
// were the vessel's very first run).
func newTestServer(t *testing.T) *Server {
	t.Helper()
	configPath := filepath.Join(t.TempDir(), "bootstrap.json")
	spa := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html>ovl-vessel</html>")}}
	s := NewServer(configPath, nil, nil, spa, BuildInfo{Version: "test"}, false)
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return s
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

func TestFullFirstRunFlow(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)

	// 1. Before anything: not configured, no master.
	rec := c.do(http.MethodGet, "/api/setup/status", nil)
	status := decodeBody[setupStatusResponse](t, rec)
	if status.Configured || status.HasMaster {
		t.Fatalf("initial status = %+v, want Configured=false HasMaster=false", status)
	}
	if status.DefaultDataDir == "" {
		t.Error("DefaultDataDir is empty, want a suggested path")
	}

	// 2. Wizard step 1: mode + data directory.
	dataDir := filepath.Join(t.TempDir(), "data")
	rec = c.do(http.MethodPost, "/api/setup/mode", setupModeRequest{Mode: bootstrap.ModeStandalone, DataDir: dataDir})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/setup/mode: status %d, body %s", rec.Code, rec.Body)
	}
	status = decodeBody[setupStatusResponse](t, rec)
	if !status.Configured || status.DataDir != dataDir {
		t.Fatalf("status after mode setup = %+v, want Configured=true DataDir=%q", status, dataDir)
	}

	// 3. Wizard step 2: enrollment, explicitly skipped.
	rec = c.do(http.MethodPost, "/api/setup/enrollment", setupEnrollmentRequest{Skip: true})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/setup/enrollment: status %d, body %s", rec.Code, rec.Body)
	}
	status = decodeBody[setupStatusResponse](t, rec)
	if status.Enrollment.Submitted {
		t.Error("Enrollment.Submitted = true after an explicit skip")
	}

	// 4. Wizard step 3: create the Master account.
	rec = c.do(http.MethodPost, "/api/setup/master", setupMasterRequest{Username: "master", Password: "correct horse battery staple"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/setup/master: status %d, body %s", rec.Code, rec.Body)
	}
	created := decodeBody[userView](t, rec)
	if created.Username != "master" || created.Role != "master" || created.MustChangePassword {
		t.Errorf("created user = %+v, want username=master role=master mustChangePassword=false", created)
	}

	// 5. The session from step 3 should already be authenticated.
	rec = c.do(http.MethodGet, "/api/auth/me", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/auth/me after setup: status %d, body %s", rec.Code, rec.Body)
	}
	me := decodeBody[userView](t, rec)
	if me.Username != "master" {
		t.Errorf("me = %+v, want username=master", me)
	}

	// 6. A second Master-bootstrap attempt must be rejected.
	rec = c.do(http.MethodPost, "/api/setup/master", setupMasterRequest{Username: "someone-else", Password: "another long password"})
	if rec.Code != http.StatusConflict {
		t.Errorf("second POST /api/setup/master: status %d, want %d", rec.Code, http.StatusConflict)
	}

	// 7. Logout clears the session.
	rec = c.do(http.MethodPost, "/api/auth/logout", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("POST /api/auth/logout: status %d", rec.Code)
	}
	rec = c.do(http.MethodGet, "/api/auth/me", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("GET /api/auth/me after logout: status %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	// 8. Login with the just-created account.
	rec = c.do(http.MethodPost, "/api/auth/login", loginRequest{Username: "master", Password: "correct horse battery staple"})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/auth/login: status %d, body %s", rec.Code, rec.Body)
	}

	// 9. Change password, then confirm the old one no longer works.
	rec = c.do(http.MethodPost, "/api/auth/change-password", changePasswordRequest{
		CurrentPassword: "correct horse battery staple",
		NewPassword:     "a brand new chosen password",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/auth/change-password: status %d, body %s", rec.Code, rec.Body)
	}
	c.jar = nil // simulate a fresh browser session
	rec = c.do(http.MethodPost, "/api/auth/login", loginRequest{Username: "master", Password: "correct horse battery staple"})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("login with old password after change: status %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	rec = c.do(http.MethodPost, "/api/auth/login", loginRequest{Username: "master", Password: "a brand new chosen password"})
	if rec.Code != http.StatusOK {
		t.Errorf("login with new password: status %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestHandleLogin_UnknownUserAndWrongPasswordLookTheSame(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	dataDir := filepath.Join(t.TempDir(), "data")
	c.do(http.MethodPost, "/api/setup/mode", setupModeRequest{Mode: bootstrap.ModeStandalone, DataDir: dataDir})
	c.do(http.MethodPost, "/api/setup/master", setupMasterRequest{Username: "master", Password: "correct horse battery staple"})
	c.jar = nil

	recUnknown := c.do(http.MethodPost, "/api/auth/login", loginRequest{Username: "nobody", Password: "whatever12345"})
	recWrong := c.do(http.MethodPost, "/api/auth/login", loginRequest{Username: "master", Password: "wrong-password-here"})

	if recUnknown.Code != http.StatusUnauthorized || recWrong.Code != http.StatusUnauthorized {
		t.Fatalf("status codes = (%d, %d), want both %d", recUnknown.Code, recWrong.Code, http.StatusUnauthorized)
	}
	if recUnknown.Body.String() != recWrong.Body.String() {
		t.Errorf("unknown-user body %q != wrong-password body %q, want identical (avoid username enumeration)", recUnknown.Body.String(), recWrong.Body.String())
	}
}

func TestHandleSetupMode_RejectsInvalidMode(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	rec := c.do(http.MethodPost, "/api/setup/mode", setupModeRequest{Mode: "warp-speed", DataDir: t.TempDir()})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleSetupMode_RejectsUnsafeDataDir(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	for _, dataDir := range []string{
		"relative/dir",
		"../escape",
		t.TempDir() + "/../escape",
	} {
		rec := c.do(http.MethodPost, "/api/setup/mode", setupModeRequest{Mode: bootstrap.ModeStandalone, DataDir: dataDir})
		if rec.Code != http.StatusBadRequest {
			t.Errorf("dataDir %q: status = %d, want %d", dataDir, rec.Code, http.StatusBadRequest)
		}
	}
}

func TestHandleSetupEnrollment_RequiresModeFirst(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	rec := c.do(http.MethodPost, "/api/setup/enrollment", setupEnrollmentRequest{Skip: true})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d (setup/mode not done yet)", rec.Code, http.StatusBadRequest)
	}
}

func TestSPAFallback(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	rec := c.do(http.MethodGet, "/some/client/side/route", nil)
	if rec.Code != http.StatusOK || rec.Body.String() != "<html>ovl-vessel</html>" {
		t.Errorf("unmatched route: status %d, body %q, want the embedded index.html", rec.Code, rec.Body.String())
	}
}

func TestSPAFallback_NoBuildOutput(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "bootstrap.json")
	s := NewServer(configPath, nil, nil, fstest.MapFS{".gitkeep": &fstest.MapFile{}}, BuildInfo{Version: "test"}, false)
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	c := newTestClient(t, s)
	rec := c.do(http.MethodGet, "/", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d when no index.html was ever embedded", rec.Code, http.StatusServiceUnavailable)
	}
}
