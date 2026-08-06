// SPDX-License-Identifier: AGPL-3.0-only

// Package httpapi is ovl-office's HTTP surface: a health endpoint, the
// local-account login/session surface (architecture 12.2's stand-in
// pending real OIDC), the
// enrollment/config-authoring/bundle-publishing API surface, the vessel
// enrollment-code redemption endpoint (POST /api/enroll, Phase 4's sync
// handshake — see office/enrollment.Redeem and office/synccred), the
// SyncService ConnectRPC handler (office/syncservice, mounted alongside
// the JSON routes on the same mux/port), and the embedded React SPA.
package httpapi

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/99designs/gqlgen/graphql/playground"

	"github.com/captv89/ovl/internal/httpjson"
	"github.com/captv89/ovl/office/apikey"
	"github.com/captv89/ovl/office/auth"
	"github.com/captv89/ovl/office/graphql"
	"github.com/captv89/ovl/office/store"
	"github.com/captv89/ovl/office/syncservice"
	"github.com/captv89/ovl/pkg/attachmentstore"
	"github.com/captv89/ovl/pkg/schema"
	"github.com/captv89/ovl/pkg/syncproto"
	"github.com/captv89/ovl/pkg/syncproto/gen/ovl/sync/v1/syncv1connect"
)

// Server routes requests and holds the Postgres store.
type Server struct {
	st          *store.Store
	sessions    *sessionStore
	spa         http.Handler
	mux         *http.ServeMux
	validator   *schema.Validator
	attachments *attachmentstore.Store
	version     string
	graphql     http.Handler
	// secureCookies marks the session cookie Secure so browsers only send
	// it over HTTPS. Office is documented as deployed behind Caddy TLS, so
	// production sets this on; it defaults off because `make run-office`
	// and CI serve plain HTTP on localhost where a Secure cookie would
	// silently drop the session (see main.go's -secure-cookies flag).
	secureCookies bool
}

// NewServer wires the API routes and the embedded SPA handler. spa is
// the dist/ subtree (already fs.Sub'd) of the built React app. validator
// checks uploaded schema JSON against the meta-schema (design handoff
// B5's upload flow) — the same *schema.Validator used to seed the
// curated schemas at boot (see main.go's seedCuratedSchemas). attachments
// is the final content-addressed attachment store (architecture 15) and
// attachmentStagingDir is where in-progress chunk uploads assemble
// before promotion into it — see office/syncservice.New's own doc
// comment for why that's a sibling directory, not nested inside.
// version is main.go's own build-stamped version string, surfaced
// read-only on design handoff B10's Administration System tab.
// secureCookies marks the session cookie Secure (HTTPS-only) — on in
// production behind Caddy TLS, off for plain-HTTP local dev.
func NewServer(st *store.Store, spa fs.FS, validator *schema.Validator, attachments *attachmentstore.Store, attachmentStagingDir string, version string, secureCookies bool) *Server {
	s := &Server{
		st:            st,
		sessions:      newSessionStore(),
		spa:           newSPAHandler(spa),
		validator:     validator,
		attachments:   attachments,
		version:       version,
		graphql:       graphql.NewHandler(st),
		secureCookies: secureCookies,
	}
	s.mux = http.NewServeMux()
	s.routes()
	s.mountSyncService(attachments, attachmentStagingDir)
	return s
}

