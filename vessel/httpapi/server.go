// SPDX-License-Identifier: AGPL-3.0-only

// Package httpapi is ovl-vessel's HTTP surface: a small JSON API backing
// the first-run wizard (architecture 9.2) and daily login (design
// handoff A2), plus serving the embedded React SPA. Standalone mode
// only (architecture 9.1) — vessel-server/LAN mode and its concurrent-
// editing concerns (9.5) are Phase 4.
package httpapi

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"sync"

	"github.com/captv89/ovl/internal/httpjson"
	"github.com/captv89/ovl/vessel/auth"
	"github.com/captv89/ovl/vessel/bootstrap"
	"github.com/captv89/ovl/vessel/store"
)

// BuildInfo is the version metadata main.go's ldflags stamp in (or the
// "dev"/"none"/"unknown" defaults for a plain `go run`/`go build`) —
// surfaced read-only via handleGetSystemInfo (design handoff A10's
// diagnostics section).
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// Server holds the mutable bootstrap/store state and routes requests.
// Before wizard step 1 completes, cfg and st are nil; NewServer accepts
// an already-configured cfg/st (e.g. on a restart of a previously set
// up vessel) so this same struct covers both the first-ever run and
// every run after.
type Server struct {
	configPath string
	buildInfo  BuildInfo

	mu  sync.RWMutex
	cfg *bootstrap.Config
	st  *store.Store

	// writeMu serializes every report load->mutate->SaveReport sequence
	// (handleCreateReport, handleSaveSection, handleCheckReport,
	// handleSubmitReport, handleStartCorrection). store.SaveReport UPSERTs
	// a report version's entire fields JSON blob per call, so two
	// concurrent writes to the same report — even to different,
	// individually unlocked sections (architecture 9.5's own worked
	// example: deck locked by 2/O, engine free) — would otherwise race:
	// last writer wins on the whole row, silently discarding the other
	// section's change. Section soft locks (Phase 4 Step 6) don't catch
	// this by themselves since neither section is double-booked; this
	// mutex is what actually makes "section saves are independent
	// writes" (architecture 9.5) hold once more than one officer can
	// write to a report at once. A single mutex, not one per report ID:
	// vessel-scale concurrency (a handful of officers) and cascade
	// already touching multiple reports in a chain per call make
	// per-report striping extra complexity for no real throughput gain.
	writeMu sync.Mutex

	// syncMu guards syncStat independently of mu (cfg/st's lifecycle
	// lock) — sync status updates on every cycle, far more often than
	// cfg/st change, and has no reason to contend with them.
	syncMu   sync.RWMutex
	syncStat syncStatusView

	sessions *sessionStore
	locks    *lockManager
	spa      http.Handler
	mux      *http.ServeMux

	// secureCookies marks the session cookie Secure so browsers only send
	// it back over HTTPS — off by default (plain-HTTP standalone/LAN use
	// is this app's common case, and a Secure cookie is silently dropped
	// there, breaking login) and on when an operator puts TLS in front of
	// vessel-server/LAN mode. Mirrors office/httpapi's own secureCookies.
	secureCookies bool
}

