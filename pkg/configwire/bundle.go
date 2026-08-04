// SPDX-License-Identifier: AGPL-3.0-only

// Package configwire defines the frozen, versioned JSON wire format for a
// config bundle's *content* as it travels from office to vessel inside
// syncv1.ConfigBundle.content_json (and inside a DR restore bundle).
//
// It exists because the wire format used to be a bare json.Marshal of the
// office-internal office/configbundle.ConfigBundle struct, which carries
// no JSON tags — the field names on the wire were whatever the Go
// identifiers happened to be, unversioned and undocumented, so renaming an
// office-side Go field would silently break every vessel (codebase audit
// 2026-07-22 §1). This package is the explicit, tagged, versioned contract
// both sides now agree on instead.
//
// The document is *already resolved for one vessel*: the office runs the
// fleet/group/vessel scope resolution (fieldpolicy.EffectiveFieldPolicy,
// compliance.EffectiveProfiles/EffectiveCadence/EffectiveSeverities)
// before marshalling, so the vessel receives exactly its own effective
// field policy, prefill classes, enabled profiles, cadence and severity
// overrides with no scope logic to run aboard. See
// office/configbundle.(*ConfigBundle).ResolveFor for the producing side.
//
// Living under pkg/ (not office/) is deliberate: it is the one config
// type both office and vessel import, and it depends on nothing office-
// or vessel-specific — only primitives, so a wire-schema change is a
// single-file, single-package decision.
package configwire

import (
	"encoding/json"
	"fmt"
	"time"
)

// WireVersion is the schema version of the Bundle JSON document, carried
// on the wire as the wireVersion field. Bump it on any breaking change to
// the shape (a removed/renamed field, or a changed meaning); additive,
// optional fields do not require a bump. Decode rejects a document whose
// wireVersion is newer than this build understands.
const WireVersion = 1

// Bundle is one config bundle's content, resolved for a single vessel and
// ready to apply. All policy/prefill/profile/severity values are plain
// strings matching the pkg/validation and office/fieldpolicy vocabularies
// (e.g. "hidden"/"companyMandatory" for policy, "carryForward"/"ghost"
// for prefill, "error"/"warning"/"info" for severity) — kept as strings
// rather than typed enums so the wire contract does not shift when an
// internal Go type is refactored.
type Bundle struct {
	WireVersion int       `json:"wireVersion"`
	BundleID    string    `json:"bundleId"`
	VersionNo   int64     `json:"versionNo"`
	PublishedAt time.Time `json:"publishedAt"`

	// Schemas carries one entry per schema version referenced by the
	// bundle, with this vessel's resolved field policy and prefill classes.
	Schemas []SchemaConfig `json:"schemas"`

	// RegulatoryProfiles is the set of profiles enabled for this vessel
	// (EffectiveProfiles' union across every covering scope). An empty
	// slice means no regulatory profile applies — a real, deliberate
	// company choice, not "all" (the pre-config-bundle stand-in that
	// enabled every profile unconditionally is what this replaces).
	RegulatoryProfiles []string `json:"regulatoryProfiles"`

	// MaxGapHours is the resolved cadence ceiling (EffectiveCadence),
	// always populated (architecture 6.3's 12h default when nothing is
	// assigned).
	MaxGapHours float64 `json:"maxGapHours"`

	// RuleSeverities overrides the default severity of a plausibility/
	// continuity rule by rule ID (EffectiveSeverities). Absent rule IDs
	// keep their built-in default. Empty when nothing is overridden.
	RuleSeverities map[string]string `json:"ruleSeverities"`

	// DefaultRoleNames is architecture 9.2's default role set (inert data,
	// see office/configbundle.ConfigBundle.DefaultRoleNames).
	DefaultRoleNames []string `json:"defaultRoleNames,omitempty"`
}

// SchemaConfig is one schema version's resolved field policy and prefill
// classes for this vessel. Policy maps a field name to its policy state;
// a field absent from Policy takes its default (optional, or
// schemaMandatory if the schema itself says so). Prefill maps a field
// name to its prefill class; the actual prefilled *value* is computed by
// the vessel from its own report history at form-load, not carried here.
type SchemaConfig struct {
	SchemaName string            `json:"schemaName"`
	Version    string            `json:"version"`
	Policy     map[string]string `json:"policy"`
	Prefill    map[string]string `json:"prefill,omitempty"`

	// Events narrows which voyage event types each field's Policy entry
	// applies to (validation.FieldEvents): a field listed here is hidden on
	// any report whose event type is not in its list, and a field absent
	// from Events applies to every event. Added 2026-07-28 as an additive,
	// optional field, which per this package's own WireVersion contract
	// needs no version bump: an older vessel decoding a newer bundle simply
	// ignores it and keeps today's event-agnostic behavior.
	Events map[string][]string `json:"events,omitempty"`
}

// SchemaConfigFor returns the config for schemaName, or nil if the bundle
// carries none for it (a schema the company has published no policy for —
// every field then takes its default, exactly as if no bundle existed for
// that schema).
func (b *Bundle) SchemaConfigFor(schemaName string) *SchemaConfig {
	for i := range b.Schemas {
		if b.Schemas[i].SchemaName == schemaName {
			return &b.Schemas[i]
		}
	}
	return nil
}

// Decode parses and version-checks a wire bundle document. It returns an
// error for malformed JSON or a wireVersion this build cannot understand
// (newer than WireVersion, or the zero value that a pre-configwire raw
// bundle would decode to) — callers treat that error as "no usable bundle"
// and fall back to their built-in defaults rather than crashing, so an
// older vessel meeting a newer office (or a stale pre-format bundle still
// in local storage) degrades gracefully instead of failing a form load.
func Decode(data []byte) (*Bundle, error) {
	var b Bundle
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("configwire: decode bundle: %w", err)
	}
	if b.WireVersion < 1 || b.WireVersion > WireVersion {
		return nil, fmt.Errorf("configwire: unsupported wire version %d (this build understands 1..%d)", b.WireVersion, WireVersion)
	}
	return &b, nil
}
