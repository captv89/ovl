// SPDX-License-Identifier: AGPL-3.0-only

// Package configbundle models the office side of config bundle publishing
// (architecture 6.5; design handoff B7's bundle composer). A ConfigBundle
// is a frozen, immutable snapshot of everything architecture 6.5 says a
// bundle contains — schema version references, field policies per
// schema, regulatory profiles, cadence rules, rule severities, and a
// default vessel role set (prefill overrides are folded into field
// policy already, see office/fieldpolicy.SchemaFieldPolicy.Prefill) — at
// the moment it was published, decoupled from whatever office/fieldpolicy,
// office/compliance, and office/schemaversions hold *now*, which keep
// changing after publish.
//
// Assignment (which scope gets which bundle) and per-vessel pulled/
// pending state are separate concerns from the bundle content itself —
// see BundleAssignment below. The actual pull mechanism ("the vessel
// pulls the newest assigned bundle on every sync and applies it
// atomically") is Phase 4 sync work and does not exist yet; this package
// only models what the office authors and assigns.
package configbundle

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/captv89/ovl/office/compliance"
	"github.com/captv89/ovl/office/fieldpolicy"
	"github.com/captv89/ovl/pkg/configwire"
)

// SchemaVersionRef identifies one schema's version included in a bundle
// by reference rather than by duplicating its content — schema versions
// are already immutable (architecture 5.2), so a bundle pointing at one
// by (SchemaName, Version, ID) is exactly how a report already references
// "the schema version it was created under."
type SchemaVersionRef struct {
	SchemaName string
	Version    string
	ID         string // office/schemaversions.SchemaVersion.ID
}

// ConfigBundle is one published, immutable configuration snapshot
// (architecture 6.5). There is no update method — every field is set
// once by Publish and never changed again, same as
// office/schemaversions.SchemaVersion.
type ConfigBundle struct {
	ID    string
	Label string // optional human label for the bundle history list (design handoff B7); may be empty

	SchemaVersions     []SchemaVersionRef
	FieldPolicies      []*fieldpolicy.SchemaFieldPolicy
	RegulatoryProfiles []*compliance.ProfileAssignment
	CadenceRules       []*compliance.CadenceRule
	RuleSeverities     []*compliance.RuleSeverityAssignment

	// DefaultRoleNames is architecture 9.2's "the default role set
	// arrives with the vessel's first config bundle" — inert data only.
	// vessel/auth.Role stays a fixed Go enum; this field carries plain
	// role name strings with no coupling to vessel/auth (office and
	// vessel share no Go package between them beyond pkg/*, and nothing
	// reads this field yet). Confirmed with the user 2026-07-07: wiring
	// this for real would mean touching the already-working, browser-
	// verified vessel first-run wizard/login for no real benefit, since
	// there is no Phase 4 sync mechanism yet to deliver a bundle to a
	// vessel at all. Revisit once that sync mechanism is real.
	DefaultRoleNames []string

	PublishedAt time.Time
	PublishedBy string
}

// Compose builds an unpublished bundle from the current state of each
// config component (design handoff B7's "bundle composer showing exactly
// what goes in"), deep-copying every mutable map/slice so that editing
// the live office/fieldpolicy or office/compliance data afterward — e.g.
// someone changes a field policy while the composer dialog is still open
// — can never retroactively change a bundle that has already been
// composed (and, once Publish is called on it, published and immutable).
// The returned bundle has no ID/PublishedAt/PublishedBy yet; call Publish
// to finalize it.
func Compose(
	schemaVersions []SchemaVersionRef,
	fieldPolicies []*fieldpolicy.SchemaFieldPolicy,
	regulatoryProfiles []*compliance.ProfileAssignment,
	cadenceRules []*compliance.CadenceRule,
	ruleSeverities []*compliance.RuleSeverityAssignment,
	defaultRoleNames []string,
) *ConfigBundle {
	return &ConfigBundle{
		SchemaVersions:     slices.Clone(schemaVersions),
		FieldPolicies:      cloneFieldPolicies(fieldPolicies),
		RegulatoryProfiles: cloneProfileAssignments(regulatoryProfiles),
		CadenceRules:       cloneCadenceRules(cadenceRules),
		RuleSeverities:     cloneRuleSeverityAssignments(ruleSeverities),
		DefaultRoleNames:   slices.Clone(defaultRoleNames),
	}
}

// Publish finalizes a composed bundle (design handoff B7's "publish with
// confirmation"), assigning it an ID and publish metadata. It returns a
// new ConfigBundle rather than mutating composed in place, matching
// office/schemaversions.Publish's own "construct, don't mutate" shape.
func Publish(composed *ConfigBundle, label, publishedBy string) (*ConfigBundle, error) {
	if composed == nil {
		return nil, errors.New("configbundle: cannot publish a nil composed bundle")
	}
	publishedBy = strings.TrimSpace(publishedBy)
	if publishedBy == "" {
		return nil, errors.New("configbundle: publishedBy is required")
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("configbundle: generate id: %w", err)
	}
	// Deep-copy again rather than aliasing composed's slices/maps: a
	// bundle composer UI plausibly lets the user keep tweaking the
	// preview after Compose but before confirming Publish, and once
	// published this bundle must never change again regardless of what
	// happens to the composed value the caller is still holding.
	return &ConfigBundle{
		ID:                 id.String(),
		Label:              strings.TrimSpace(label),
		SchemaVersions:     slices.Clone(composed.SchemaVersions),
		FieldPolicies:      cloneFieldPolicies(composed.FieldPolicies),
		RegulatoryProfiles: cloneProfileAssignments(composed.RegulatoryProfiles),
		CadenceRules:       cloneCadenceRules(composed.CadenceRules),
		RuleSeverities:     cloneRuleSeverityAssignments(composed.RuleSeverities),
		DefaultRoleNames:   slices.Clone(composed.DefaultRoleNames),
		PublishedAt:        time.Now().UTC(),
		PublishedBy:        publishedBy,
	}, nil
}

