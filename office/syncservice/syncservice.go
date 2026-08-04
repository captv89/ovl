// SPDX-License-Identifier: AGPL-3.0-only

// Package syncservice is ovl-office's ConnectRPC handler for
// architecture 11's SyncService (proto/ovl/sync/v1/sync.proto). Built
// incrementally across Phase 4 (see PROJECT.md's 2026-07-11 decisions
// log for the step sequencing) — SyncStatus, PushOutbox, PullInbox, and
// the attachment chunk-upload pair are implemented; ChatMessage payloads
// within PushOutbox are accepted-and-acked plumbing only (Phase 5).
package syncservice

import (
	"context"
	"time"

	"connectrpc.com/connect"

	syncv1 "github.com/captv89/ovl/pkg/syncproto/gen/ovl/sync/v1"
	"github.com/captv89/ovl/pkg/syncproto/gen/ovl/sync/v1/syncv1connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/captv89/ovl/pkg/attachmentstore"

	"github.com/captv89/ovl/office/store"
)

// Server implements syncv1connect.SyncServiceHandler.
type Server struct {
	syncv1connect.UnimplementedSyncServiceHandler
	st          *store.Store
	attachments *attachmentstore.Store
	stagingDir  string
}

// New builds a Server backed by st, with attachments as the final
// content-addressed store (architecture 15) and stagingDir as where
// in-progress chunk uploads are assembled before promotion — a sibling
// of attachments' own BaseDir, not nested inside it, so incomplete/
// transient staging files never end up inside the directory a real
// backup/restore flow would copy as "the attachment store."
func New(st *store.Store, attachments *attachmentstore.Store, stagingDir string) *Server {
	return &Server{st: st, attachments: attachments, stagingDir: stagingDir}
}

// SyncStatus records the calling vessel's check-in (architecture 11.2
// step 4: "vessel reports its cursors and app version. Office records
// last-seen for the dashboard.") and returns the office's current time.
// Cursor-based staleness reporting is deferred to Step 4 (PullInbox),
// which is what actually populates cursors with meaning — for now this
// only proves the authenticated round trip end to end.
func (s *Server) SyncStatus(ctx context.Context, req *connect.Request[syncv1.SyncStatusRequest]) (*connect.Response[syncv1.SyncStatusResponse], error) {
	vesselID, ok := VesselIDFromContext(ctx)
	if !ok {
		// AuthInterceptor runs on every SyncService RPC (see its own doc
		// comment) — reaching this branch would mean it was bypassed, not
		// that the caller lacks a credential (that case never reaches the
		// handler at all).
		return nil, connect.NewError(connect.CodeInternal, errUnauthenticatedContext)
	}
	now := time.Now().UTC()
	if err := s.st.UpsertVesselSyncStatus(ctx, &store.VesselSyncStatus{
		VesselID:             vesselID,
		AppVersion:           req.Msg.GetAppVersion(),
		LastSeenAt:           now,
		AppliedBundleID:      req.Msg.GetAppliedBundleId(),
		AppliedBundleVersion: req.Msg.GetAppliedBundleVersion(),
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	// Architecture 12.5's DR push path, "confirming it's pushed": the
	// vessel reports which restore commands it applied since its last
	// check-in, piggybacked on the existing per-cycle status call rather
	// than a dedicated RPC — see this field's own proto doc comment.
	if ids := req.Msg.GetAppliedRestoreCommandIds(); len(ids) > 0 {
		if err := s.st.MarkRestoreCommandsApplied(ctx, vesselID, ids, now); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
	}
	// Architecture 9.3/12.4's remote user administration, "confirming
	// it's applied": same ack shape as restore commands above.
	if ids := req.Msg.GetAppliedUserCommandIds(); len(ids) > 0 {
		if err := s.st.MarkUserCommandsApplied(ctx, vesselID, ids, now); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
	}
	// The vessel's full current user roster, reported every check-in —
	// office's only visibility into who exists on a vessel (see
	// VesselUserSummary's own proto doc comment). Replaced wholesale, not
	// merged, so a since-deleted local account doesn't linger in the
	// mirror forever. An empty report is a real, harmless state (an
	// old vessel binary that predates this field) — the mirror just
	// stays empty until a newer binary syncs, not a reason to skip the
	// call.
	pbUsers := req.Msg.GetUsers()
	users := make([]store.VesselUser, len(pbUsers))
	for i, pu := range pbUsers {
		users[i] = store.VesselUser{
			Username: pu.GetUsername(), Role: pu.GetRole(), Active: pu.GetActive(), CanSubmit: pu.GetCanSubmit(),
			UpdatedAt: pu.GetUpdatedAt().AsTime(),
		}
	}
	if err := s.st.ReplaceVesselUsers(ctx, vesselID, users, now); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&syncv1.SyncStatusResponse{ServerTime: timestamppb.New(now)}), nil
}
