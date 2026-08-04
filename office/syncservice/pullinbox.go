// SPDX-License-Identifier: AGPL-3.0-only

package syncservice

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	syncv1 "github.com/captv89/ovl/pkg/syncproto/gen/ovl/sync/v1"

	"github.com/captv89/ovl/office/configbundle"
	"github.com/captv89/ovl/pkg/syncproto"
)

// PullInbox delivers config bundles and schema versions newer than the
// calling vessel's cursors (architecture 11.2 step 3). Two different
// pull models, per the Phase 4 decision (PROJECT.md 2026-07-11's
// decisions log, confirmed with the user):
//
//   - schema_versions is a global stream: every published version with
//     cursor > the vessel's schema_version_cursor, in cursor order. Not
//     scoped per vessel/bundle — a vessel may still need an older
//     version for a report already in progress under an earlier bundle.
//   - config_bundles is a single current-pointer comparison, matching
//     architecture 6.5's literal "the vessel pulls the newest assigned
//     bundle": resolve the vessel's currently-assigned bundle
//     (office/configbundle.Resolve, vessel-scope wins over group-scope)
//     and include it only if its cursor is newer than what the vessel
//     already has.
//
// Chat messages, invalidation notices, remarks, and restore commands are
// a third model, vessel-scoped like config_bundles but with no "current
// pointer" — every row with cursor > the vessel's own cursor (Slices
// S3/S4/S5; restore commands landed 2026-07-20, architecture 12.5's DR
// push path, office/store.QueueRestoreCommand via B2's DR tab).
func (s *Server) PullInbox(ctx context.Context, req *connect.Request[syncv1.PullInboxRequest]) (*connect.Response[syncv1.PullInboxResponse], error) {
	vesselID, ok := VesselIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, errUnauthenticatedContext)
	}
	cursors := req.Msg.GetCursors()

	pbSchemaVersions, nextSchemaCursor, err := s.pullSchemaVersions(ctx, cursors.GetSchemaVersionCursor())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	pbConfigBundles, nextBundleCursor, err := s.pullAssignedBundle(ctx, vesselID, cursors.GetConfigBundleCursor())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	pbChatMessages, nextChatCursor, err := s.pullChatMessages(ctx, vesselID, cursors.GetChatCursor())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	pbInvalidationNotices, nextInvalidationCursor, err := s.pullInvalidationNotices(ctx, vesselID, cursors.GetInvalidationNoticeCursor())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	pbRemarks, nextRemarkCursor, err := s.pullRemarks(ctx, vesselID, cursors.GetRemarkCursor())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	pbRestoreCommands, nextRestoreCommandCursor, err := s.pullRestoreCommands(ctx, vesselID, cursors.GetRestoreCommandCursor())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	pbUserCommands, nextUserCommandCursor, err := s.pullUserCommands(ctx, vesselID, cursors.GetUserCommandCursor())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&syncv1.PullInboxResponse{
		ConfigBundles:       pbConfigBundles,
		SchemaVersions:      pbSchemaVersions,
		ChatMessages:        pbChatMessages,
		InvalidationNotices: pbInvalidationNotices,
		Remarks:             pbRemarks,
		RestoreCommands:     pbRestoreCommands,
		UserCommands:        pbUserCommands,
		NextCursors: &syncv1.SyncCursors{
			ConfigBundleCursor:       nextBundleCursor,
			SchemaVersionCursor:      nextSchemaCursor,
			RemarkCursor:             nextRemarkCursor,
			ChatCursor:               nextChatCursor,
			InvalidationNoticeCursor: nextInvalidationCursor,
			RestoreCommandCursor:     nextRestoreCommandCursor,
			UserCommandCursor:        nextUserCommandCursor,
		},
	}), nil
}

