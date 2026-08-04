// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"context"
	"maps"
	"net/http"

	"github.com/captv89/ovl/internal/httpjson"
	"github.com/captv89/ovl/pkg/configwire"
	"github.com/captv89/ovl/pkg/validation"
)

// This file is the vessel side of config-bundle *application* (codebase
// audit 2026-07-22 §1). Until now the vessel pulled and stored the office's
// config bundle but rendered forms and ran validation from hardcoded
// stand-ins (fieldConfigFor/validationConfigFor/AllRegulatoryProfiles), so
// everything a Config Manager authored ashore was decorative on board.
// These resolvers make the applied bundle authoritative: the vessel reads
// the highest-version bundle from its local config_bundles table, decodes
// the tagged pkg/configwire.Bundle, and uses its resolved field policy,
// prefill classes, rule severities and regulatory profiles. A vessel with
// no bundle synced yet — or one holding an undecodable (stale-format or
// corrupt) bundle — falls back to the same built-in stand-ins as before,
// so a fresh or offline vessel is never worse off than it was.

// appliedBundle returns this vessel's currently-applied config bundle, or
// nil if none is synced or the stored content can't be decoded (callers
// then fall back to the built-in stand-in config).
func (s *Server) appliedBundle(ctx context.Context) *configwire.Bundle {
	st := s.storeOrNil()
	if st == nil {
		return nil
	}
	cb, ok, err := st.LatestConfigBundle(ctx)
	if err != nil || !ok {
		return nil
	}
	b, err := configwire.Decode(cb.Content)
	if err != nil {
		// A bundle we can't decode (e.g. a pre-configwire raw bundle left
		// from before this feature, or a future wireVersion) must not fail
		// the form load — degrade to the stand-in config instead.
		return nil
	}
	return b
}

// fieldConfigFromBundle resolves the field policy + per-event applicability
// + prefill classes for schemaName from an already-decoded bundle (nil ⇒
// built-in stand-in). The returned policy map is a copy the caller may
// mutate (handleGetSchema layers real prefill values on top). When a bundle
// is applied it is authoritative even if it carries no override for a field
// — that field simply takes its default — so the demo stand-in is used only
// when there is no bundle at all.
//
// The returned events map is empty when the bundle narrows no field to
// specific voyage events, which is both the pre-2026-07-28 shape and what an
// older office still sends: an empty map means every field applies to every
// event (validation.FieldEvents.AppliesTo), so nothing is hidden by default.
func fieldConfigFromBundle(b *configwire.Bundle, schemaName string) (map[string]string, validation.FieldEvents, map[string]*prefillEntry) {
	if b == nil {
		policy, prefill := fieldConfigFor(schemaName)
		return policy, validation.FieldEvents{}, prefill
	}
	policy := map[string]string{}
	events := validation.FieldEvents{}
	prefill := map[string]*prefillEntry{}
	if sc := b.SchemaConfigFor(schemaName); sc != nil {
		policy = maps.Clone(sc.Policy)
		if policy == nil {
			policy = map[string]string{}
		}
		maps.Copy(events, sc.Events)
		// Only the prefill *class* comes from the bundle; the actual value
		// is computed by handleGetSchema from this vessel's report history.
		for field, class := range sc.Prefill {
			prefill[field] = &prefillEntry{Class: class}
		}
	}
	return policy, events, prefill
}

// validationConfigFromBundle returns the plausibility/continuity config for
// schemaName: the curated base (series + tolerances, still shared and not
// company-authored — architecture 8.3) with the bundle's resolved
// per-rule severity overrides layered on. nil bundle ⇒ base only.
func validationConfigFromBundle(b *configwire.Bundle, schemaName string) *validation.Config {
	cfg := validationConfigFor(schemaName)
	if b != nil {
		sev := make(map[string]validation.Severity, len(b.RuleSeverities))
		for id, v := range b.RuleSeverities {
			sev[id] = validation.Severity(v)
		}
		cfg.Severities = sev
	}
	return cfg
}

// profilesFromBundle returns the regulatory profiles enabled for this
// vessel. nil bundle ⇒ every known profile (the pre-config-bundle
// behavior), so an un-synced vessel keeps showing all readiness chips
// rather than none.
func profilesFromBundle(b *configwire.Bundle) []validation.RegulatoryProfile {
	if b == nil {
		return validation.AllRegulatoryProfiles
	}
	out := make([]validation.RegulatoryProfile, len(b.RegulatoryProfiles))
	for i, p := range b.RegulatoryProfiles {
		out[i] = validation.RegulatoryProfile(p)
	}
	return out
}

// maxGapHoursFromBundle returns the resolved cadence ceiling (architecture
// 6.3), defaulting to 12h when no bundle is applied — the same constant the
// vessel frontend used to hardcode in homeData.ts.
func maxGapHoursFromBundle(b *configwire.Bundle) float64 {
	if b != nil && b.MaxGapHours > 0 {
		return b.MaxGapHours
	}
	return 12
}

// vesselConfigView is the frontend-facing slice of this vessel's applied
// config bundle: the operational settings the SPA needs directly (as
// opposed to field policy/prefill, which ride along with each schema
// fetch, and rule severities, which only the server-side engine consumes).
type vesselConfigView struct {
	MaxGapHours        float64  `json:"maxGapHours"`
	RegulatoryProfiles []string `json:"regulatoryProfiles"`
	BundleID           string   `json:"bundleId,omitempty"`
	BundleVersionNo    int64    `json:"bundleVersionNo,omitempty"`
}

// handleGetVesselConfig exposes the applied config bundle's operational
// settings to the SPA: the cadence ceiling (Home's overdue banner,
// replacing homeData.ts's hardcoded 12h) and the enabled regulatory
// profiles. Any authenticated user may read it — like the sync settings,
// it is vessel-wide operational config, not per-user. Falls back to the
// built-in defaults when no bundle is applied yet.
func (s *Server) handleGetVesselConfig(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	b := s.appliedBundle(r.Context())
	profiles := profilesFromBundle(b)
	profileStrs := make([]string, len(profiles))
	for i, p := range profiles {
		profileStrs[i] = string(p)
	}
	view := vesselConfigView{
		MaxGapHours:        maxGapHoursFromBundle(b),
		RegulatoryProfiles: profileStrs,
	}
	if b != nil {
		view.BundleID = b.BundleID
		view.BundleVersionNo = b.VersionNo
	}
	httpjson.WriteJSON(w, http.StatusOK, view)
}
