// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/captv89/ovl/internal/httpjson"
	"github.com/captv89/ovl/pkg/attachmentstore"
	"github.com/captv89/ovl/pkg/backupcrypto"
	"github.com/captv89/ovl/pkg/restorebundle"
	"github.com/captv89/ovl/vessel/store"
)

// backupsDir is where nightly/on-demand snapshots live — a fixed
// subdirectory of the vessel's own data directory, not a separately
// configured "second path" (design handoff A10 mentions a configurable
// target, but there is no settings screen or config-bundle mechanism yet
// to hold that setting; documented simplification, see PROJECT.md).
// Each snapshot gets its own timestamped folder containing ovl.db and,
// if any exist, a copy of the attachment store.
func backupsDir(dataDir string) string {
	return filepath.Join(dataDir, "backups")
}

// attachmentsDir is where this vessel's content-addressed attachment
// store lives (architecture 15): Bunker/EDN report attachments captured
// locally, read from here by Phase 4's chunk-upload sync path to push
// to the office (see vessel/sync's attachment phase). Local capture
// itself (the upload UI, client-side image downscaling) is not built —
// see PROJECT.md. Backup checks for and copies this directory
// regardless, so DR needs no revisiting once capture lands.
func attachmentsDir(dataDir string) string {
	return filepath.Join(dataDir, "attachments")
}

type backupInfo struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
}

const backupIDLayout = "20060102T150405Z"

// requireSuperAdmin is requireStore's counterpart for the local-DR admin
// endpoints (architecture 9.6: "guarded by Master login"): Master is the
// vessel's only super-admin (architecture 9.3), and backup/restore is
// exactly the kind of vessel-wide, hard-to-reverse action that
// permission is reserved for.
func (s *Server) requireSuperAdmin(w http.ResponseWriter, r *http.Request) (*store.Store, bool) {
	st, ok := s.requireStore(w)
	if !ok {
		return nil, false
	}
	user, ok := s.authenticatedUser(w, r)
	if !ok {
		return nil, false
	}
	if !user.IsSuperAdmin() {
		httpjson.WriteError(w, http.StatusForbidden, "only the Master account can manage backups")
		return nil, false
	}
	return st, true
}

// handleListBackups lists existing snapshots, newest first (design
// handoff A10's Backup section: "restore from snapshot").
func (s *Server) handleListBackups(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireSuperAdmin(w, r); !ok {
		return
	}
	entries, err := os.ReadDir(backupsDir(s.config().DataDir))
	if err != nil && !os.IsNotExist(err) {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	backups := make([]backupInfo, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		at, err := time.Parse(backupIDLayout, e.Name())
		if err != nil {
			continue // not one of ours; ignore rather than fail the whole list
		}
		backups = append(backups, backupInfo{ID: e.Name(), CreatedAt: at.UTC()})
	}
	sort.Slice(backups, func(i, j int) bool { return backups[i].CreatedAt.After(backups[j].CreatedAt) })
	httpjson.WriteJSON(w, http.StatusOK, backups)
}

// RunNightlySnapshot performs one snapshot if the vessel has completed
// first-run setup (architecture 9.6's nightly job; vessel/main.go's
// scheduler calls this on a timer) — a no-op before that, since there is
// no store yet to snapshot. Returns the error rather than writing an
// HTTP response (there is no request here); the caller logs and
// continues rather than treating a single failed nightly snapshot as
// fatal to the running server.
func (s *Server) RunNightlySnapshot(ctx context.Context) (backupInfo, error) {
	st := s.storeOrNil()
	if st == nil {
		return backupInfo{}, nil
	}
	return s.snapshotNow(ctx, st)
}

// handleSnapshotNow performs an immediate snapshot (design handoff A10:
// "Snapshot now") — the same routine the nightly scheduler (see
// vessel/main.go) calls on its own timer.
func (s *Server) handleSnapshotNow(w http.ResponseWriter, r *http.Request) {
	st, ok := s.requireSuperAdmin(w, r)
	if !ok {
		return
	}
	info, err := s.snapshotNow(r.Context(), st)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, info)
}