// pullUserCommands returns vesselID's user-management commands with a
// cursor greater than sinceCursor — a vessel-scoped stream (architecture
// 9.3/12.4's remote user administration), mirroring pullRestoreCommands'
// shape. Unlike restore commands, the full command (including any
// plaintext temporary password) travels directly in this response —
// there is no separate fetch step, since a user command is tiny compared
// to a restore bundle and needs no out-of-band transport.
func (s *Server) pullUserCommands(ctx context.Context, vesselID string, sinceCursor int64) ([]*syncv1.UserCommand, int64, error) {
	items, err := s.st.ListUserCommandsSince(ctx, vesselID, sinceCursor)
	if err != nil {
		return nil, sinceCursor, fmt.Errorf("list user commands: %w", err)
	}
	out := make([]*syncv1.UserCommand, 0, len(items))
	nextCursor := sinceCursor
	for _, item := range items {
		action, err := syncproto.UserCommandActionFromString(item.Command.Action)
		if err != nil {
			return nil, sinceCursor, err
		}
		out = append(out, &syncv1.UserCommand{
			CommandId:         item.Command.ID,
			Action:            action,
			Username:          item.Command.Username,
			Role:              item.Command.Role,
			TemporaryPassword: item.Command.TemporaryPassword,
			CanSubmit:         item.Command.CanSubmit,
			Active:            item.Command.Active,
			IssuedBy:          item.Command.IssuedBy,
			IssuedAt:          timestamppb.New(item.Command.IssuedAt),
		})
		if item.Cursor > nextCursor {
			nextCursor = item.Cursor
		}
		if err := s.st.MarkUserCommandFetched(ctx, item.Command.ID, time.Now().UTC()); err != nil {
			return nil, sinceCursor, fmt.Errorf("mark user command %s fetched: %w", item.Command.ID, err)
		}
	}
	return out, nextCursor, nil
}

// pullRestoreCommands returns vesselID's restore commands with a cursor
// greater than sinceCursor — a vessel-scoped stream (architecture 12.5's
// DR push path), mirroring pullRemarks' shape. The bundle itself is
// fetched separately via FetchRestoreBundle (syncservice.go) once the
// vessel sees the command_id here; this only delivers the notification
// (proto's RestoreCommand doc comment).
func (s *Server) pullRestoreCommands(ctx context.Context, vesselID string, sinceCursor int64) ([]*syncv1.RestoreCommand, int64, error) {
	items, err := s.st.ListRestoreCommandsSince(ctx, vesselID, sinceCursor)
	if err != nil {
		return nil, sinceCursor, fmt.Errorf("list restore commands: %w", err)
	}
	out := make([]*syncv1.RestoreCommand, 0, len(items))
	nextCursor := sinceCursor
	for _, item := range items {
		out = append(out, &syncv1.RestoreCommand{
			CommandId: item.Command.ID,
			Reason:    item.Command.Reason,
			IssuedAt:  timestamppb.New(item.Command.IssuedAt),
		})
		if item.Cursor > nextCursor {
			nextCursor = item.Cursor
		}
	}
	return out, nextCursor, nil
}

// pullRemarks returns vesselID's remarks with a cursor greater than
// sinceCursor — a vessel-scoped stream (Slice S5), mirroring
// pullChatMessages/pullInvalidationNotices' shape.
func (s *Server) pullRemarks(ctx context.Context, vesselID string, sinceCursor int64) ([]*syncv1.Remark, int64, error) {
	items, err := s.st.ListRemarksSince(ctx, vesselID, sinceCursor)
	if err != nil {
		return nil, sinceCursor, fmt.Errorf("list remarks: %w", err)
	}
	out := make([]*syncv1.Remark, 0, len(items))
	nextCursor := sinceCursor
	for _, item := range items {
		out = append(out, syncproto.RemarkFromDomain(item.Remark))
		if item.Cursor > nextCursor {
			nextCursor = item.Cursor
		}
	}
	return out, nextCursor, nil
}

// pullInvalidationNotices returns vesselID's invalidation notices with a
// cursor greater than sinceCursor — a vessel-scoped stream (Slice S4),
// mirroring pullChatMessages' shape.
func (s *Server) pullInvalidationNotices(ctx context.Context, vesselID string, sinceCursor int64) ([]*syncv1.InvalidationNotice, int64, error) {
	items, err := s.st.ListInvalidationNoticesSince(ctx, vesselID, sinceCursor)
	if err != nil {
		return nil, sinceCursor, fmt.Errorf("list invalidation notices: %w", err)
	}
	out := make([]*syncv1.InvalidationNotice, 0, len(items))
	nextCursor := sinceCursor
	for _, item := range items {
		out = append(out, syncproto.InvalidationNoticeFromDomain(item.Notice))
		if item.Cursor > nextCursor {
			nextCursor = item.Cursor
		}
	}
	return out, nextCursor, nil
}

