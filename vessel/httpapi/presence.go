// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/captv89/ovl/internal/httpjson"
	"github.com/captv89/ovl/pkg/schema"
	ovlschemas "github.com/captv89/ovl/schemas"
	"github.com/captv89/ovl/vessel/auth"
)

// lockView is the JSON shape for one active section soft lock — returned
// by acquire/list and carried in every SSE "locked" event.
type lockView struct {
	Section    string    `json:"section"`
	UserID     string    `json:"userId"`
	Username   string    `json:"username"`
	Role       auth.Role `json:"role"`
	AcquiredAt time.Time `json:"acquiredAt"`
	ExpiresAt  time.Time `json:"expiresAt"`
}

func toLockView(l sectionLock) lockView {
	return lockView{
		Section:    l.Section,
		UserID:     l.UserID,
		Username:   l.Username,
		Role:       l.Role,
		AcquiredAt: l.AcquiredAt,
		ExpiresAt:  l.RenewedAt.Add(lockTTL),
	}
}

// lockConflictResponse is the 409 body for both handleAcquireLock and
// handleSaveSection's lockConflict enforcement — the same shape either
// way, so the frontend has one type to handle regardless of which
// endpoint rejected it.
type lockConflictResponse struct {
	Error string   `json:"error"`
	Lock  lockView `json:"lock"`
}

// loadSchemaFor loads schemaName's curated, validated schema — the same
// two-step validator+schema.Load evaluateReport needs, factored out so
// section-lock validation doesn't duplicate it a third time.
func (s *Server) loadSchemaFor(schemaName string) (*schema.Schema, error) {
	validator, err := getSchemaValidator()
	if err != nil {
		return nil, err
	}
	sch, err := schema.Load(ovlschemas.FS, "ovd-3.13/"+schemaName+".json", validator)
	if err != nil {
		return nil, fmt.Errorf("load schema %q: %w", schemaName, err)
	}
	return sch, nil
}

// schemaHasSection reports whether any field of sch belongs to section —
// checked against the fields themselves (every field.Section is always
// populated) rather than the schema's own Sections list, which is just
// declared display ordering and, per the meta-schema, optional.
func schemaHasSection(sch *schema.Schema, section string) bool {
	for _, f := range sch.Fields {
		if f.Section == section {
			return true
		}
	}
	return false
}

// broadcastLock fans a lock-table change out to every SSE subscriber of
// reportID's lock stream.
func (s *Server) broadcastLock(reportID, eventType string, lock sectionLock) {
	s.locks.broadcast(reportID, lockEvent{Type: eventType, Lock: lock})
}

// SweepExpiredLocks releases every section soft lock that's expired due
// to inactivity (architecture 9.5's "5 minutes... default") and
// broadcasts each release live, so an idle lock's expiry is visible on
// its report's lock stream immediately rather than only the next time
// something happens to touch it via acquire/release/holder. Called on a
// timer (vessel/main.go's runLockSweepScheduler); no context.Context
// param, unlike RunNightlySnapshot/RunSyncCycle — this does no I/O.
func (s *Server) SweepExpiredLocks() {
	for _, lock := range s.locks.sweep(time.Now().UTC()) {
		s.broadcastLock(lock.ReportID, "unlocked", lock)
	}
}

// handleListLocks returns a snapshot of reportID's active section soft
// locks — used for a client's initial paint before its SSE stream
// connects (handleLockStream also emits this same snapshot itself on
// connect, so a caller doesn't strictly have to call both, but this
// exists as a plain point-in-time read for anything that only wants
// that).
func (s *Server) handleListLocks(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStore(w); !ok {
		return
	}
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	locks := s.locks.snapshot(r.PathValue("id"))
	views := make([]lockView, len(locks))
	for i, l := range locks {
		views[i] = toLockView(l)
	}
	httpjson.WriteJSON(w, http.StatusOK, views)
}

