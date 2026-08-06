// SPDX-License-Identifier: AGPL-3.0-only

package validation

// FieldPolicyState is the company-assigned state for a field, one of the
// five states in architecture 6.1. Config authoring (the office field
// policy editor) is Phase 3 work; this type is what that config bundle
// will eventually carry per field.
type FieldPolicyState string

const (
	FieldHidden           FieldPolicyState = "hidden"
	FieldOptional         FieldPolicyState = "optional"
	FieldRecommended      FieldPolicyState = "recommended"
	FieldCompanyMandatory FieldPolicyState = "companyMandatory"
	FieldSchemaMandatory  FieldPolicyState = "schemaMandatory"
)

// FieldPolicy maps field name to its company-assigned state. A field
// absent from the map defaults to FieldRecommended when its curated
// relevance ties it to a real regulatory profile (see profileRelevance in
// regulatory.go) — a GHG-relevant field shouldn't quietly collapse behind
// "optional" just because no office has explicitly configured it yet — and
// to FieldOptional otherwise. Fields the schema itself marks
// schemaMandatory are always FieldSchemaMandatory regardless of what the
// policy says (schema mandatoriness is immutable, architecture 6.1).
type FieldPolicy map[string]FieldPolicyState

// StateFor returns the effective policy state for a field, given the
// schema's own schemaMandatory flag and curated relevance string (pass ""
// if unknown/not applicable — it only affects the unlisted-field default).
//
// This is the event-agnostic answer. Callers that know which voyage event
// the report is for must use StateForEvent instead, so a field the company
// scoped to specific events does not leak onto reports it was never meant
// for.
func (p FieldPolicy) StateFor(fieldName string, schemaMandatory bool, relevance string) FieldPolicyState {
	if schemaMandatory {
		return FieldSchemaMandatory
	}
	if s, ok := p[fieldName]; ok {
		return s
	}
	if GHGRelevant(relevance) {
		return FieldRecommended
	}
	return FieldOptional
}

// AllEvents is the wildcard event code meaning "this field's policy applies
// to every voyage event type". It matches the curated schema's own
// appliesToEvents wildcard (schemas/meta-schema.json), so the two vocabularies
// read the same way.
const AllEvents = "*"

// FieldEvents maps a field name to the voyage event types its policy entry
// applies to (architecture 6.1, extended 2026-07-28). It exists because a
// single company policy state per field is not enough in practice: the number
// of tugs used is mandatory on an Arrival or Departure and meaningless on a
// Noon at sea, and before this the only way to express that was to show the
// field on every report or none.
//
// A field absent from the map — and an entry that is empty or contains
// AllEvents — applies to every event, which is why adding this to an existing
// policy changes nothing until an event list is actually authored.
//
// One list per field, not one per (field, state): a field carries a single
// policy state that applies to the events listed here, and is hidden
// everywhere else. That deliberately cannot express "mandatory on Arrival,
// optional on Noon" — the user chose the simpler authoring model on
// 2026-07-28, on the reasoning that `recommended` already covers the
// situational case without blocking submit. Widening this to a
// list of rules per field later is an additive, optional wire field and would
// not need a configwire.WireVersion bump.
type FieldEvents map[string][]string

// AppliesTo reports whether fieldName's policy entry covers eventType.
//
// An empty eventType skips the gate entirely (returns true): schemas with no
// event concept of their own — bunker-report, edn-report, commercial-period,
// cargo-nomination — must never have their fields gated away by a policy
// authored for log-abstract's event vocabulary.
func (e FieldEvents) AppliesTo(fieldName, eventType string) bool {
	if eventType == "" {
		return true
	}
	events, ok := e[fieldName]
	if !ok || len(events) == 0 {
		return true
	}
	for _, ev := range events {
		if ev == AllEvents || ev == eventType {
			return true
		}
	}
	return false
}

// StateForEvent is StateFor with the company's per-event applicability gate
// applied: a field whose event list omits eventType resolves to FieldHidden,
// so it does not render, has no health check effect, and is never exported —
// exactly as if the company had hidden it outright (architecture 6.1's
// "field does not exist for the crew").
//
// schemaMandatory is checked first and is never gated. Architecture 6.1 calls
// schema mandatoriness immutable and "set by the OVD schema itself"; letting
// an event list suppress it would make a company config able to produce OVD
// output the standard itself rejects.
func (p FieldPolicy) StateForEvent(
	fieldName string, schemaMandatory bool, relevance string, events FieldEvents, eventType string,
) FieldPolicyState {
	if schemaMandatory {
		return FieldSchemaMandatory
	}
	if !events.AppliesTo(fieldName, eventType) {
		return FieldHidden
	}
	return p.StateFor(fieldName, schemaMandatory, relevance)
}