// pullChatMessages returns vesselID's office-authored chat messages
// (direction=office) with a cursor greater than sinceCursor — a
// vessel-scoped stream, mirroring pullAssignedBundle's shape rather than
// pullSchemaVersions' global one, since chat is inherently per-vessel.
func (s *Server) pullChatMessages(ctx context.Context, vesselID string, sinceCursor int64) ([]*syncv1.ChatMessage, int64, error) {
	items, err := s.st.ListChatMessagesSince(ctx, vesselID, sinceCursor)
	if err != nil {
		return nil, sinceCursor, fmt.Errorf("list chat messages: %w", err)
	}
	out := make([]*syncv1.ChatMessage, 0, len(items))
	nextCursor := sinceCursor
	for _, item := range items {
		out = append(out, syncproto.ChatMessageFromDomain(item.Message))
		if item.Cursor > nextCursor {
			nextCursor = item.Cursor
		}
	}
	return out, nextCursor, nil
}

func (s *Server) pullSchemaVersions(ctx context.Context, sinceCursor int64) ([]*syncv1.SchemaVersion, int64, error) {
	items, err := s.st.ListSchemaVersionsSince(ctx, sinceCursor)
	if err != nil {
		return nil, sinceCursor, fmt.Errorf("list schema versions: %w", err)
	}
	out := make([]*syncv1.SchemaVersion, 0, len(items))
	nextCursor := sinceCursor
	for _, item := range items {
		kind, err := syncproto.SchemaKindFromName(item.Version.SchemaName)
		if err != nil {
			return nil, sinceCursor, err
		}
		out = append(out, &syncv1.SchemaVersion{
			SchemaKind:  kind,
			Version:     item.Version.Version,
			SchemaJson:  item.Version.Content,
			PublishedAt: timestamppb.New(item.Version.PublishedAt),
		})
		if item.Cursor > nextCursor {
			nextCursor = item.Cursor
		}
	}
	return out, nextCursor, nil
}

// pullAssignedBundle resolves vesselID's currently-assigned bundle and
// includes it (as the sole element of a slice of at most one) only if
// it's newer than sinceCursor. content_json is the tagged, versioned
// pkg/configwire.Bundle — the office resolves the bundle's fleet/group/
// vessel scopes down to this vessel's own effective config
// (ConfigBundle.ResolveFor) before marshalling, so the vessel applies a
// flat document with no scope logic aboard (codebase audit 2026-07-22 §1,
// resolve-office-side design confirmed 2026-07-23).
func (s *Server) pullAssignedBundle(ctx context.Context, vesselID string, sinceCursor int64) ([]*syncv1.ConfigBundle, int64, error) {
	vessel, err := s.st.GetVessel(ctx, vesselID)
	if err != nil {
		return nil, sinceCursor, fmt.Errorf("load vessel %s: %w", vesselID, err)
	}
	assignments, err := s.st.ListBundleAssignments(ctx)
	if err != nil {
		return nil, sinceCursor, fmt.Errorf("list bundle assignments: %w", err)
	}
	match := configbundle.Resolve(assignments, vesselID, vessel.Groups)
	if match == nil {
		return nil, sinceCursor, nil
	}

	bundleCursor, err := s.st.GetConfigBundleCursor(ctx, match.BundleID)
	if err != nil {
		return nil, sinceCursor, fmt.Errorf("get cursor for bundle %s: %w", match.BundleID, err)
	}
	if bundleCursor <= sinceCursor {
		return nil, sinceCursor, nil
	}

	bundle, err := s.st.GetConfigBundle(ctx, match.BundleID)
	if err != nil {
		return nil, sinceCursor, fmt.Errorf("get bundle %s: %w", match.BundleID, err)
	}
	contentJSON, err := json.Marshal(bundle.ResolveFor(vesselID, vessel.Groups, bundleCursor))
	if err != nil {
		return nil, sinceCursor, fmt.Errorf("marshal bundle %s: %w", bundle.ID, err)
	}
	pb := &syncv1.ConfigBundle{
		BundleId:    bundle.ID,
		VersionNo:   bundleCursor,
		ContentJson: contentJSON,
		PublishedAt: timestamppb.New(bundle.PublishedAt),
	}
	return []*syncv1.ConfigBundle{pb}, bundleCursor, nil
}
