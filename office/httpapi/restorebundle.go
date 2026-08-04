// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/captv89/ovl/internal/httpjson"
	"github.com/captv89/ovl/office/restorebundle"
	"github.com/captv89/ovl/office/store"
	"github.com/captv89/ovl/office/vessels"
	"github.com/captv89/ovl/pkg/backupcrypto"
)

// requireVesselWithDRKey loads vesselID and its enrollment, failing with
// the same 404/409 either handleGenerateRestoreBundle or
// handlePushRestoreBundle would want: a restore bundle (downloaded or
// pushed) can only ever be encrypted against a real DR public key, and
// there is nothing to encrypt against until the vessel has
// (re-)redeemed its enrollment at least once since the DR keypair
// exchange landed (pkg/backupcrypto).
func (s *Server) requireVesselWithDRKey(w http.ResponseWriter, r *http.Request, vesselID string) (*vessels.Vessel, string, bool) {
	v, err := s.st.GetVessel(r.Context(), vesselID)
	if err != nil {
		httpjson.WriteError(w, http.StatusNotFound, "vessel not found")
		return nil, "", false
	}
	e, err := s.st.GetEnrollment(r.Context(), vesselID)
	if err != nil || e.DRPublicKey == "" {
		httpjson.WriteError(w, http.StatusConflict, "this vessel has no restore-bundle key on file yet — it must (re-)redeem its enrollment code before a restore bundle can be generated for it")
		return nil, "", false
	}
	return v, e.DRPublicKey, true
}

// handleGenerateRestoreBundle serves design handoff B2's DR tab: an
// Admin downloads an encrypted, ready-to-import restore bundle for one
// vessel (architecture 12.5's "all reports, config, chat and
// attachments" — attachments still deliberately excluded, see
// office/restorebundle.BuildBundle's own doc comment for why).
// Bundle content is built by office/restorebundle.BuildBundle, shared
// with office/syncservice's FetchRestoreBundle RPC (the sync-delivered
// push path, architecture 11.2's PullInbox restore_commands). This is
// the manual "download the file yourself" path, kept alongside
// handlePushRestoreBundle for a Master who'd rather import via the
// vessel's own local admin endpoint than wait for the next sync cycle.
func (s *Server) handleGenerateRestoreBundle(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	v, drPublicKey, ok := s.requireVesselWithDRKey(w, r, r.PathValue("id"))
	if !ok {
		return
	}

	bundle, err := restorebundle.BuildBundle(r.Context(), s.st, v.ID, v.Name, v.IMO)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	plaintext, err := json.Marshal(bundle)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	ciphertext, err := backupcrypto.Encrypt(plaintext, drPublicKey)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-restore-bundle.age"`, v.IMO))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(ciphertext) // #nosec G705 -- encrypted bytes served as application/octet-stream + Content-Disposition: attachment, never inline-rendered by a browser
}

// defaultPushRestoreBundleReason is used when an Admin pushes without
// typing one — B2's DR tab makes the reason field optional, since the
// scenario is usually self-evident (a vessel that just had its
// enrollment revoked and re-issued because it lost its own data).
const defaultPushRestoreBundleReason = "Restore bundle pushed from office"

type pushRestoreBundleRequest struct {
	Reason string `json:"reason"`
}

// handlePushRestoreBundle is design handoff B2's DR tab "push to vessel"
// action (architecture 12.5/11.2's DR push path): queues a
// RestoreCommand (office/store.QueueRestoreCommand) rather than
// generating and returning bundle bytes directly — the vessel picks it
// up on its own next sync cycle (PullInbox's restore_commands stream),
// fetches the actual bundle itself (FetchRestoreBundle RPC, built fresh
// at that point so it reflects "as of last sync," not "as of when this
// endpoint was called"), and reports back via SyncStatus once applied.
// This endpoint's own response is just confirmation the push was queued,
// not that the vessel has it yet — B2's DR tab polls the vessel detail
// fetch's RestoreCommands for the fetched/applied timestamps that answer
// "has it landed."
func (s *Server) handlePushRestoreBundle(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	vesselID := r.PathValue("id")
	if _, _, ok := s.requireVesselWithDRKey(w, r, vesselID); !ok {
		return
	}

	var req pushRestoreBundleRequest
	if r.ContentLength != 0 {
		if err := httpjson.DecodeJSON(r, &req); err != nil {
			httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = defaultPushRestoreBundleReason
	}

	id, err := uuid.NewV7()
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	cmd := &store.RestoreCommand{ID: id.String(), VesselID: vesselID, Reason: reason, IssuedBy: user.Username, IssuedAt: time.Now().UTC()}
	if err := s.st.QueueRestoreCommand(r.Context(), cmd); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, toRestoreCommandView(*cmd))
}
