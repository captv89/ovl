// SPDX-License-Identifier: AGPL-3.0-only

package syncservice

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"connectrpc.com/connect"

	syncv1 "github.com/captv89/ovl/pkg/syncproto/gen/ovl/sync/v1"

	"github.com/captv89/ovl/office/restorebundle"
	"github.com/captv89/ovl/office/store"
	"github.com/captv89/ovl/pkg/backupcrypto"
)

// FetchRestoreBundle is architecture 12.5's DR push path, second half
// (see this RPC's own proto doc comment): once the calling vessel has
// seen a RestoreCommand notification via PullInbox, it calls this,
// authenticated with its own sync credential (same AuthInterceptor as
// every other SyncService RPC — no auth carve-out needed, unlike
// enrollment redemption), to fetch the actual encrypted bundle. Builds
// and encrypts fresh on every call rather than caching the bytes
// QueueRestoreCommand-time — content may have changed between when the
// command was queued and when the vessel actually calls in, and a
// restore bundle is meant to reflect "as of last sync," not "as of
// whenever an Admin clicked push."
func (s *Server) FetchRestoreBundle(ctx context.Context, req *connect.Request[syncv1.FetchRestoreBundleRequest]) (*connect.Response[syncv1.FetchRestoreBundleResponse], error) {
	vesselID, ok := VesselIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, errUnauthenticatedContext)
	}
	commandID := req.Msg.GetCommandId()

	cmd, err := s.st.GetRestoreCommand(ctx, commandID, vesselID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("restore command not found for this vessel"))
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	vessel, err := s.st.GetVessel(ctx, vesselID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	e, err := s.st.GetEnrollment(ctx, vesselID)
	if err != nil || e.DRPublicKey == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("this vessel has no restore-bundle key on file — it must (re-)redeem its enrollment code first"))
	}

	bundle, err := restorebundle.BuildBundle(ctx, s.st, vessel.ID, vessel.Name, vessel.IMO)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	plaintext, err := json.Marshal(bundle)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	ciphertext, err := backupcrypto.Encrypt(plaintext, e.DRPublicKey)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	if err := s.st.MarkRestoreCommandFetched(ctx, cmd.ID, time.Now().UTC()); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&syncv1.FetchRestoreBundleResponse{Ciphertext: ciphertext}), nil
}