// snapshotNow is the shared snapshot routine: a fresh timestamped folder
// under backupsDir with a VACUUM INTO'd database copy and (if any
// attachments exist) a copy of the attachment store.
func (s *Server) snapshotNow(ctx context.Context, st *store.Store) (backupInfo, error) {
	dataDir := s.config().DataDir
	now := time.Now().UTC()
	id := now.Format(backupIDLayout)
	dir := filepath.Join(backupsDir(dataDir), id)

	if err := st.SnapshotTo(ctx, store.DBPath(dir)); err != nil {
		return backupInfo{}, fmt.Errorf("snapshot database: %w", err)
	}
	attachments := &attachmentstore.Store{BaseDir: attachmentsDir(dataDir)}
	if err := attachments.CopyAllTo(filepath.Join(dir, "attachments")); err != nil {
		return backupInfo{}, fmt.Errorf("snapshot attachments: %w", err)
	}
	return backupInfo{ID: id, CreatedAt: now}, nil
}

type restoreRequest struct {
	// Confirm must be true — the server-side half of design handoff A10's
	// "restore from snapshot (guarded, two-step)"; the frontend's own
	// confirmation dialog is the other half.
	Confirm bool `json:"confirm"`
}

// handleRestoreBackup replaces the live database with a chosen snapshot.
// Closes the current Store, swaps the files, and reopens — all under the
// same write lock setConfigured uses for the equivalent first-run-wizard
// swap, so no request can observe a half-restored store (requireStore's
// read lock simply blocks until this completes).
func (s *Server) handleRestoreBackup(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireSuperAdmin(w, r); !ok {
		return
	}
	var req restoreRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !req.Confirm {
		httpjson.WriteError(w, http.StatusBadRequest, "confirm must be true")
		return
	}
	id := r.PathValue("id")
	// id becomes a filesystem path component below — never trust it as
	// client-supplied without validating it's actually one of the
	// timestamp-formatted IDs this app itself generates (snapshotNow),
	// or a "../"-laden id could escape backupsDir (CWE-22). Reusing
	// backupIDLayout, the exact format handleListBackups already parses
	// entries with, keeps this in lockstep with how IDs are minted.
	if _, err := time.Parse(backupIDLayout, id); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid backup id")
		return
	}
	dataDir := s.config().DataDir
	snapshotPath := store.DBPath(filepath.Join(backupsDir(dataDir), id))
	if _, err := os.Stat(snapshotPath); os.IsNotExist(err) { // #nosec G703 -- id was validated against backupIDLayout above; gosec's taint tracker doesn't see time.Parse as a sanitizer
		httpjson.WriteError(w, http.StatusNotFound, "backup not found")
		return
	}

	if err := s.restoreFromSnapshot(dataDir, snapshotPath); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string]bool{"restored": true})
}

// restoreFromSnapshot holds the write lock for the entire close-replace-
// reopen sequence: SQLite must not have dataDir's database file open
// while it's replaced, and no request may observe the store mid-swap.
func (s *Server) restoreFromSnapshot(dataDir, snapshotPath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.st != nil {
		if err := s.st.Close(); err != nil {
			return fmt.Errorf("close store before restore: %w", err)
		}
		s.st = nil
	}
	if err := store.RestoreDatabase(dataDir, snapshotPath); err != nil {
		return err
	}
	newSt, err := store.Open(dataDir)
	if err != nil {
		return fmt.Errorf("reopen store after restore: %w", err)
	}
	s.st = newSt
	return nil
}

// restoreBundleImportResult summarizes what an import actually applied
// (design handoff A10's own DR section, level 2) — the operator's
// confirmation that the office-generated bundle actually landed, not
// just a bare 200.
type restoreBundleImportResult struct {
	Reports             int  `json:"reports"`
	Versions            int  `json:"versions"`
	Events              int  `json:"events"`
	ChatMessages        int  `json:"chatMessages"`
	ConfigBundleApplied bool `json:"configBundleApplied"`
}

// errNoDRIdentity distinguishes "this vessel has no restore keypair on
// file yet" from a generic decrypt/apply failure — both
// handleImportRestoreBundle (409) and the auto-fetch-on-sync path
// (vessel/httpapi/sync.go's pullInboxBatch, which just leaves the
// command pending for a later cycle rather than surfacing an HTTP error)
// need to tell them apart.
var errNoDRIdentity = errors.New("this vessel has no restore keypair on file — it must complete enrollment (or re-enrollment) before a restore bundle can be imported")

