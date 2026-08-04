package graphql

import (
	"context"

	"github.com/captv89/ovl/office/apikey"
)

type contextKey int

const apiKeyContextKey contextKey = iota

// WithAPIKey attaches the already-authenticated API key to ctx — set by
// office/httpapi's handleGraphQL right after authenticatedAPIKey
// succeeds, read back by the Reports resolver to enforce the key's own
// vessel-group scope regardless of what a query argument requests.
func WithAPIKey(ctx context.Context, k *apikey.APIKey) context.Context {
	return context.WithValue(ctx, apiKeyContextKey, k)
}

// APIKeyFromContext retrieves the key WithAPIKey attached, if any.
func APIKeyFromContext(ctx context.Context) (*apikey.APIKey, bool) {
	k, ok := ctx.Value(apiKeyContextKey).(*apikey.APIKey)
	return k, ok
}
