// SPDX-License-Identifier: AGPL-3.0-only

package graphql

import (
	"net/http"

	"github.com/99designs/gqlgen/graphql/handler"

	"github.com/captv89/ovl/office/store"
)

// NewHandler builds the GraphQL HTTP handler for architecture 13.2's
// external data API. Mounted by office/httpapi's own handleGraphQL,
// which authenticates the bearer API key first (authenticatedAPIKey)
// and attaches it to the request context (WithAPIKey) before delegating
// here — this handler itself has no auth of its own, matching how
// office/syncservice's own ConnectRPC handler is a plain handler wrapped
// by a separate interceptor, not self-authenticating.
func NewHandler(st *store.Store) http.Handler {
	return handler.NewDefaultServer(NewExecutableSchema(Config{Resolvers: &Resolver{Store: st}}))
}