// NewServer wires the API routes and the embedded SPA handler. spa is
// the dist/ subtree (already fs.Sub'd) of the built React app.
func NewServer(configPath string, cfg *bootstrap.Config, st *store.Store, spa fs.FS, buildInfo BuildInfo, secureCookies bool) *Server {
	s := &Server{
		configPath:    configPath,
		buildInfo:     buildInfo,
		cfg:           cfg,
		st:            st,
		sessions:      newSessionStore(),
		locks:         newLockManager(),
		spa:           newSPAHandler(spa),
		secureCookies: secureCookies,
	}
	s.mux = http.NewServeMux()
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/setup/status", s.handleSetupStatus)
	s.mux.HandleFunc("POST /api/setup/mode", s.handleSetupMode)
	s.mux.HandleFunc("POST /api/setup/enrollment", s.handleSetupEnrollment)
	s.mux.HandleFunc("POST /api/setup/master", s.handleSetupMaster)

	s.mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	s.mux.HandleFunc("POST /api/auth/logout", s.handleLogout)
	s.mux.HandleFunc("GET /api/auth/me", s.handleMe)
	s.mux.HandleFunc("POST /api/auth/change-password", s.handleChangePassword)

	s.mux.HandleFunc("GET /api/schemas/{name}", s.handleGetSchema)
	s.mux.HandleFunc("GET /api/event-types", s.handleListEventTypes)
	s.mux.HandleFunc("GET /api/event-suggestions", s.handleListEventSuggestions)
	s.mux.HandleFunc("GET /api/enums/{name}", s.handleGetEnum)

	s.mux.HandleFunc("GET /api/settings/sensor-source", s.handleGetSensorSource)
	s.mux.HandleFunc("PUT /api/settings/sensor-source", s.handleSaveSensorSource)
	s.mux.HandleFunc("POST /api/settings/sensor-source/test", s.handleTestSensorSource)
	s.mux.HandleFunc("GET /api/settings/vms-source", s.handleGetVMSSource)
	s.mux.HandleFunc("PUT /api/settings/vms-source", s.handleSaveVMSSource)
	s.mux.HandleFunc("POST /api/settings/vms-source/test", s.handleTestVMSSource)
	s.mux.HandleFunc("GET /api/vessel-config", s.handleGetVesselConfig)
	s.mux.HandleFunc("GET /api/settings/sync", s.handleGetSyncSettings)
	s.mux.HandleFunc("PUT /api/settings/sync", s.handleSaveSyncSettings)
	s.mux.HandleFunc("POST /api/schemas/{name}/fetch-sensor-data", s.handleFetchSensorData)
	s.mux.HandleFunc("POST /api/schemas/{name}/fetch-vms-data", s.handleFetchVMSData)

	s.mux.HandleFunc("POST /api/reports", s.handleCreateReport)
	s.mux.HandleFunc("GET /api/reports", s.handleListReports)
	s.mux.HandleFunc("GET /api/reports/{id}", s.handleGetReport)
	s.mux.HandleFunc("DELETE /api/reports/{id}", s.handleDeleteReport)
	s.mux.HandleFunc("GET /api/reports/{id}/events", s.handleListReportEvents)
	s.mux.HandleFunc("GET /api/reports/{id}/versions", s.handleListReportVersions)
	s.mux.HandleFunc("GET /api/reports/{id}/chat", s.handleListChat)
	s.mux.HandleFunc("GET /api/reports/{id}/invalidation-notices", s.handleListInvalidationNotices)
	s.mux.HandleFunc("GET /api/reports/{id}/remarks", s.handleListRemarks)
	s.mux.HandleFunc("POST /api/reports/{id}/chat", s.handlePostChat)
	s.mux.HandleFunc("PATCH /api/reports/{id}", s.handleSaveSection)
	s.mux.HandleFunc("POST /api/reports/{id}/validate", s.handleValidateReport)
	s.mux.HandleFunc("POST /api/reports/{id}/check", s.handleCheckReport)
	s.mux.HandleFunc("POST /api/reports/{id}/findings/acknowledge", s.handleAcknowledgeFinding)
	s.mux.HandleFunc("POST /api/reports/{id}/submit", s.handleSubmitReport)
	s.mux.HandleFunc("POST /api/reports/{id}/correction", s.handleStartCorrection)

	s.mux.HandleFunc("POST /api/reports/{id}/attachments", s.handleUploadAttachment)
	s.mux.HandleFunc("GET /api/reports/{id}/attachments", s.handleListAttachments)
	s.mux.HandleFunc("GET /api/reports/{id}/attachments/{attachmentId}", s.handleDownloadAttachment)
	s.mux.HandleFunc("DELETE /api/reports/{id}/attachments/{attachmentId}", s.handleDeleteAttachment)

	s.mux.HandleFunc("GET /api/reports/{id}/locks", s.handleListLocks)
	s.mux.HandleFunc("GET /api/reports/{id}/locks/stream", s.handleLockStream)
	s.mux.HandleFunc("POST /api/reports/{id}/locks/{section}", s.handleAcquireLock)
	s.mux.HandleFunc("DELETE /api/reports/{id}/locks/{section}", s.handleReleaseLock)
	s.mux.HandleFunc("POST /api/reports/{id}/locks/{section}/force-release", s.handleForceReleaseLock)

	s.mux.HandleFunc("GET /api/voyage/current", s.handleGetCurrentVoyage)

	s.mux.HandleFunc("GET /api/sync/status", s.handleSyncStatus)
	s.mux.HandleFunc("POST /api/sync/now", s.handleSyncNow)

	s.mux.HandleFunc("GET /api/admin/backups", s.handleListBackups)
	s.mux.HandleFunc("POST /api/admin/backups", s.handleSnapshotNow)
	s.mux.HandleFunc("POST /api/admin/backups/{id}/restore", s.handleRestoreBackup)
	s.mux.HandleFunc("POST /api/admin/restore-bundle/import", s.handleImportRestoreBundle)

	s.mux.HandleFunc("GET /api/admin/users", s.handleListUsers)
	s.mux.HandleFunc("POST /api/admin/users", s.handleCreateUser)
	s.mux.HandleFunc("POST /api/admin/users/{id}/reset-password", s.handleResetUserPassword)
	s.mux.HandleFunc("PATCH /api/admin/users/{id}", s.handleUpdateUser)

	s.mux.HandleFunc("GET /api/system/info", s.handleGetSystemInfo)

	s.mux.Handle("/", s.spa)
}

