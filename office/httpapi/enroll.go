// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"errors"
	"net/http"

	"github.com/captv89/ovl/internal/httpjson"
	"github.com/captv89/ovl/office/enrollment"
	"github.com/captv89/ovl/office/synccred"
)

type redeemEnrollmentRequest struct {
	Code string `json:"code"`
	// DRPublicKey is the vessel's freshly generated restore-bundle
	// recipient (architecture 12.5, pkg/backupcrypto) — plain text, not
	// a secret; the matching private key stays vessel-side.
	DRPublicKey string `json:"drPublicKey"`
}

type redeemEnrollmentResponse struct {
	Credential string `json:"credential"`
	VesselName string `json:"vesselName"`
	VesselIMO  string `json:"vesselIMO"`
}

// handleRedeemEnrollment is the vessel-facing half of architecture
// 11.2's sync handshake: exchange a one-time enrollment code (printed on
// the office's enrollment sheet, architecture 12.4) for a long-lived
// sync credential. Deliberately not session/cookie authenticated like
// every other office/httpapi handler — the vessel has no office account,
// only the code — and deliberately a plain endpoint rather than a
// SyncService RPC (Phase 4 decision): this keeps SyncService uniformly
// credential-gated with no auth-exception carve-out, and leaves the
// frozen sync.proto untouched.
//
// The request carries only the code, no vessel id — codes are globally
// unique and self-identifying (same decision), so office/enrollment.Redeem
// scans every currently-issued enrollment to find the match.
func (s *Server) handleRedeemEnrollment(w http.ResponseWriter, r *http.Request) {
	var req redeemEnrollmentRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Code == "" {
		httpjson.WriteError(w, http.StatusBadRequest, "code is required")
		return
	}

	candidates, err := s.st.ListIssuedEnrollments(r.Context())
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	e, err := enrollment.Redeem(candidates, req.Code)
	if err != nil {
		if errors.Is(err, enrollment.ErrCodeNotFound) {
			httpjson.WriteError(w, http.StatusUnauthorized, "code not recognized or already used")
		} else {
			httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	result, err := synccred.Mint(e.VesselID)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	e.DRPublicKey = req.DRPublicKey

	// The vessel has no local profile of its own before enrollment
	// (manual-test feedback, 2026-07-14: an enrolled vessel shouldn't
	// have to re-enter IMO/name it every report) — hand back the office's
	// own vessel profile here so the vessel can store it once and prefill
	// from it forever after.
	v, err := s.st.GetVessel(r.Context(), e.VesselID)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Persist the enrollment state flip (Issued -> Enrolled, code
	// cleared) before the credential: if the credential write then fails,
	// the code has still been consumed rather than silently reusable,
	// matching Redeem's one-time-code semantics — recovery is an Admin
	// re-issuing, the same manual step a lost printed sheet already
	// requires.
	if err := s.st.UpsertEnrollment(r.Context(), e); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.st.UpsertVesselCredential(r.Context(), result.Credential); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	httpjson.WriteJSON(w, http.StatusOK, redeemEnrollmentResponse{
		Credential: result.Token,
		VesselName: v.Name,
		VesselIMO:  v.IMO,
	})
}