// decryptRestoreBundle decrypts ciphertext with this vessel's own DR
// private key (pkg/backupcrypto, generated at enrollment redemption —
// see vessel/sync.Redeem) and unmarshals it into a Bundle. Shared by
// handleImportRestoreBundle (Master pastes/uploads a browser-downloaded
// file) and pullInboxBatch's auto-fetch path (architecture 11.2's
// PullInbox restore_commands, fetched over FetchRestoreBundle) — same
// bytes, same key, two different ways of arriving at this vessel.
func decryptRestoreBundle(ctx context.Context, st *store.Store, ciphertext []byte) (*restorebundle.Bundle, error) {
	identity, err := st.GetDRIdentity(ctx)
	if err != nil {
		return nil, errNoDRIdentity
	}
	plaintext, err := backupcrypto.Decrypt(ciphertext, identity.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("could not decrypt restore bundle — it may not have been encrypted against this vessel's current key: %w", err)
	}
	var bundle restorebundle.Bundle
	if err := json.Unmarshal(plaintext, &bundle); err != nil {
		return nil, fmt.Errorf("restore bundle content is not valid: %w", err)
	}
	return &bundle, nil
}

// applyRestoreBundle is architecture 12.5's level-2 DR apply step,
// shared by handleImportRestoreBundle and pullInboxBatch's auto-fetch
// path (see decryptRestoreBundle's own doc comment on why both exist).
// This is meant for rebuilding a vessel that has lost its local data,
// not a repeatable merge operation against a healthy one: SaveReport's
// own upsert-by-(report_id, version_no) makes re-applying the same
// bundle to an empty vessel and re-running it later both safe (submitted
// reports are immutable, so the same bundle always contains the same
// values), but AppendEvent has no dedup key, so running this twice
// against a vessel that already has some of these events would
// duplicate its audit trail — a real, undocumented-until-now boundary,
// not silently glossed over, and not a concern for the actual DR
// scenario (an empty vessel has nothing to duplicate against). Also
// applies the bundle's embedded config bundle snapshot, if any
// (architecture 12.5's "all reports, config, chat" — the same
// insert-if-absent path an ordinary PullInbox pull would have used, see
// vessel/store.ApplyConfigBundle's own doc comment).
func applyRestoreBundle(ctx context.Context, st *store.Store, bundle *restorebundle.Bundle) (restoreBundleImportResult, error) {
	var result restoreBundleImportResult
	for _, br := range bundle.Reports {
		result.Reports++
		for _, version := range br.Versions {
			if err := st.SaveReport(ctx, version); err != nil {
				return result, fmt.Errorf("save report %s v%d: %w", version.ReportID, version.VersionNo, err)
			}
			result.Versions++
		}
		for _, event := range br.Events {
			if _, err := st.AppendEvent(ctx, event); err != nil {
				return result, fmt.Errorf("append event for report %s: %w", br.ReportID, err)
			}
			result.Events++
		}
		for _, msg := range br.Chat {
			if err := st.InsertChatMessage(ctx, msg); err != nil {
				return result, fmt.Errorf("insert chat message for report %s: %w", br.ReportID, err)
			}
			result.ChatMessages++
		}
	}
	if bundle.ConfigBundle != nil {
		if err := st.ApplyConfigBundle(ctx, store.PulledConfigBundle{
			BundleID: bundle.ConfigBundle.BundleID, VersionNo: bundle.ConfigBundle.VersionNo,
			Content: bundle.ConfigBundle.ContentJSON, PublishedAt: bundle.ConfigBundle.PublishedAt,
		}); err != nil {
			return result, fmt.Errorf("apply config bundle %s: %w", bundle.ConfigBundle.BundleID, err)
		}
		result.ConfigBundleApplied = true
	}
	return result, nil
}

// handleImportRestoreBundle is design handoff A10's Master-facing manual
// import: paste/upload an office-downloaded restore bundle file. The
// Confirm gate this shares with handleRestoreBackup exists because this
// can overwrite locally-drafted, not-yet-submitted work at the same
// (report_id, version_no) with the bundle's version.
func (s *Server) handleImportRestoreBundle(w http.ResponseWriter, r *http.Request) {
	st, ok := s.requireSuperAdmin(w, r)
	if !ok {
		return
	}
	if r.URL.Query().Get("confirm") != "true" {
		httpjson.WriteError(w, http.StatusBadRequest, "confirm=true is required")
		return
	}
	ciphertext, err := io.ReadAll(r.Body)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "could not read request body: "+err.Error())
		return
	}

	bundle, err := decryptRestoreBundle(r.Context(), st, ciphertext)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errNoDRIdentity) {
			status = http.StatusConflict
		}
		httpjson.WriteError(w, status, err.Error())
		return
	}
	result, err := applyRestoreBundle(r.Context(), st, bundle)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, result)
}
