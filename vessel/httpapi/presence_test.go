// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/captv89/ovl/vessel/auth"
)

// loginSecondOfficer creates and logs in a Second Officer (no canSubmit
// override) against s, returning a client authenticated as them —
// mirrors TestHandleSubmitReport_RequiresReadyAndPermission's own
// second-user setup, used here so lock tests have two distinct real
// identities to contend with each other.
func loginSecondOfficer(t *testing.T, s *Server) *testClient {
	t.Helper()
	st := s.storeOrNil()
	officer, err := auth.NewUser("second-officer", "another long password", auth.RoleSecondOfficer)
	if err != nil {
		t.Fatalf("auth.NewUser: %v", err)
	}
	if err := st.CreateUser(context.Background(), officer); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	c2 := newTestClient(t, s)
	if rec := c2.do(http.MethodPost, "/api/auth/login", loginRequest{Username: "second-officer", Password: "another long password"}); rec.Code != http.StatusOK {
		t.Fatalf("login as second-officer: status %d, body %s", rec.Code, rec.Body)
	}
	return c2
}

func TestHandleAcquireLock_Success(t *testing.T) {
	_, c := newLoggedInTestServer(t)
	created := createBunkerReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC))

	rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/locks/header", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("acquire: status %d, body %s", rec.Code, rec.Body)
	}
	lock := decodeBody[lockView](t, rec)
	if lock.Section != "header" || lock.Username != "master" || lock.Role != auth.RoleMaster {
		t.Errorf("lock = %+v, want section=header username=master role=master", lock)
	}
	if !lock.ExpiresAt.After(lock.AcquiredAt) {
		t.Errorf("ExpiresAt (%v) should be after AcquiredAt (%v)", lock.ExpiresAt, lock.AcquiredAt)
	}
}

func TestHandleAcquireLock_ConflictWithAnotherHolder(t *testing.T) {
	s, c := newLoggedInTestServer(t)
	created := createBunkerReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC))
	if rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/locks/header", nil); rec.Code != http.StatusOK {
		t.Fatalf("first acquire: status %d, body %s", rec.Code, rec.Body)
	}

	c2 := loginSecondOfficer(t, s)
	rec := c2.do(http.MethodPost, "/api/reports/"+created.ReportID+"/locks/header", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("second officer's acquire: status %d, want %d, body %s", rec.Code, http.StatusConflict, rec.Body)
	}
	conflict := decodeBody[lockConflictResponse](t, rec)
	if conflict.Lock.Username != "master" {
		t.Errorf("conflict.Lock.Username = %q, want master", conflict.Lock.Username)
	}
}