// ResolveFor produces the vessel-specific, fully-resolved wire form of
// this bundle (codebase audit 2026-07-22 §1): it runs the same
// fleet/group/vessel scope resolution the office already uses for its own
// dashboards — fieldpolicy.EffectiveFieldPolicy and
// compliance.EffectiveProfiles/EffectiveCadence/EffectiveSeverities — and
// flattens the result into a configwire.Bundle a vessel can apply with no
// scope logic of its own. versionNo is the bundle's pull-cursor value
// (office/store.GetConfigBundleCursor), carried through so the vessel can
// report which bundle version it is running (§3.3) and compare it against
// what the office assigned.
//
// Resolving office-side rather than shipping every scope's assignments and
// resolving aboard is the deliberate design choice (confirmed 2026-07-23):
// it wires the previously-dead Effective* resolvers into the pull path,
// keeps the wire document trivial, and never ships one vessel's config to
// another. A vessel with no group tags and no per-vessel overrides simply
// gets the fleet-scope (or built-in default) resolution.
func (b *ConfigBundle) ResolveFor(vesselID string, vesselGroups []string, versionNo int64) *configwire.Bundle {
	profiles := compliance.EffectiveProfiles(b.RegulatoryProfiles, vesselID, vesselGroups)
	profileStrs := make([]string, len(profiles))
	for i, p := range profiles {
		profileStrs[i] = string(p)
	}

	cadence := compliance.EffectiveCadence(b.CadenceRules, vesselID, vesselGroups)

	sevs := compliance.EffectiveSeverities(b.RuleSeverities, vesselID, vesselGroups)
	sevStrs := make(map[string]string, len(sevs))
	for id, s := range sevs {
		sevStrs[id] = string(s)
	}

	schemas := make([]configwire.SchemaConfig, 0, len(b.SchemaVersions))
	for _, sv := range b.SchemaVersions {
		eff := fieldpolicy.EffectiveFieldPolicy(b.FieldPolicies, vesselID, vesselGroups, sv.SchemaName, sv.Version)
		policy := make(map[string]string, len(eff.Policy))
		for f, st := range eff.Policy {
			policy[f] = string(st)
		}
		prefill := make(map[string]string, len(eff.Prefill))
		for f, c := range eff.Prefill {
			prefill[f] = string(c)
		}
		events := make(map[string][]string, len(eff.Events))
		maps.Copy(events, eff.Events)
		schemas = append(schemas, configwire.SchemaConfig{
			SchemaName: sv.SchemaName,
			Version:    sv.Version,
			Policy:     policy,
			Prefill:    prefill,
			Events:     events,
		})
	}

	return &configwire.Bundle{
		WireVersion:        configwire.WireVersion,
		BundleID:           b.ID,
		VersionNo:          versionNo,
		PublishedAt:        b.PublishedAt,
		Schemas:            schemas,
		RegulatoryProfiles: profileStrs,
		MaxGapHours:        cadence.MaxGapHours,
		RuleSeverities:     sevStrs,
		DefaultRoleNames:   slices.Clone(b.DefaultRoleNames),
	}
}

func cloneFieldPolicies(list []*fieldpolicy.SchemaFieldPolicy) []*fieldpolicy.SchemaFieldPolicy {
	if list == nil {
		return nil
	}
	out := make([]*fieldpolicy.SchemaFieldPolicy, len(list))
	for i, p := range list {
		cp := *p
		cp.Policy = maps.Clone(p.Policy)
		cp.Prefill = maps.Clone(p.Prefill)
		out[i] = &cp
	}
	return out
}

func cloneProfileAssignments(list []*compliance.ProfileAssignment) []*compliance.ProfileAssignment {
	if list == nil {
		return nil
	}
	out := make([]*compliance.ProfileAssignment, len(list))
	for i, a := range list {
		cp := *a
		cp.Profiles = slices.Clone(a.Profiles)
		out[i] = &cp
	}
	return out
}

func cloneCadenceRules(list []*compliance.CadenceRule) []*compliance.CadenceRule {
	if list == nil {
		return nil
	}
	out := make([]*compliance.CadenceRule, len(list))
	for i, r := range list {
		cp := *r
		out[i] = &cp
	}
	return out
}

func cloneRuleSeverityAssignments(list []*compliance.RuleSeverityAssignment) []*compliance.RuleSeverityAssignment {
	if list == nil {
		return nil
	}
	out := make([]*compliance.RuleSeverityAssignment, len(list))
	for i, a := range list {
		cp := *a
		cp.Severities = maps.Clone(a.Severities)
		out[i] = &cp
	}
	return out
}