// Handler returns the http.Handler to serve.
func (s *Server) Handler() http.Handler {
	return s.mux
}

// Close releases the store, if one is open.
func (s *Server) Close() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.st == nil {
		return nil
	}
	return s.st.Close()
}

// config returns a copy of the current bootstrap config, or nil if
// wizard step 1 hasn't run yet.
func (s *Server) config() *bootstrap.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cfg == nil {
		return nil
	}
	cp := *s.cfg
	return &cp
}

// storeOrNil returns the open store, or nil if wizard step 1 hasn't run
// yet.
func (s *Server) storeOrNil() *store.Store {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.st
}

// requireStore returns the open store, writing a 400 and returning ok=false
// if setup hasn't happened yet.
func (s *Server) requireStore(w http.ResponseWriter) (*store.Store, bool) {
	st := s.storeOrNil()
	if st == nil {
		httpjson.WriteError(w, http.StatusBadRequest, "vessel is not set up yet")
		return nil, false
	}
	return st, true
}

// setConfigured installs cfg and st as the now-active configuration
// (wizard step 1 completing, or step 2's enrollment update) and
// persists cfg to disk.
func (s *Server) setConfigured(cfg *bootstrap.Config, st *store.Store) error {
	if err := bootstrap.Save(s.configPath, cfg); err != nil {
		return err
	}
	s.mu.Lock()
	s.cfg = cfg
	if st != nil {
		s.st = st
	}
	s.mu.Unlock()
	return nil
}

// userView is the JSON shape returned for "the current user" across
// setup/login/me responses.
type userView struct {
	ID                 string    `json:"id"`
	Username           string    `json:"username"`
	Role               auth.Role `json:"role"`
	CanSubmit          bool      `json:"canSubmit"`
	MustChangePassword bool      `json:"mustChangePassword"`
}

func toUserView(u *auth.User) userView {
	return userView{
		ID:                 u.ID,
		Username:           u.Username,
		Role:               u.Role,
		CanSubmit:          u.CanSubmitReports(),
		MustChangePassword: u.MustChangePassword,
	}
}

// authenticatedUser resolves the session cookie on r to a User, or
// writes a 401 and returns ok=false.
func (s *Server) authenticatedUser(w http.ResponseWriter, r *http.Request) (*auth.User, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		httpjson.WriteError(w, http.StatusUnauthorized, "not logged in")
		return nil, false
	}
	userID, ok := s.sessions.lookup(cookie.Value)
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "session expired")
		return nil, false
	}
	st, ok := s.requireStore(w)
	if !ok {
		return nil, false
	}
	u, err := st.GetUser(context.Background(), userID)
	if err != nil {
		httpjson.WriteError(w, http.StatusUnauthorized, "session refers to an unknown user")
		return nil, false
	}
	// Re-checked on every request (not just at login) so a Master
	// deactivating a user kills that user's live session immediately,
	// not just their next login attempt.
	if !u.Active {
		httpjson.WriteError(w, http.StatusUnauthorized, "this account has been deactivated")
		return nil, false
	}
	return u, true
}

func (s *Server) startSession(w http.ResponseWriter, userID string) error {
	token, err := s.sessions.create(userID)
	if err != nil {
		return fmt.Errorf("start session: %w", err)
	}
	http.SetCookie(w, s.sessionCookie(token, int(sessionTTL.Seconds())))
	return nil
}

// sessionCookie builds the session cookie so login and logout stay in
// lockstep on every attribute — a logout cookie must match the login
// cookie's Secure/HttpOnly/SameSite/Path to reliably overwrite it.
func (s *Server) sessionCookie(value string, maxAge int) *http.Cookie {
	return &http.Cookie{ // #nosec G124 -- Secure is s.secureCookies (see its own doc comment), not a hardcoded false; gosec can't verify a runtime bool
		Name:     sessionCookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.secureCookies,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   maxAge,
	}
}