func TestHandleAcquireLock_UnknownSection(t *testing.T) {
	_, c := newLoggedInTestServer(t)
	created := createBunkerReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC))
	rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/locks/not-a-real-section", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleAcquireLock_RejectsOnSubmittedReport(t *testing.T) {
	_, c := newLoggedInTestServer(t)
	created := createTestReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), commercialPeriodFields())
	if rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/check", nil); rec.Code != http.StatusOK {
		t.Fatalf("check: status %d, body %s", rec.Code, rec.Body)
	}
	if rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/submit", nil); rec.Code != http.StatusOK {
		t.Fatalf("submit: status %d, body %s", rec.Code, rec.Body)
	}
	rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/locks/details", nil)
	if rec.Code != http.StatusConflict {
		t.Errorf("acquire on a submitted report: status %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestHandleReleaseLock_OnlyHolderReleases(t *testing.T) {
	s, c := newLoggedInTestServer(t)
	created := createBunkerReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC))
	if rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/locks/header", nil); rec.Code != http.StatusOK {
		t.Fatalf("acquire: status %d, body %s", rec.Code, rec.Body)
	}

	c2 := loginSecondOfficer(t, s)
	if rec := c2.do(http.MethodDelete, "/api/reports/"+created.ReportID+"/locks/header", nil); rec.Code != http.StatusNoContent {
		t.Errorf("non-holder release: status %d, want %d (idempotent no-op)", rec.Code, http.StatusNoContent)
	}
	// Still held by master: a third acquire attempt by the second officer still conflicts.
	if rec := c2.do(http.MethodPost, "/api/reports/"+created.ReportID+"/locks/header", nil); rec.Code != http.StatusConflict {
		t.Errorf("acquire after non-holder's release attempt: status %d, want %d (lock should be untouched)", rec.Code, http.StatusConflict)
	}

	if rec := c.do(http.MethodDelete, "/api/reports/"+created.ReportID+"/locks/header", nil); rec.Code != http.StatusNoContent {
		t.Errorf("holder release: status %d, want %d", rec.Code, http.StatusNoContent)
	}
	if rec := c2.do(http.MethodPost, "/api/reports/"+created.ReportID+"/locks/header", nil); rec.Code != http.StatusOK {
		t.Errorf("acquire after the real holder released: status %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestHandleForceReleaseLock_RequiresMaster(t *testing.T) {
	s, c := newLoggedInTestServer(t)
	created := createBunkerReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC))
	c2 := loginSecondOfficer(t, s)
	if rec := c2.do(http.MethodPost, "/api/reports/"+created.ReportID+"/locks/header", nil); rec.Code != http.StatusOK {
		t.Fatalf("second officer acquire: status %d, body %s", rec.Code, rec.Body)
	}

	// Non-Master force-release attempt: forbidden.
	if rec := c2.do(http.MethodPost, "/api/reports/"+created.ReportID+"/locks/header/force-release", nil); rec.Code != http.StatusForbidden {
		t.Errorf("non-Master force-release: status %d, want %d", rec.Code, http.StatusForbidden)
	}

	// Master succeeds, even without holding it themselves.
	if rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/locks/header/force-release", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("Master force-release: status %d, want %d", rec.Code, http.StatusNoContent)
	}
	// And it's idempotent.
	if rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/locks/header/force-release", nil); rec.Code != http.StatusNoContent {
		t.Errorf("repeat force-release of an already-unlocked section: status %d, want %d", rec.Code, http.StatusNoContent)
	}
	// The section is genuinely free now.
	if rec := c2.do(http.MethodPost, "/api/reports/"+created.ReportID+"/locks/header", nil); rec.Code != http.StatusOK {
		t.Errorf("acquire after force-release: status %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestHandleListLocks_ReturnsActiveOnly(t *testing.T) {
	s, c := newLoggedInTestServer(t)
	created := createBunkerReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC))
	c2 := loginSecondOfficer(t, s)

	c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/locks/header", nil)
	c2.do(http.MethodPost, "/api/reports/"+created.ReportID+"/locks/fuelProperties", nil)

	rec := c.do(http.MethodGet, "/api/reports/"+created.ReportID+"/locks", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list locks: status %d, body %s", rec.Code, rec.Body)
	}
	locks := decodeBody[[]lockView](t, rec)
	if len(locks) != 2 {
		t.Fatalf("len(locks) = %d, want 2", len(locks))
	}
	if locks[0].Section != "fuelProperties" || locks[0].Username != "second-officer" {
		t.Errorf("locks[0] = %+v, want fuelProperties held by second-officer (sorted by section)", locks[0])
	}
	if locks[1].Section != "header" || locks[1].Username != "master" {
		t.Errorf("locks[1] = %+v, want header held by master", locks[1])
	}
}

