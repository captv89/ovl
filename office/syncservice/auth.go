// SPDX-License-Identifier: AGPL-3.0-only

package syncservice

import (
	"context"
	"errors"
	"strings"
	"time"

	"connectrpc.com/connect"

	"github.com/captv89/ovl/office/store"
	"github.com/captv89/ovl/office/synccred"
)

// errUnauthenticatedContext signals AuthInterceptor was not applied to
// this RPC — every SyncService method requires it, with no exceptions
// (that's exactly why enrollment redemption is a separate plain
// endpoint, office/httpapi.handleRedeemEnrollment, not an RPC on this
// service).
var errUnauthenticatedContext = errors.New("syncservice: no authenticated vessel in context")

type vesselIDContextKey struct{}

// VesselIDFromContext returns the vessel id AuthInterceptor resolved for
// the current RPC, if any.
func VesselIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(vesselIDContextKey{}).(string)
	return id, ok
}

// AuthInterceptor authenticates every SyncService RPC against a
// vessel_credentials row (architecture 11.1: "Authentication via the
// per-vessel credential from enrollment, revocable in the office").
// Applied to the whole service with no per-method exceptions — see
// office/httpapi.handleRedeemEnrollment's doc comment for why enrollment
// itself is a separate, unauthenticated endpoint instead of a carve-out
// here.
func AuthInterceptor(st *store.Store) connect.Interceptor {
	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			token := bearerToken(req.Header().Get("Authorization"))
			if token == "" {
				return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("missing bearer credential"))
			}

			cred, err := st.GetVesselCredentialByLookupHash(ctx, synccred.LookupHash(token))
			if errors.Is(err, store.ErrNotFound) {
				return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("credential not recognized"))
			}
			if err != nil {
				return nil, connect.NewError(connect.CodeInternal, err)
			}
			match, err := cred.Verify(token)
			if err != nil {
				return nil, connect.NewError(connect.CodeInternal, err)
			}
			if !match {
				return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("credential not recognized"))
			}

			if err := st.TouchVesselCredentialLastUsed(ctx, cred.VesselID, time.Now().UTC()); err != nil {
				return nil, connect.NewError(connect.CodeInternal, err)
			}

			ctx = context.WithValue(ctx, vesselIDContextKey{}, cred.VesselID)
			return next(ctx, req)
		}
	})
}

// bearerToken extracts the token from an "Authorization: Bearer <token>"
// header value, or returns "" if the header is missing or malformed.
func bearerToken(header string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, prefix))
}
