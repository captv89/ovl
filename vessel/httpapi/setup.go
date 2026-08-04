// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/captv89/ovl/internal/httpjson"
	"github.com/captv89/ovl/vessel/auth"
	"github.com/captv89/ovl/vessel/bootstrap"
	"github.com/captv89/ovl/vessel/store"
	ovlsync "github.com/captv89/ovl/vessel/sync"
)

// setupStatusResponse is the shape the wizard polls on load to decide
// which step to show.
type setupStatusResponse struct {
	Configured     bool                 `json:"configured"`
	Mode           bootstrap.Mode       `json:"mode"`
	DataDir        string               `json:"dataDir"`
	DefaultDataDir string               `json:"defaultDataDir"`
	Enrollment     bootstrap.Enrollment `json:"enrollment"`
	HasMaster      bool                 `json:"hasMaster"`
}

func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	defaultDir, err := bootstrap.DefaultDataDir()
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := setupStatusResponse{DefaultDataDir: defaultDir}

	cfg := s.config()
	if cfg != nil {
		resp.Configured = true
		resp.Mode = cfg.Mode
		resp.DataDir = cfg.DataDir
		resp.Enrollment = cfg.Enrollment
	}

	if st := s.storeOrNil(); st != nil {
		hasMaster, err := st.HasAnyUser(r.Context())
		if err != nil {
			httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		resp.HasMaster = hasMaster
	}

	httpjson.WriteJSON(w, http.StatusOK, resp)
}

type setupModeRequest struct {
	Mode    bootstrap.Mode `json:"mode"`
	DataDir string         `json:"dataDir"`
}

// handleSetupMode is wizard step 1 (architecture 9.2: "Choose mode ...
// and data directory"). It opens (or creates) the SQLite store at
// DataDir to prove it's usable before persisting the choice.
func (s *Server) handleSetupMode(w http.ResponseWriter, r *http.Request) {
	var req setupModeRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !req.Mode.Valid() {
		httpjson.WriteError(w, http.StatusBadRequest, "mode must be \"standalone\" or \"server\"")
		return
	}
	req.DataDir = strings.TrimSpace(req.DataDir)
	if req.DataDir == "" {
		httpjson.WriteError(w, http.StatusBadRequest, "dataDir is required")
		return
	}

	st, err := store.Open(req.DataDir)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "could not open a data store at that path: "+err.Error())
		return
	}

	cfg := s.config()
	if cfg == nil {
		cfg = &bootstrap.Config{}
	}
	cfg.Mode = req.Mode
	cfg.DataDir = req.DataDir
	if err := s.setConfigured(cfg, st); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.handleSetupStatus(w, r)
}

type setupEnrollmentRequest struct {
	OfficeURL string `json:"officeURL"`
	Code      string `json:"code"`
	Skip      bool   `json:"skip"`
}

// handleSetupEnrollment is wizard step 2. A non-skip submission now
// actually redeems the code against the office (architecture 11.2's sync
// handshake, vessel/sync.Redeem) and stores the returned long-lived
// credential — Enrollment.Submitted means "the office validated this and
// issued a credential," not just "the operator went through this step."
// A rejected or unreachable office is reported back as an error rather
// than silently recorded, so the wizard can't show "Enrolled" for a code
// that never actually worked. Explicitly skipping is still equally valid
// (design handoff A1: "Offline install (enrollment deferred) must be
// possible") and bypasses all of this.
func (s *Server) handleSetupEnrollment(w http.ResponseWriter, r *http.Request) {
	cfg := s.config()
	if !cfg.Configured() {
		httpjson.WriteError(w, http.StatusBadRequest, "complete mode/data-directory setup first")
		return
	}
	var req setupEnrollmentRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Skip {
		cfg.Enrollment = bootstrap.Enrollment{}
		if err := s.setConfigured(cfg, nil); err != nil {
			httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.handleSetupStatus(w, r)
		return
	}

	st, ok := s.requireStore(w)
	if !ok {
		return
	}
	officeURL := strings.TrimSpace(req.OfficeURL)
	code := strings.TrimSpace(req.Code)

	result, err := ovlsync.Redeem(r.Context(), nil, officeURL, code)
	if err != nil {
		status := http.StatusBadGateway // could not reach/parse a response from the office
		if errors.Is(err, ovlsync.ErrRedeemRejected) {
			status = http.StatusUnauthorized // office reached, code refused
		}
		httpjson.WriteError(w, status, "could not enroll with the office: "+err.Error())
		return
	}
	if err := st.SaveSyncCredential(r.Context(), &store.SyncCredential{Token: result.Credential, IssuedAt: result.IssuedAt}); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := st.SaveDRIdentity(r.Context(), &store.DRIdentity{PublicKey: result.DRPublicKey, PrivateKey: result.DRPrivateKey, IssuedAt: result.IssuedAt}); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := st.SaveVesselIdentity(r.Context(), &store.VesselIdentity{Name: result.VesselName, IMO: result.VesselIMO}); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Code is not persisted to bootstrap.json past this point: it is a
	// one-time secret the office has already invalidated on its side
	// (office/enrollment.Redeem clears CodeHash), so keeping a copy here
	// is needless residue — nothing in the UI reads it back either
	// (only OfficeURL/Submitted are displayed, see SettingsScreen.tsx).
	cfg.Enrollment = bootstrap.Enrollment{OfficeURL: officeURL, Submitted: true}
	if err := s.setConfigured(cfg, nil); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Run a sync cycle now rather than waiting for the next scheduler tick
	// (vessel/main.go's runSyncScheduler only fires every syncInterval,
	// 5 minutes) — otherwise syncStatusView.Enrolled stays false and
	// Home's "Sync now" button (disabled on exactly that flag) is stuck
	// unusable for up to 5 minutes right after the operator just finished
	// enrolling (2026-07-14 manual-test feedback: reports "looked" synced
	// but weren't reaching the office — root-caused to this dead window).
	// The office is already known-reachable (Redeem just talked to it
	// above), so this is cheap; its result is discarded here and simply
	// left for the next GET /api/sync/status poll to pick up.
	s.RunSyncCycle(r.Context())

	s.handleSetupStatus(w, r)
}

type setupMasterRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// handleSetupMaster is wizard step 3 for the offline/deferred-enrollment
// path: since no office issued a pre-set Master credential to log in
// with (that's the enrolled path, Phase 4), the operator chooses the
// Master account's real password here directly. Because they are
// choosing it themselves, live, MustChangePassword is left false —
// unlike auth.NewUser's normal default of true, which assumes a
// temporary password handed to someone else (see PROJECT.md's decisions
// log for the reasoning).
func (s *Server) handleSetupMaster(w http.ResponseWriter, r *http.Request) {
	st, ok := s.requireStore(w)
	if !ok {
		return
	}
	hasMaster, err := st.HasAnyUser(r.Context())
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if hasMaster {
		httpjson.WriteError(w, http.StatusConflict, "a user already exists; use the login screen instead")
		return
	}

	var req setupMasterRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	u, err := auth.NewUser(req.Username, req.Password, auth.RoleMaster)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	u.MustChangePassword = false

	if err := st.CreateUser(context.Background(), u); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.startSession(w, u.ID); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, toUserView(u))
}