func TestHandleSaveSection_LockEnforcement(t *testing.T) {
	s, c := newLoggedInTestServer(t)
	created := createBunkerReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC))
	c2 := loginSecondOfficer(t, s)

	if rec := c2.do(http.MethodPost, "/api/reports/"+created.ReportID+"/locks/header", nil); rec.Code != http.StatusOK {
		t.Fatalf("second officer acquires header: status %d, body %s", rec.Code, rec.Body)
	}

	// Master (not the holder) is rejected trying to save the locked section.
	rec := c.do(http.MethodPatch, "/api/reports/"+created.ReportID, saveSectionRequest{
		Section: "header",
		Changes: map[string]any{"BDN_Number": "should-not-land"},
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("master saving locked-by-other section: status %d, want %d, body %s", rec.Code, http.StatusConflict, rec.Body)
	}
	conflict := decodeBody[lockConflictResponse](t, rec)
	if conflict.Lock.Username != "second-officer" {
		t.Errorf("conflict.Lock.Username = %q, want second-officer", conflict.Lock.Username)
	}

	// The holder can save their own locked section.
	rec = c2.do(http.MethodPatch, "/api/reports/"+created.ReportID, saveSectionRequest{
		Section: "header",
		Changes: map[string]any{"BDN_Number": "held-by-holder"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("holder saving their own locked section: status %d, body %s", rec.Code, rec.Body)
	}

	// Master can freely save the unlocked section.
	rec = c.do(http.MethodPatch, "/api/reports/"+created.ReportID, saveSectionRequest{
		Section: "fuelProperties",
		Changes: map[string]any{"Sustainability": "cert-A"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("master saving an unlocked section: status %d, body %s", rec.Code, rec.Body)
	}
}

// TestHandleLockStream_SnapshotThenLiveEvents can't use the recorder-
// based testClient — SSE needs an incrementally-readable real
// connection — so it drives a real net/http.Client against a real
// httptest.NewServer instead of the ResponseRecorder harness every
// other test in this package uses.
func TestHandleLockStream_SnapshotThenLiveEvents(t *testing.T) {
	s, c := newLoggedInTestServer(t)
	created := createBunkerReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC))
	// Establish one lock before the stream connects, to prove the
	// initial snapshot burst works, not just live events.
	if rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/locks/header", nil); rec.Code != http.StatusOK {
		t.Fatalf("acquire header before streaming: status %d, body %s", rec.Code, rec.Body)
	}

	httpServer := httptest.NewServer(s.Handler())
	defer httpServer.Close()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}
	client := &http.Client{Jar: jar}
	// Reuse the already-authenticated session cookie from the recorder-based
	// client rather than logging in again against the real server.
	u, err := url.Parse(httpServer.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	jar.SetCookies(u, c.jar)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, httpServer.URL+"/api/reports/"+created.ReportID+"/locks/stream", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET locks/stream: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("locks/stream: status %d", resp.StatusCode)
	}

	reader := bufio.NewReader(resp.Body)
	readFrame := func() (event string, data string) {
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				t.Fatalf("read SSE frame: %v", err)
			}
			line = strings.TrimRight(line, "\n")
			switch {
			case strings.HasPrefix(line, "event: "):
				event = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				data = strings.TrimPrefix(line, "data: ")
			case line == "":
				if event != "" {
					return event, data
				}
				// A blank line with no event (a keepalive comment) — keep reading.
			}
		}
	}

	event, data := readFrame()
	if event != "locked" {
		t.Fatalf("initial frame event = %q, want locked (the pre-existing snapshot)", event)
	}
	var snapshot lockView
	if err := json.Unmarshal([]byte(data), &snapshot); err != nil {
		t.Fatalf("decode snapshot frame: %v", err)
	}
	if snapshot.Section != "header" {
		t.Errorf("snapshot frame section = %q, want header", snapshot.Section)
	}

	// Now trigger a live event: acquire the second section via the
	// existing recorder-based client, then confirm the stream sees it.
	if rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/locks/fuelProperties", nil); rec.Code != http.StatusOK {
		t.Fatalf("acquire fuelProperties: status %d, body %s", rec.Code, rec.Body)
	}
	event, data = readFrame()
	if event != "locked" {
		t.Fatalf("live frame event = %q, want locked", event)
	}
	var live lockView
	if err := json.Unmarshal([]byte(data), &live); err != nil {
		t.Fatalf("decode live frame: %v", err)
	}
	if live.Section != "fuelProperties" {
		t.Errorf("live frame section = %q, want fuelProperties", live.Section)
	}
}