// mountSyncService wires office/syncservice's SyncService handler onto
// this server's mux, credential-gated by syncservice.AuthInterceptor for
// every RPC (architecture 11.1) and zstd-compression-enabled
// (pkg/syncproto.ServerOptions, matching the vessel client's
// pkg/syncproto.ClientOptions).
func (s *Server) mountSyncService(attachments *attachmentstore.Store, attachmentStagingDir string) {
	opts := append([]connect.HandlerOption{
		connect.WithInterceptors(syncservice.AuthInterceptor(s.st)),
	}, syncproto.ServerOptions()...)
	path, handler := syncv1connect.NewSyncServiceHandler(syncservice.New(s.st, attachments, attachmentStagingDir), opts...)
	s.mux.Handle(path, handler)
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/health", s.handleHealth)

	s.mux.HandleFunc("GET /api/setup/status", s.handleSetupStatus)
	s.mux.HandleFunc("POST /api/setup/admin", s.handleSetupAdmin)

	s.mux.HandleFunc("POST /api/enroll", s.handleRedeemEnrollment)

	s.mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	s.mux.HandleFunc("POST /api/auth/logout", s.handleLogout)
	s.mux.HandleFunc("GET /api/auth/me", s.handleMe)
	s.mux.HandleFunc("POST /api/auth/change-password", s.handleChangePassword)

	s.mux.HandleFunc("GET /api/dashboard", s.handleGetDashboard)

	s.mux.HandleFunc("GET /api/notifications", s.handleListNotifications)
	s.mux.HandleFunc("POST /api/notifications/mark-read", s.handleMarkNotificationsRead)

	s.mux.HandleFunc("GET /api/system", s.handleGetSystem)

	s.mux.HandleFunc("GET /api/users", s.handleListUsers)
	s.mux.HandleFunc("POST /api/users", s.handleCreateUser)
	s.mux.HandleFunc("PUT /api/users/{id}/roles", s.handleUpdateUserRoles)
	s.mux.HandleFunc("POST /api/users/{id}/deactivate", s.handleDeactivateUser)
	s.mux.HandleFunc("POST /api/users/{id}/reactivate", s.handleReactivateUser)
	s.mux.HandleFunc("POST /api/users/{id}/reset-password", s.handleResetUserPassword)

	s.mux.HandleFunc("GET /api/api-keys", s.handleListAPIKeys)
	s.mux.HandleFunc("POST /api/api-keys", s.handleCreateAPIKey)
	s.mux.HandleFunc("POST /api/api-keys/{id}/revoke", s.handleRevokeAPIKey)
	s.mux.HandleFunc("DELETE /api/api-keys/{id}", s.handleDeleteAPIKey)
	s.mux.HandleFunc("GET /api/api-keys/{id}/events", s.handleListAPIKeyEvents)

	s.mux.HandleFunc("POST /api/vessel-groups/rename", s.handleRenameVesselGroup)
	s.mux.HandleFunc("POST /api/vessel-groups/delete", s.handleDeleteVesselGroup)

	s.mux.HandleFunc("GET /api/vessels/positions", s.handleListVesselPositions)
	s.mux.HandleFunc("GET /api/vessels", s.handleListVessels)
	s.mux.HandleFunc("POST /api/vessels", s.handleCreateVessel)
	s.mux.HandleFunc("GET /api/vessels/{id}", s.handleGetVessel)
	s.mux.HandleFunc("PUT /api/vessels/{id}", s.handleUpdateVessel)
	s.mux.HandleFunc("POST /api/vessels/{id}/enrollment/issue", s.handleIssueEnrollment)
	s.mux.HandleFunc("POST /api/vessels/{id}/enrollment/reissue", s.handleReissueEnrollment)
	s.mux.HandleFunc("POST /api/vessels/{id}/enrollment/revoke", s.handleRevokeEnrollment)
	s.mux.HandleFunc("GET /api/vessels/{id}/restore-bundle", s.handleGenerateRestoreBundle)
	s.mux.HandleFunc("POST /api/vessels/{id}/restore-bundle/push", s.handlePushRestoreBundle)

	s.mux.HandleFunc("POST /api/vessels/{id}/users", s.handleCreateVesselUser)
	s.mux.HandleFunc("POST /api/vessels/{id}/users/{username}/reset-password", s.handleResetVesselUserPassword)
	s.mux.HandleFunc("PUT /api/vessels/{id}/users/{username}/role", s.handleSetVesselUserRole)
	s.mux.HandleFunc("PUT /api/vessels/{id}/users/{username}/can-submit", s.handleSetVesselUserCanSubmit)
	s.mux.HandleFunc("POST /api/vessels/{id}/users/{username}/deactivate", s.handleDeactivateVesselUser)
	s.mux.HandleFunc("POST /api/vessels/{id}/users/{username}/reactivate", s.handleReactivateVesselUser)

	s.mux.HandleFunc("GET /api/schema-versions", s.handleListLatestSchemaVersions)
	s.mux.HandleFunc("GET /api/schema-versions/{name}/versions", s.handleListSchemaVersionHistory)
	s.mux.HandleFunc("GET /api/schema-versions/{name}/versions/{version}", s.handleGetSchemaVersion)
	s.mux.HandleFunc("GET /api/schema-versions/{name}/versions/{version}/download", s.handleDownloadSchemaVersion)
	s.mux.HandleFunc("POST /api/schema-versions/{name}/preview", s.handlePreviewSchemaUpload)
	s.mux.HandleFunc("POST /api/schema-versions/{name}/publish", s.handlePublishSchemaVersion)

	s.mux.HandleFunc("GET /api/field-policies/{name}", s.handleGetFieldPolicy)
	s.mux.HandleFunc("PUT /api/field-policies/{name}", s.handleSaveFieldPolicy)
	s.mux.HandleFunc("GET /api/field-policies/{name}/assignments", s.handleListFieldPolicyAssignments)

	s.mux.HandleFunc("GET /api/compliance/profiles", s.handleListProfileAssignments)
	s.mux.HandleFunc("PUT /api/compliance/profiles", s.handleSaveProfileAssignment)
	s.mux.HandleFunc("GET /api/compliance/cadence", s.handleListCadenceRules)
	s.mux.HandleFunc("PUT /api/compliance/cadence", s.handleSaveCadenceRule)
	s.mux.HandleFunc("GET /api/compliance/rules", s.handleListOverridableRules)
	s.mux.HandleFunc("GET /api/compliance/rule-severities", s.handleListRuleSeverityAssignments)
	s.mux.HandleFunc("PUT /api/compliance/rule-severities", s.handleSaveRuleSeverityAssignment)

	s.mux.HandleFunc("GET /api/config-bundles", s.handleListConfigBundles)
	s.mux.HandleFunc("GET /api/config-bundles/preview", s.handlePreviewConfigBundle)
	s.mux.HandleFunc("POST /api/config-bundles/publish", s.handlePublishConfigBundle)
	s.mux.HandleFunc("GET /api/config-bundles/assignments", s.handleListBundleAssignments)
	s.mux.HandleFunc("PUT /api/config-bundles/assignments", s.handleSaveBundleAssignment)
	s.mux.HandleFunc("GET /api/vessel-configs", s.handleListVesselConfigs)

	s.mux.HandleFunc("POST /api/commercial/{schemaName}/reports", s.handleCreateCommercialReport)

	s.mux.HandleFunc("GET /api/reports", s.handleListReports)
	s.mux.HandleFunc("POST /api/reports/mark-reviewed", s.handleBulkMarkReviewed)
	s.mux.HandleFunc("GET /api/reports/{vesselId}/{reportId}", s.handleGetReport)
	s.mux.HandleFunc("GET /api/reports/{vesselId}/{reportId}/events", s.handleListReportEvents)
	s.mux.HandleFunc("GET /api/reports/{vesselId}/{reportId}/versions", s.handleListReportVersions)
	s.mux.HandleFunc("GET /api/reports/{vesselId}/{reportId}/chat", s.handleListChat)
	s.mux.HandleFunc("POST /api/reports/{vesselId}/{reportId}/chat", s.handlePostChat)
	s.mux.HandleFunc("GET /api/reports/{vesselId}/{reportId}/remarks", s.handleListRemarks)
	s.mux.HandleFunc("POST /api/reports/{vesselId}/{reportId}/remarks", s.handleCreateRemarkSet)
	s.mux.HandleFunc("GET /api/reports/{vesselId}/{reportId}/attachments", s.handleListReportAttachments)
	s.mux.HandleFunc("GET /api/reports/{vesselId}/{reportId}/attachments/{attachmentId}", s.handleDownloadReportAttachment)
	s.mux.HandleFunc("PATCH /api/remarks/{id}", s.handleSetRemarkResolved)

	s.mux.HandleFunc("POST /api/v1/graphql", s.handleGraphQL)
	s.mux.HandleFunc("GET /api/v1/graphql/playground", s.handleGraphQLPlayground)
	s.mux.HandleFunc("GET /api/v1/reports.csv", s.handleReportsCSV)

	s.mux.Handle("/", s.spa)
}

