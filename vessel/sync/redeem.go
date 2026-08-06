// SPDX-License-Identifier: AGPL-3.0-only

// Package sync is ovl-vessel's client side of the office sync protocol
// (architecture 11). Built incrementally across Phase 4; this file covers
// only the enrollment handshake (architecture 11.2: exchange a one-time
// code for a long-lived credential) — the ConnectRPC SyncService client
// itself is a later step in the same phase.
package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/captv89/ovl/pkg/backupcrypto"
)

// RedeemResult is what a successful enrollment exchange returns.
// DRPrivateKey/DRPublicKey are this vessel's freshly generated restore-
// bundle keypair (architecture 12.5) — DRPublicKey is also sent to
// office as part of the request (it's already reached office by the
// time this is returned; included here too purely so the caller can
// display/verify it locally without re-deriving it). DRPrivateKey never
// leaves the vessel — the caller must persist it locally (see
// vessel/httpapi/setup.go) — a lost one means DR from that office copy
// is no longer possible until the vessel re-enrolls.
type RedeemResult struct {
	Credential   string
	IssuedAt     time.Time
	VesselName   string
	VesselIMO    string
	DRPrivateKey string
	DRPublicKey  string
}

type redeemRequest struct {
	Code        string `json:"code"`
	DRPublicKey string `json:"drPublicKey"`
}

type redeemResponse struct {
	Credential string `json:"credential"`
	VesselName string `json:"vesselName"`
	VesselIMO  string `json:"vesselIMO"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// ErrRedeemRejected is returned by Redeem when the office reaches it but
// refuses the code (unknown, already used, or its enrollment was
// revoked) — distinct from a transport/network failure, so a caller can
// tell "the office said no" from "could not reach the office."
var ErrRedeemRejected = errors.New("sync: office rejected the enrollment code")

// Redeem exchanges code for a long-lived sync credential by calling
// officeURL's POST /api/enroll (office/httpapi.handleRedeemEnrollment —
// deliberately a plain HTTP endpoint, not a SyncService RPC; see that
// handler's doc comment for why). officeURL is the office's base URL as
// entered in the enrollment wizard step (architecture 9.2, e.g.
// "https://office.example.com"). httpClient may be nil, in which case
// http.DefaultClient is used.
func Redeem(ctx context.Context, httpClient *http.Client, officeURL, code string) (*RedeemResult, error) {
	officeURL = strings.TrimRight(strings.TrimSpace(officeURL), "/")
	if officeURL == "" {
		return nil, errors.New("sync: office URL is required")
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, errors.New("sync: code is required")
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	// Generated fresh on every redemption (including re-enrollment after
	// a revoked/reissued code) — at most one DR keypair active at a time,
	// same "supersedes in place" shape as the sync credential itself.
	drIdentity, err := backupcrypto.GenerateIdentity()
	if err != nil {
		return nil, fmt.Errorf("sync: generate DR keypair: %w", err)
	}

	body, err := json.Marshal(redeemRequest{Code: code, DRPublicKey: drIdentity.PublicKey})
	if err != nil {
		return nil, fmt.Errorf("sync: marshal redeem request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, officeURL+"/api/enroll", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("sync: build redeem request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sync: call %s: %w", officeURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		var errResp errorResponse
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		message := errResp.Error
		if message == "" {
			message = fmt.Sprintf("office returned status %d", resp.StatusCode)
		}
		// 401 specifically means "the office understood the request and
		// said no" (unknown/used/revoked code) — every other non-200
		// status is a transport-ish/office-side failure, not a rejection,
		// so callers can tell "try again" from "get a new code."
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, fmt.Errorf("%w: %s", ErrRedeemRejected, message)
		}
		return nil, fmt.Errorf("sync: %s", message)
	}

	var out redeemResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("sync: decode redeem response: %w", err)
	}
	if out.Credential == "" {
		return nil, errors.New("sync: office returned an empty credential")
	}
	return &RedeemResult{
		Credential:   out.Credential,
		IssuedAt:     time.Now().UTC(),
		VesselName:   out.VesselName,
		VesselIMO:    out.VesselIMO,
		DRPrivateKey: drIdentity.PrivateKey,
		DRPublicKey:  drIdentity.PublicKey,
	}, nil
}
