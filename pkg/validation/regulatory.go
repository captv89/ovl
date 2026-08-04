// SPDX-License-Identifier: AGPL-3.0-only

package validation

import (
	"strings"

	"github.com/captv89/ovl/pkg/schema"
)

// RegulatoryProfile is one of the independent regulatory-reporting toggles
// architecture 6.2 lists a vessel or group can enable. MRV, ETS and FuelEU
// Maritime are bundled into one profile here rather than three: they reuse
// the same underlying fuel-consumption/CO2 data collected under the EU MRV
// regulation, and architecture 6.2 itself lists them comma-joined as a
// single toggle ("MRV/ETS/FuelEU"), unlike DCS and CII correction factors
// which each get their own slot in that same list.
type RegulatoryProfile string

const (
	ProfileMRV                RegulatoryProfile = "mrv" // bundles MRV, ETS, FuelEU Maritime
	ProfileDCS                RegulatoryProfile = "dcs"
	ProfileCII                RegulatoryProfile = "cii"
	ProfileVoyageVerification RegulatoryProfile = "voyageVerification"
)

// AllRegulatoryProfiles lists every known profile, in the order architecture
// 6.2 names them. Config-bundle-driven per-vessel enablement (6.5) is Phase
// 3 work; until then callers enable all of them, same stand-in pattern as
// FieldPolicy.
var AllRegulatoryProfiles = []RegulatoryProfile{ProfileMRV, ProfileDCS, ProfileCII, ProfileVoyageVerification}

// relevanceRequirement is how strongly one curated field's relevance ties it
// to a profile.
type relevanceRequirement int

const (
	requirementNone          relevanceRequirement = iota
	requirementInformational                      // relevant, but not counted as "missing" if empty
	requirementNeeded                             // counts toward the profile's missing-field list if empty
)

// profileRelevance classifies one curated schema field's exact `relevance`
// string (schemas/ovd-3.13/log-abstract.json, sourced from the OVD interface
// description xlsx's own relevance column per architecture 6.2) into
// per-profile requirement levels.
//
// The xlsx's relevance column is free text, not structured per-profile data,
// but its actual vocabulary across all 409 Log Abstract fields turned out to
// be a small, fixed set of distinct strings (verified by inspection, not
// assumed). Enumerated exactly rather than fuzzy-matched, so a future OVD
// release introducing new relevance wording fails loudly (falls through to
// "no profile") instead of being silently misclassified.
//
// "DSC only, voluntary wrt MRV" is treated as DCS, not a fifth "DSC"
// profile — almost certainly a typo for DCS in the source xlsx, since DCS is
// the one regulatory profile architecture 6.2 names with a similar acronym,
// and "DSC" (Digital Selective Calling) is an unrelated maritime-radio term
// with no reporting-profile meaning here.
func profileRelevance(relevance string) map[RegulatoryProfile]relevanceRequirement {
	switch strings.TrimSpace(relevance) {
	case "mandatory for MRV&DCS", "recommended for MRV&DCS":
		return map[RegulatoryProfile]relevanceRequirement{ProfileMRV: requirementNeeded, ProfileDCS: requirementNeeded}
	case "mandatory for MRV":
		return map[RegulatoryProfile]relevanceRequirement{ProfileMRV: requirementNeeded}
	case "voluntary wrt MRV":
		return map[RegulatoryProfile]relevanceRequirement{ProfileMRV: requirementInformational}
	case "for CII correction, voluntary wrt MRV":
		return map[RegulatoryProfile]relevanceRequirement{ProfileCII: requirementInformational, ProfileMRV: requirementInformational}
	case "for CII correction":
		return map[RegulatoryProfile]relevanceRequirement{ProfileCII: requirementInformational}
	case "DSC only, voluntary wrt MRV":
		return map[RegulatoryProfile]relevanceRequirement{ProfileDCS: requirementNeeded, ProfileMRV: requirementInformational}
	case "mandatory for FEUM and in case of no fuel consumption for any verification":
		return map[RegulatoryProfile]relevanceRequirement{ProfileMRV: requirementNeeded}
	case "recommended for voyage level verfication schemes":
		return map[RegulatoryProfile]relevanceRequirement{ProfileVoyageVerification: requirementNeeded}
	default: // "optional input", "not in use", "out of scope for GHG verification", anything else
		return nil
	}
}

// GHGRelevant reports whether a curated field's relevance ties it to any
// regulatory profile at all (mandatory, recommended, or merely informational/
// voluntary) — used by FieldPolicy.StateFor to decide an unlisted field's
// default visibility, distinct from EvaluateRegulatoryReadiness's stricter
// requirementNeeded-only test for what counts as "missing."
func GHGRelevant(relevance string) bool {
	return len(profileRelevance(relevance)) > 0
}

// ProfileReadiness is one enabled regulatory profile's completeness against
// a specific report: which of its required fields are still empty.
type ProfileReadiness struct {
	Profile       RegulatoryProfile
	MissingFields []string // schema field names; empty (not nil) when ready
}

// Ready reports whether every field the profile needs is filled in.
func (p ProfileReadiness) Ready() bool {
	return len(p.MissingFields) == 0
}

// EvaluateRegulatoryReadiness computes, for each enabled profile, which of
// its schema-relevant fields are still empty on r — architecture 6.2's
// "compliance badges in the health check" (design handoff A6 item 3: "MRV
// ✓", "DCS ✓", "Voyage verification: 3 fields missing"). Only fields
// classified requirementNeeded count toward MissingFields;
// requirementInformational fields are associated with the profile but don't
// block its readiness (e.g. a CII correction factor that's "voluntary wrt
// MRV"). Profiles with no relevant fields on the schema at all are omitted
// from the result rather than reported as trivially ready.
//
// policy/events gate out fields the company has hidden from the crew
// (outright, or for r's specific EventType) exactly like EvaluateFieldRules
// does (fieldrules.go): a hidden field is not "missing," it doesn't exist
// for this report at all, so it's skipped before it can affect either
// seenByProfile or MissingFields. Both may be nil (no field policy
// configured), matching FieldPolicy.StateForEvent's own nil-map behavior.
func EvaluateRegulatoryReadiness(r *Report, fields []schema.Field, enabled []RegulatoryProfile, policy FieldPolicy, events FieldEvents) []ProfileReadiness {
	enabledSet := make(map[RegulatoryProfile]bool, len(enabled))
	for _, p := range enabled {
		enabledSet[p] = true
	}
	missingByProfile := map[RegulatoryProfile][]string{}
	seenByProfile := map[RegulatoryProfile]bool{}
	for _, f := range fields {
		if policy.StateForEvent(f.Name, f.SchemaMandatory, f.Relevance, events, r.EventType) == FieldHidden {
			continue
		}
		for profile, req := range profileRelevance(f.Relevance) {
			if req == requirementNone || !enabledSet[profile] {
				continue
			}
			seenByProfile[profile] = true
			if req == requirementNeeded && r.IsEmpty(f.Name) {
				missingByProfile[profile] = append(missingByProfile[profile], f.Name)
			}
		}
	}
	var out []ProfileReadiness
	for _, p := range enabled {
		if !seenByProfile[p] {
			continue
		}
		out = append(out, ProfileReadiness{Profile: p, MissingFields: missingByProfile[p]})
	}
	return out
}