// Handler returns the http.Handler to serve.
func (s *Server) Handler() http.Handler {
	return s.mux
}

// Close releases the store.
func (s *Server) Close() error {
	return s.st.Close()
}

// userView is the JSON shape returned for "the current user" across
// setup/login/me responses, matching vessel/httpapi's own userView shape
// on office's combinable Roles rather than a single Role.
type userView struct {
	ID                 string     `json:"id"`
	Username           string     `json:"username"`
	Roles              auth.Roles `json:"roles"`
	MustChangePassword bool       `json:"mustChangePassword"`
	Active             bool       `json:"active"`
	CreatedAt          time.Time  `json:"createdAt"`
}

func toUserView(u *auth.User) userView {
	return userView{
		ID:                 u.ID,
		Username:           u.Username,
		Roles:              u.Roles,
		MustChangePassword: u.MustChangePassword,
		Active:             u.Active,
		CreatedAt:          u.CreatedAt,
	}
}

// authenticatedUser resolves the session cookie on r to a User, or
// writes a 401 and returns ok=false.
func (s *Server) authenticatedUser(w http.ResponseWriter, r *http.Request) (*auth.User, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		httpjson.WriteError(w, http.StatusUnauthorized, "not logged in")
		return nil, false
	}
	userID, ok := s.sessions.lookup(cookie.Value)
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "session expired")
		return nil, false
	}
	u, err := s.st.GetUser(context.Background(), userID)
	if err != nil {
		httpjson.WriteError(w, http.StatusUnauthorized, "session refers to an unknown user")
		return nil, false
	}
	// A deactivated account (design handoff B10) must be cut off
	// immediately, not just blocked at its next login — same "revoking
	// must also cut off anything already issued" reasoning as
	// handleRevokeEnrollment. The session token itself isn't destroyed
	// here (Reactivate should resume it without a fresh login), only
	// rejected while inactive.
	if !u.Active {
		httpjson.WriteError(w, http.StatusUnauthorized, "session expired")
		return nil, false
	}
	return u, true
}

