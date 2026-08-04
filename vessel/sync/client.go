// SPDX-License-Identifier: AGPL-3.0-only

package sync

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"

	syncv1 "github.com/captv89/ovl/pkg/syncproto/gen/ovl/sync/v1"
	"github.com/captv89/ovl/pkg/syncproto/gen/ovl/sync/v1/syncv1connect"

	"github.com/captv89/ovl/pkg/syncproto"
)

// Client is a vessel's authenticated connection to its office's
// SyncService (architecture 11). Construct with NewClient once a sync
// credential has been redeemed (see Redeem) — every RPC call is
// authenticated with that credential, which office/syncservice.
// AuthInterceptor requires on every method with no exceptions.
type Client struct {
	rpc syncv1connect.SyncServiceClient
}

// NewClient builds a Client for officeURL, authenticating every RPC with
// credential (a vessel_credential's plaintext token — see RedeemResult
// and vessel/store.SyncCredential). httpClient may be nil, in which case
// http.DefaultClient is used.
func NewClient(httpClient *http.Client, officeURL, credential string) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	officeURL = strings.TrimRight(strings.TrimSpace(officeURL), "/")

	authInterceptor := connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			req.Header().Set("Authorization", "Bearer "+credential)
			return next(ctx, req)
		}
	})
	opts := append([]connect.ClientOption{connect.WithInterceptors(authInterceptor)}, syncproto.ClientOptions()...)

	return &Client{rpc: syncv1connect.NewSyncServiceClient(httpClient, officeURL, opts...)}
}

// SyncStatus calls the office's SyncStatus RPC (architecture 11.2 step
// 4), reporting appVersion; which restore commands this vessel has
// applied since its last check-in (appliedRestoreCommandIDs, may be nil
// — architecture 12.5's DR push path, office's "confirming it's pushed"
// signal); which user-management commands it has applied
// (appliedUserCommandIDs, may be nil — architecture 9.3/12.4's remote
// user administration, same ack shape); this vessel's full current user
// roster (users, may be nil — office's only visibility into who exists on
// a vessel, since accounts are otherwise purely local); and the config
// bundle this vessel is currently running on (appliedBundleID/Version,
// empty/0 until any bundle is applied — audit 2026-07-22 §3.3, so office
// can tell whether the vessel is on its assigned bundle or behind).
// Returns the office's reported server time.
func (c *Client) SyncStatus(ctx context.Context, appVersion string, appliedRestoreCommandIDs, appliedUserCommandIDs []string, users []*syncv1.VesselUserSummary, appliedBundleID string, appliedBundleVersion int64) (time.Time, error) {
	resp, err := c.rpc.SyncStatus(ctx, connect.NewRequest(&syncv1.SyncStatusRequest{
		AppVersion: appVersion, AppliedRestoreCommandIds: appliedRestoreCommandIDs,
		AppliedUserCommandIds: appliedUserCommandIDs, Users: users,
		AppliedBundleId: appliedBundleID, AppliedBundleVersion: appliedBundleVersion,
	}))
	if err != nil {
		return time.Time{}, fmt.Errorf("sync: call SyncStatus: %w", err)
	}
	return resp.Msg.GetServerTime().AsTime(), nil
}

// FetchRestoreBundle calls the office's FetchRestoreBundle RPC
// (architecture 12.5's DR push path, second half): once this vessel has
// seen commandID as a RestoreCommand in a PullInbox pull, it fetches the
// actual encrypted bundle bytes here, over the same credential-
// authenticated connection — see the RPC's own proto doc comment for why
// this doesn't need a separate out-of-band file transfer.
func (c *Client) FetchRestoreBundle(ctx context.Context, commandID string) ([]byte, error) {
	resp, err := c.rpc.FetchRestoreBundle(ctx, connect.NewRequest(&syncv1.FetchRestoreBundleRequest{CommandId: commandID}))
	if err != nil {
		return nil, fmt.Errorf("sync: call FetchRestoreBundle: %w", err)
	}
	return resp.Msg.GetCiphertext(), nil
}

// PushOutbox sends a batch of outbox items to the office (architecture
// 11.2 step 1) and returns the per-item Acks the office returned — the
// caller (vessel/httpapi.RunSyncCycle) is responsible for acking each
// accepted item in its own store; this client is a thin RPC wrapper with
// no store dependency of its own, same as SyncStatus above. items and
// their proto types come from pkg/syncproto's conversion functions.
func (c *Client) PushOutbox(ctx context.Context, items []*syncv1.OutboxItem) ([]*syncv1.ItemAck, error) {
	resp, err := c.rpc.PushOutbox(ctx, connect.NewRequest(&syncv1.PushOutboxRequest{Items: items}))
	if err != nil {
		return nil, fmt.Errorf("sync: call PushOutbox: %w", err)
	}
	return resp.Msg.GetAcks(), nil
}

// PullInbox calls the office's PullInbox RPC (architecture 11.2 step 3),
// reporting cursors and returning the full response — the caller
// (vessel/httpapi.RunSyncCycle) is responsible for applying whatever
// comes back to its own store (vessel/store.ApplyInboxPull) and
// persisting the returned next_cursors; this client stays a thin RPC
// wrapper with no store dependency, same as SyncStatus/PushOutbox above.
func (c *Client) PullInbox(ctx context.Context, cursors *syncv1.SyncCursors) (*syncv1.PullInboxResponse, error) {
	resp, err := c.rpc.PullInbox(ctx, connect.NewRequest(&syncv1.PullInboxRequest{Cursors: cursors}))
	if err != nil {
		return nil, fmt.Errorf("sync: call PullInbox: %w", err)
	}
	return resp.Msg, nil
}