// writeLockEvent writes one SSE frame. "unlocked" carries only the
// section name — the client already has everything else about a lock
// it used to hold in its own state.
func writeLockEvent(w http.ResponseWriter, eventType string, lock sectionLock) {
	var data any
	if eventType == "unlocked" {
		data = struct {
			Section string `json:"section"`
		}{Section: lock.Section}
	} else {
		data = toLockView(lock)
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, payload)
}

// lockStreamKeepalive is how often a comment frame is sent on an
// otherwise-idle SSE connection, so intermediary proxies/browsers don't
// time it out.
const lockStreamKeepalive = 20 * time.Second

// handleLockStream is architecture 9.5's live presence over SSE: on
// connect, it emits reportID's current lock snapshot as a burst of
// "locked" events (so a client connecting mid-session sees existing
// holders without a separate racing REST call), then streams live
// "locked"/"unlocked" events as they happen, plus a keepalive comment
// frame on lockStreamKeepalive.
func (s *Server) handleLockStream(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStore(w); !ok {
		return
	}
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		httpjson.WriteError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}
	reportID := r.PathValue("id")

	ch, cancel := s.locks.subscribe(reportID)
	defer cancel()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	for _, lock := range s.locks.snapshot(reportID) {
		writeLockEvent(w, "locked", lock)
	}
	flusher.Flush()

	keepalive := time.NewTicker(lockStreamKeepalive)
	defer keepalive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-ch:
			writeLockEvent(w, ev.Type, ev.Lock)
			flusher.Flush()
		case <-keepalive.C:
			_, _ = fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

// handleAcquireLock claims (or, if the caller already holds it, renews)
// reportID/section for the caller — one endpoint for both, since both
// are "I am actively in this section right now" (architecture 9.5).
func (s *Server) handleAcquireLock(w http.ResponseWriter, r *http.Request) {
	st, ok := s.requireStore(w)
	if !ok {
		return
	}
	user, ok := s.authenticatedUser(w, r)
	if !ok {
		return
	}
	reportID := r.PathValue("id")
	section := r.PathValue("section")

	report, ok := s.loadEditableReport(w, r, st, reportID)
	if !ok {
		return
	}
	sch, err := s.loadSchemaFor(report.SchemaName)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !schemaHasSection(sch, section) {
		httpjson.WriteError(w, http.StatusBadRequest, fmt.Sprintf("section %q is not part of schema %q", section, report.SchemaName))
		return
	}

	lock, ok := s.locks.acquire(reportID, section, user.ID, user.Username, user.Role)
	if !ok {
		httpjson.WriteJSON(w, http.StatusConflict, lockConflictResponse{
			Error: fmt.Sprintf("section %q is locked by %s", section, lock.Username),
			Lock:  toLockView(lock),
		})
		return
	}
	s.broadcastLock(reportID, "locked", lock)
	httpjson.WriteJSON(w, http.StatusOK, toLockView(lock))
}

// handleReleaseLock releases reportID/section's lock if the caller is
// the current holder. Idempotent — releasing a lock you don't hold
// (never acquired, already expired, already force-released) is not an
// error, always 204.
func (s *Server) handleReleaseLock(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStore(w); !ok {
		return
	}
	user, ok := s.authenticatedUser(w, r)
	if !ok {
		return
	}
	reportID := r.PathValue("id")
	section := r.PathValue("section")
	if lock, ok := s.locks.release(reportID, section, user.ID); ok {
		s.broadcastLock(reportID, "unlocked", lock)
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleForceReleaseLock is architecture 9.3/9.5's Master-only
// force-release. Idempotent — force-releasing an already-unlocked
// section is not an error, always 204.
func (s *Server) handleForceReleaseLock(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireSuperAdmin(w, r); !ok {
		return
	}
	reportID := r.PathValue("id")
	section := r.PathValue("section")
	if lock, ok := s.locks.forceRelease(reportID, section); ok {
		s.broadcastLock(reportID, "unlocked", lock)
	}
	w.WriteHeader(http.StatusNoContent)
}