// authenticatedAPIKey resolves the Authorization: Bearer header on r to
// an *apikey.APIKey, or writes a 401 and returns ok=false. Parallel to
// authenticatedUser but a separate mechanism entirely — the data API
// (GraphQL/CSV, architecture 13.1) is external-customer-facing and never
// accepts the office-staff session cookie, and office staff never
// present a bearer API key. Same "O(1) lookup-hash, then slow-hash
// verify" shape office/syncservice's own vessel-credential auth
// interceptor already uses, extended here rather than duplicated
// differently.
func (s *Server) authenticatedAPIKey(w http.ResponseWriter, r *http.Request) (*apikey.APIKey, bool) {
	authHeader := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(authHeader, prefix) {
		httpjson.WriteError(w, http.StatusUnauthorized, "missing bearer API key")
		return nil, false
	}
	token := strings.TrimPrefix(authHeader, prefix)
	k, err := s.st.GetAPIKeyByLookupHash(r.Context(), apikey.LookupHash(token))
	if err != nil {
		httpjson.WriteError(w, http.StatusUnauthorized, "invalid API key")
		return nil, false
	}
	ok, err := k.Verify(token)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return nil, false
	}
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "invalid API key")
		return nil, false
	}
	now := time.Now().UTC()
	if err := s.st.TouchAPIKeyLastUsed(r.Context(), k.ID, now); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return nil, false
	}
	return k, true
}

