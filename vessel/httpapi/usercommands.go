// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/captv89/ovl/pkg/syncproto"
	syncv1 "github.com/captv89/ovl/pkg/syncproto/gen/ovl/sync/v1"

	"github.com/captv89/ovl/vessel/auth"
	"github.com/captv89/ovl/vessel/store"
)

// applyUserCommand is architecture 9.3/12.4's remote vessel-user-
// administration path, apply half: dispatches one office-queued
// UserCommand (pulled via PullInbox) to the matching local action. Every
// path here reuses exactly the same core logic and guardrails as this
// package's own local Master-facing handlers (createLocalUser,
// auth.User's ResetPassword/SetRole/SetActive/SetCanSubmit) — a remote
// command is never trusted to do anything a Master couldn't already do
// sitting at the vessel console, and role=master is refused for both
// CREATE and SET_ROLE regardless of what office sent (defense in depth:
// office already refuses these at queue time too, see
// office/httpapi/vesselusers.go, but the vessel is the final authority
// over its own accounts and re-checks independently).
func applyUserCommand(ctx context.Context, st *store.Store, cmd *syncv1.UserCommand) error {
	action, err := syncproto.UserCommandActionToString(cmd.GetAction())
	if err != nil {
		return err
	}
	switch action {
	case syncproto.UserCommandActionCreate:
		_, err := createLocalUser(ctx, st, cmd.GetUsername(), auth.Role(cmd.GetRole()), cmd.GetTemporaryPassword())
		return err
	case syncproto.UserCommandActionResetPassword:
		return applyResetPasswordCommand(ctx, st, cmd)
	case syncproto.UserCommandActionSetRole:
		return applySetRoleCommand(ctx, st, cmd)
	case syncproto.UserCommandActionSetActive:
		return applySetActiveCommand(ctx, st, cmd)
	case syncproto.UserCommandActionSetCanSubmit:
		return applySetCanSubmitCommand(ctx, st, cmd)
	default:
		return fmt.Errorf("unknown user command action %q", action)
	}
}

func applyResetPasswordCommand(ctx context.Context, st *store.Store, cmd *syncv1.UserCommand) error {
	u, err := st.GetUserByUsername(ctx, cmd.GetUsername())
	if err != nil {
		return fmt.Errorf("reset password for %s: %w", cmd.GetUsername(), err)
	}
	if err := u.ResetPassword(cmd.GetTemporaryPassword()); err != nil {
		return err
	}
	return st.UpdateUser(ctx, u)
}

// applySetRoleCommand refuses role=master via auth.User.SetRole's own
// guard (see that method's doc comment) — this vessel-side check is the
// authoritative one; office's own refusal at queue time is
// defense-in-depth, not the thing actually relied on.
func applySetRoleCommand(ctx context.Context, st *store.Store, cmd *syncv1.UserCommand) error {
	u, err := st.GetUserByUsername(ctx, cmd.GetUsername())
	if err != nil {
		return fmt.Errorf("set role for %s: %w", cmd.GetUsername(), err)
	}
	if err := u.SetRole(auth.Role(cmd.GetRole())); err != nil {
		return err
	}
	return st.UpdateUser(ctx, u)
}

// applySetActiveCommand refuses deactivating the Master, mirroring
// vessel/httpapi's own local handleUpdateUser guard exactly (see that
// handler's doc comment) — reactivating the Master is a no-op state
// change, not meaningfully guarded, since the Master can never actually
// end up deactivated in the first place.
func applySetActiveCommand(ctx context.Context, st *store.Store, cmd *syncv1.UserCommand) error {
	u, err := st.GetUserByUsername(ctx, cmd.GetUsername())
	if err != nil {
		return fmt.Errorf("set active for %s: %w", cmd.GetUsername(), err)
	}
	if !cmd.GetActive() && u.IsSuperAdmin() {
		return errors.New("the Master account cannot be deactivated")
	}
	u.SetActive(cmd.GetActive())
	return st.UpdateUser(ctx, u)
}

func applySetCanSubmitCommand(ctx context.Context, st *store.Store, cmd *syncv1.UserCommand) error {
	u, err := st.GetUserByUsername(ctx, cmd.GetUsername())
	if err != nil {
		return fmt.Errorf("set canSubmit for %s: %w", cmd.GetUsername(), err)
	}
	u.SetCanSubmit(cmd.GetCanSubmit())
	return st.UpdateUser(ctx, u)
}

// buildVesselUserSummaries is what this vessel reports as its current
// roster on every SyncStatus call (architecture 9.3/12.4 — office has no
// other way to know who exists on a vessel). No password data.
func buildVesselUserSummaries(ctx context.Context, st *store.Store) ([]*syncv1.VesselUserSummary, error) {
	users, err := st.ListUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list users for roster report: %w", err)
	}
	out := make([]*syncv1.VesselUserSummary, len(users))
	for i, u := range users {
		out[i] = &syncv1.VesselUserSummary{
			Username: u.Username, Role: string(u.Role), Active: u.Active, CanSubmit: u.CanSubmit,
			UpdatedAt: timestamppb.New(u.UpdatedAt),
		}
	}
	return out, nil
}
