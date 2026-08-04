// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/captv89/ovl/internal/httpjson"
	"github.com/captv89/ovl/office/configbundle"
	"github.com/captv89/ovl/office/fieldpolicy"
	"github.com/captv89/ovl/office/store"
)

// composeCurrentBundle builds an unpublished bundle snapshot from every
// config component's *current* live state (design handoff B7's bundle
// composer: "showing exactly what goes in") — the latest published
// version of every known schema, that version's field policy, and every
// regulatory profile/cadence/rule-severity assignment across all scopes.
// DefaultRoleNames is always nil: architecture 9.2's default-role-set
// field is inert data only until Phase 4 sync exists to deliver a
// bundle at all (see PROJECT.md's 2026-07-07 decisions log) — nothing
// in this office UI ever sets it.
func (s *Server) composeCurrentBundle(ctx context.Context) (*configbundle.ConfigBundle, error) {
	names, err := s.st.ListSchemaNames(ctx)
	if err != nil {
		return nil, fmt.Errorf("list schema names: %w", err)
	}
	schemaVersions := make([]configbundle.SchemaVersionRef, 0, len(names))
	var fieldPolicies []*fieldpolicy.SchemaFieldPolicy
	for _, name := range names {
		latest, err := s.st.LatestSchemaVersion(ctx, name)
		if err != nil {
			return nil, fmt.Errorf("latest schema version for %s: %w", name, err)
		}
		schemaVersions = append(schemaVersions, configbundle.SchemaVersionRef{
			SchemaName: latest.SchemaName, Version: latest.Version, ID: latest.ID,
		})
		// Every scope's assignment for this (schema, version) — not just
		// one global policy — so a vessel's effective field policy can
		// still be resolved (fieldpolicy.EffectiveFieldPolicy) once this
		// bundle reaches it, matching how RegulatoryProfiles/CadenceRules/
		// RuleSeverities below already embed every scope's assignment.
		assignments, err := s.st.ListFieldPolicyAssignments(ctx, name, latest.Version)
		if err != nil {
			return nil, fmt.Errorf("list field policy assignments for %s@%s: %w", name, latest.Version, err)
		}
		fieldPolicies = append(fieldPolicies, assignments...)
	}

	profiles, err := s.st.ListProfileAssignments(ctx)
	if err != nil {
		return nil, fmt.Errorf("list profile assignments: %w", err)
	}
	cadence, err := s.st.ListCadenceRules(ctx)
	if err != nil {
		return nil, fmt.Errorf("list cadence rules: %w", err)
	}
	severities, err := s.st.ListRuleSeverityAssignments(ctx)
	if err != nil {
		return nil, fmt.Errorf("list rule severity assignments: %w", err)
	}

	return configbundle.Compose(schemaVersions, fieldPolicies, profiles, cadence, severities, nil), nil
}

// --- Bundle history ---

type configBundleSummaryView struct {
	ID                    string    `json:"id"`
	Label                 string    `json:"label"`
	SchemaVersionCount    int       `json:"schemaVersionCount"`
	FieldPolicyRows       int       `json:"fieldPolicyRows"`
	RegulatoryProfileRows int       `json:"regulatoryProfileRows"`
	CadenceRuleRows       int       `json:"cadenceRuleRows"`
	RuleSeverityRows      int       `json:"ruleSeverityRows"`
	PublishedAt           time.Time `json:"publishedAt"`
	PublishedBy           string    `json:"publishedBy"`
}

func toConfigBundleSummaryView(b *configbundle.ConfigBundle) configBundleSummaryView {
	return configBundleSummaryView{
		ID: b.ID, Label: b.Label, SchemaVersionCount: len(b.SchemaVersions),
		FieldPolicyRows:       len(b.FieldPolicies),
		RegulatoryProfileRows: len(b.RegulatoryProfiles), CadenceRuleRows: len(b.CadenceRules),
		RuleSeverityRows: len(b.RuleSeverities), PublishedAt: b.PublishedAt, PublishedBy: b.PublishedBy,
	}
}

func (s *Server) handleListConfigBundles(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	list, err := s.st.ListConfigBundles(r.Context())
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]configBundleSummaryView, len(list))
	for i, b := range list {
		out[i] = toConfigBundleSummaryView(b)
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}

// handlePreviewConfigBundle serves design handoff B7's "bundle composer
// showing exactly what goes in" without publishing anything.
func (s *Server) handlePreviewConfigBundle(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	composed, err := s.composeCurrentBundle(r.Context())
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toConfigBundleSummaryView(composed))
}

type publishBundleRequest struct {
	Label string `json:"label"`
}

// handlePublishConfigBundle recomputes composeCurrentBundle fresh (not a
// two-step compose-then-publish round trip through the client — unlike
// schema-version upload, this content is already-valid live application
// state, not user-uploaded bytes needing re-validation, so there is
// nothing to gain from echoing it back through the client between
// requests) and publishes it as a new immutable bundle. Config-Manager-
// only, per design handoff B7.
func (s *Server) handlePublishConfigBundle(w http.ResponseWriter, r *http.Request) {
	user, ok := s.authenticatedUser(w, r)
	if !ok {
		return
	}
	if !user.CanManageConfig() {
		httpjson.WriteError(w, http.StatusForbidden, "only Config Manager may publish config bundles")
		return
	}
	var req publishBundleRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	composed, err := s.composeCurrentBundle(r.Context())
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	bundle, err := configbundle.Publish(composed, req.Label, user.ID)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.st.CreateConfigBundle(r.Context(), bundle); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, toConfigBundleSummaryView(bundle))
}

// --- Bundle assignments ---
//
// Reuses bundleAssignmentView (vessels.go) for the response shape — same
// "which bundle, from where" data B2's Config tab already reads, just
// listed here across every scope rather than resolved for one vessel.

func (s *Server) handleListBundleAssignments(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	list, err := s.st.ListBundleAssignments(r.Context())
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]bundleAssignmentView, len(list))
	for i, a := range list {
		view := bundleAssignmentView{ScopeType: string(a.Scope.Type), ScopeKey: a.Scope.Key, BundleID: a.BundleID}
		if bundle, err := s.st.GetConfigBundle(r.Context(), a.BundleID); err == nil {
			view.BundleLabel = bundle.Label
			view.PublishedAt = bundle.PublishedAt
		}
		out[i] = view
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}

type saveBundleAssignmentRequest struct {
	Scope    scopeView `json:"scope"`
	BundleID string    `json:"bundleId"`
}

func (s *Server) handleSaveBundleAssignment(w http.ResponseWriter, r *http.Request) {
	if !s.requireConfigManager(w, r) {
		return
	}
	var req saveBundleAssignmentRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	scope, err := req.Scope.toScope()
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	bundle, err := s.st.GetConfigBundle(r.Context(), req.BundleID)
	if errors.Is(err, store.ErrNotFound) {
		httpjson.WriteError(w, http.StatusNotFound, "bundle not found")
		return
	} else if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a, err := configbundle.NewBundleAssignment(scope, req.BundleID)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.st.SaveBundleAssignment(r.Context(), a); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, bundleAssignmentView{
		ScopeType: string(a.Scope.Type), ScopeKey: a.Scope.Key, BundleID: a.BundleID,
		BundleLabel: bundle.Label, PublishedAt: bundle.PublishedAt,
	})
}