// handleGraphQL serves the external data API (architecture 13.2).
// authenticatedAPIKey does the actual gating for a real bearer-token
// caller; this handler's own job is just to attach the resolved key to
// the request context (so office/graphql's Reports resolver can enforce
// the key's own vessel-group scope) before delegating to the generated
// GraphQL executor.
//
// A request with no Authorization header at all takes a second path
// instead of failing outright: office's own Admin, already logged in
// via the browser session cookie handleGraphQLPlayground itself already
// requires, gets treated as an unscoped (GroupID nil) caller with the
// same full visibility Admin already has everywhere else in the office
// UI — no separate API key to mint and copy-paste just to click around
// the interactive playground. This is not a relaxation of the real
// external contract: a genuine external caller always sends a bearer
// token (that's the whole point of the credential), so it always takes
// the first branch and is scoped exactly as before; only a same-origin
// browser request with the admin session cookie and no bearer header
// can reach the second branch at all. No api_key_events row exists for
// a session-based caller, so RecordAPIKeyEvent is skipped for it — that
// audit trail is specifically for API-key usage, not staff activity
// (already tracked by other means, per office's existing audit trail).
func (s *Server) handleGraphQL(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") == "" {
		user, ok := s.requireAdmin(w, r)
		if !ok {
			return
		}
		sessionKey := &apikey.APIKey{
			ID:        "admin-session:" + user.ID,
			Label:     "Admin session (" + user.Username + ")",
			CreatedBy: user.Username,
		}
		s.graphql.ServeHTTP(w, r.WithContext(graphql.WithAPIKey(r.Context(), sessionKey)))
		return
	}
	key, ok := s.authenticatedAPIKey(w, r)
	if !ok {
		return
	}
	if err := s.st.RecordAPIKeyEvent(r.Context(), key.ID, "usedGraphQL", time.Now().UTC()); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.graphql.ServeHTTP(w, r.WithContext(graphql.WithAPIKey(r.Context(), key)))
}

// handleGraphQLPlayground serves gqlgen's built-in query-builder UI
// against /api/v1/graphql (18.07.26 manual-test item 5: "API
// documentation and a playground for the office side GraphQL API" —
// gqlgen's own schema/docs sidebar covers the documentation half; the
// README's existing curl walkthrough covers external-consumer docs).
// Gated behind requireAdmin, same as the query endpoint's own
// session-cookie branch above — an Admin who can reach this page can
// already query through it with no separate key. The Headers panel
// pre-fill this comment used to describe (paste a bearer key before
// GraphiQL's automatic schema-introspection call, or it 401s) is gone:
// handleGraphQL's session-cookie branch means that first automatic
// request just succeeds on its own, using the same cookie that got you
// this page, so there's nothing to paste and no error to explain away.
// Pasting a real API key into the Headers panel is still meaningful,
// just optional now — it overrides the session identity for that
// request, letting an Admin verify what a specific *scoped* key would
// actually see (its own vessel-group restriction) rather than the
// unscoped view their own session gets.
func (s *Server) handleGraphQLPlayground(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	playground.Handler("OVL Data API", "/api/v1/graphql").ServeHTTP(w, r)
}

func (s *Server) startSession(w http.ResponseWriter, userID string) error {
	token, err := s.sessions.create(userID)
	if err != nil {
		return fmt.Errorf("start session: %w", err)
	}
	http.SetCookie(w, s.sessionCookie(token, int(sessionTTL.Seconds())))
	return nil
}

// sessionCookie builds the session cookie so login and logout stay in
// lockstep on every attribute — a logout cookie must match the login
// cookie's Secure/HttpOnly/SameSite/Path to reliably overwrite it.
func (s *Server) sessionCookie(value string, maxAge int) *http.Cookie {
	return &http.Cookie{ // #nosec G124 -- Secure is s.secureCookies (see its own doc comment), not a hardcoded false; gosec can't verify a runtime bool
		Name:     sessionCookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.secureCookies,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   maxAge,
	}
}
