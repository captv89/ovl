// SPDX-License-Identifier: AGPL-3.0-only

// Package fieldpolicy models the office side of company field-policy
// authoring (architecture 6.1 field policy, 6.4 prefill classes; design
// handoff B6, the field policy editor). It builds directly on
// pkg/validation's FieldPolicy/FieldPolicyState — the same type the
// vessel-side rule engine already consumes — rather than re-declaring
// the five-state vocabulary here, so office authoring and vessel
// evaluation can never drift apart on what the states mean.
//
// There is no schema-version registry yet (design handoff B5's upload/
// download/diff workflow is the next Phase 3 checklist item after this
// one) — every curated schema currently has exactly one published
// version, so authoring here identifies a schema purely by
// (schemaName, schemaVersion) strings, matching pkg/schema.Schema's own
// identity fields, with no separate version-registry row to join
// against.
package fieldpolicy

import (
	"errors"
	"fmt"
	"time"

	"github.com/captv89/ovl/office/compliance"
	"github.com/captv89/ovl/pkg/validation"
)

// PrefillClass is the company-assigned prefill treatment for a field
// (architecture 6.4). The curated schema JSON carries no prefillClass
// property of its own (confirmed during the Phase 2 form-engine work),
// so this is office-authored config-bundle data, not schema data, and lives here
// rather than in pkg/schema.
type PrefillClass string

const (
	PrefillNone         PrefillClass = "none"
	PrefillCarryForward PrefillClass = "carryForward"
	PrefillComputed     PrefillClass = "computed"
	PrefillGhost        PrefillClass = "ghost"
)

// validPrefillClasses is used to reject typos/unknown values at the
// authoring boundary rather than storing them silently.
var validPrefillClasses = map[PrefillClass]bool{
	PrefillNone:         true,
	PrefillCarryForward: true,
	PrefillComputed:     true,
	PrefillGhost:        true,
}

// SchemaFieldPolicy is one schema version's full set of company field
// policy and prefill overrides, at one scope: fleet-wide, one vessel
// group, or one vessel (architecture 6.5's "field policies per schema"
// is part of the config bundle, assignable with the same fleet/group/
// vessel shape as regulatory profiles/cadence rules/rule severities —
// see office/compliance.Scope's own doc comment). This is what lets a
// company give DP vessels a policy that shows DP-specific fields while
// a container or bulk vessel group's policy keeps them hidden, rather
// than every vessel in the fleet being stuck on one global policy.
//
// A field absent from Policy defaults to validation.FieldOptional (or
// FieldSchemaMandatory, if the schema says so — see
// FieldPolicy.StateFor); a field absent from Prefill defaults to
// PrefillNone. Only overrides are stored, mirroring
// validation.FieldPolicy's own "absent = default" shape.
// Events narrows which voyage event types each field's Policy entry
// applies to (validation.FieldEvents); a field absent from Events applies
// to every event, so an assignment authored before this existed keeps
// behaving exactly as it did. Events is deliberately part of the same
// per-field rule as Policy — see EffectiveFieldPolicy on why the two never
// resolve from different scopes.
type SchemaFieldPolicy struct {
	Scope         compliance.Scope
	SchemaName    string
	SchemaVersion string
	Policy        validation.FieldPolicy
	Events        validation.FieldEvents
	Prefill       map[string]PrefillClass
	UpdatedAt     time.Time
}

// New returns an empty SchemaFieldPolicy for one scope and schema
// version — every field starts at its default state until
// SetPolicy/SetPrefillClass override it. Returns an error for an
// invalid scope, matching compliance.NewCadenceRule/
// NewRuleSeverityAssignment's own validate-at-construction shape.
func New(scope compliance.Scope, schemaName, schemaVersion string) (*SchemaFieldPolicy, error) {
	if err := scope.Validate(); err != nil {
		return nil, fmt.Errorf("fieldpolicy: %w", err)
	}
	return newUnchecked(scope, schemaName, schemaVersion), nil
}

// newUnchecked skips scope validation for callers building a
// SchemaFieldPolicy from a scope that is already known-valid (carried
// forward from another SchemaFieldPolicy, or resolved internally by
// EffectiveFieldPolicy) — Migrate and EffectiveFieldPolicy use this
// rather than propagating an error that can never actually occur.
func newUnchecked(scope compliance.Scope, schemaName, schemaVersion string) *SchemaFieldPolicy {
	return &SchemaFieldPolicy{
		Scope:         scope,
		SchemaName:    schemaName,
		SchemaVersion: schemaVersion,
		Policy:        validation.FieldPolicy{},
		Events:        validation.FieldEvents{},
		Prefill:       map[string]PrefillClass{},
	}
}

// ErrSchemaMandatoryImmutable is returned when a caller tries to set a
// policy state for a field the schema itself marks schemaMandatory, or
// tries to manually assign FieldSchemaMandatory to a field the schema
// does not. Architecture 6.1 calls schemaMandatory "Immutable, set by
// the OVD schema itself" — it is never a company choice in either
// direction.
var ErrSchemaMandatoryImmutable = errors.New("fieldpolicy: schemaMandatory is set by the schema and cannot be assigned by policy")

// SetPolicy assigns fieldName's policy state. schemaMandatory must
// reflect the field's own schema.Field.SchemaMandatory flag, so this can
// enforce architecture 6.1's immutability rule instead of trusting the
// caller to have checked it already.
func (p *SchemaFieldPolicy) SetPolicy(fieldName string, state validation.FieldPolicyState, schemaMandatory bool) error {
	if schemaMandatory || state == validation.FieldSchemaMandatory {
		return ErrSchemaMandatoryImmutable
	}
	switch state {
	case validation.FieldHidden, validation.FieldOptional, validation.FieldRecommended, validation.FieldCompanyMandatory:
		p.Policy[fieldName] = state
		return nil
	default:
		return fmt.Errorf("fieldpolicy: unknown policy state %q", state)
	}
}

// SetEvents narrows fieldName's policy entry to the given voyage event
// types — the office authoring side of validation.FieldEvents. An empty
// list, or one containing validation.AllEvents, removes the entry entirely
// (every event is the default), keeping the map limited to real narrowings
// exactly as SetPrefillClass does for PrefillNone.
//
// validEvents is the curated event-type vocabulary the schema's Event field
// draws from; pass nil to skip the check (schemas with no event concept).
// Rejecting unknown codes here rather than storing them silently matches
// SetPolicy/SetPrefillClass, and matters more for events than for the other
// two: a typo'd event code would not fail loudly, it would quietly hide the
// field on every real report.
func (p *SchemaFieldPolicy) SetEvents(fieldName string, events []string, validEvents map[string]bool) error {
	for _, ev := range events {
		if ev == validation.AllEvents {
			delete(p.Events, fieldName)
			return nil
		}
		if validEvents != nil && !validEvents[ev] {
			return fmt.Errorf("fieldpolicy: unknown voyage event type %q", ev)
		}
	}
	if len(events) == 0 {
		delete(p.Events, fieldName)
		return nil
	}
	p.Events[fieldName] = events
	return nil
}

// EventsFor returns the voyage event types fieldName's policy applies to,
// or nil when it applies to every event.
func (p *SchemaFieldPolicy) EventsFor(fieldName string) []string {
	return p.Events[fieldName]
}

// SetPrefillClass assigns fieldName's prefill class. Setting
// PrefillNone removes the field from Prefill entirely (it is the
// default), keeping the map limited to real overrides.
func (p *SchemaFieldPolicy) SetPrefillClass(fieldName string, class PrefillClass) error {
	if !validPrefillClasses[class] {
		return fmt.Errorf("fieldpolicy: unknown prefill class %q", class)
	}
	if class == PrefillNone {
		delete(p.Prefill, fieldName)
		return nil
	}
	p.Prefill[fieldName] = class
	return nil
}

// PrefillFor returns fieldName's effective prefill class, defaulting to
// PrefillNone when no override is set.
func (p *SchemaFieldPolicy) PrefillFor(fieldName string) PrefillClass {
	if c, ok := p.Prefill[fieldName]; ok {
		return c
	}
	return PrefillNone
}

// EffectiveFieldPolicy resolves the field policy/prefill overrides that
// apply to one vessel for one (schemaName, schemaVersion), from whatever
// mix of fleet/group/vessel assignments exist — the office/compliance.
// EffectiveSeverities pattern applied to field policy: resolution is
// per field name, not per whole assignment, so a vessel can inherit one
// field's override from the fleet while overriding a different field
// itself via its own group.
//
// Precedence per field, most to least specific: a vessel-scoped
// assignment's own entry for that field wins outright if present;
// otherwise, among every group-scoped assignment covering one of the
// vessel's groups, the strictest entry wins (see policyStateRank —
// unlike EffectiveSeverities' error/warning/info axis, there is no
// single natural "more severe" ordering for a five-state field policy,
// but picking the option that shows/requires more data is the safer
// default when two groups disagree, since it never hides something a
// company elsewhere considered necessary); otherwise the fleet-wide
// assignment's entry, if any; otherwise the field is absent from the
// result entirely (StateFor's own default applies, exactly as if no
// override existed at all).
//
// This is what makes it possible to give one vessel group (e.g. DP
// vessels) a policy that shows DP-specific fields while another group
// (e.g. container or bulk carriers) keeps them hidden: the fleet-wide
// assignment hides the field by default, and only the DP group's
// assignment sets an override for it.
func EffectiveFieldPolicy(assignments []*SchemaFieldPolicy, vesselID string, vesselGroups []string, schemaName, schemaVersion string) *SchemaFieldPolicy {
	var vesselA, fleetA *SchemaFieldPolicy
	var groupAs []*SchemaFieldPolicy
	for _, a := range assignments {
		if a.SchemaName != schemaName || a.SchemaVersion != schemaVersion {
			continue
		}
		switch a.Scope.Type {
		case compliance.ScopeVessel:
			if a.Scope.Key == vesselID {
				vesselA = a
			}
		case compliance.ScopeFleet:
			fleetA = a
		case compliance.ScopeGroup:
			if a.Scope.CoversVessel(vesselID, vesselGroups) {
				groupAs = append(groupAs, a)
			}
		}
	}

	fieldNames := map[string]bool{}
	prefillNames := map[string]bool{}
	for _, a := range allOf(vesselA, fleetA, groupAs) {
		for name := range a.Policy {
			fieldNames[name] = true
		}
		// An event narrowing with no accompanying state override is a real
		// authoring outcome ("leave this field at its default state, but only
		// on Arrival"), so Events contributes field names of its own rather
		// than only riding along with Policy.
		for name := range a.Events {
			fieldNames[name] = true
		}
		for name := range a.Prefill {
			prefillNames[name] = true
		}
	}

	// The resolved result is "this vessel's effective view," not itself
	// a stored assignment at some scope, but Scope is carried as a
	// vessel-scoped value (rather than left zero) so a caller logging or
	// comparing the result can still see which vessel it was resolved
	// for.
	result := newUnchecked(compliance.Scope{Type: compliance.ScopeVessel, Key: vesselID}, schemaName, schemaVersion)
	for name := range fieldNames {
		if a, ok := winningRule(vesselA, fleetA, groupAs, name); ok {
			result.takeRule(a, name)
		}
	}
	for name := range prefillNames {
		if vesselA != nil {
			if c, ok := vesselA.Prefill[name]; ok {
				result.Prefill[name] = c
				continue
			}
		}
		if c, ok := firstGroupPrefill(groupAs, name); ok {
			result.Prefill[name] = c
			continue
		}
		if fleetA != nil {
			if c, ok := fleetA.Prefill[name]; ok {
				result.Prefill[name] = c
			}
		}
	}
	return result
}

func allOf(vesselA, fleetA *SchemaFieldPolicy, groupAs []*SchemaFieldPolicy) []*SchemaFieldPolicy {
	out := make([]*SchemaFieldPolicy, 0, len(groupAs)+2)
	if vesselA != nil {
		out = append(out, vesselA)
	}
	if fleetA != nil {
		out = append(out, fleetA)
	}
	out = append(out, groupAs...)
	return out
}

// policyStateRank orders FieldPolicyState from least to most "shows/
// requires data," for picking the strictest of several conflicting
// group assignments in EffectiveFieldPolicy. schemaMandatory never
// appears as a stored override (SetPolicy rejects it), so it has no
// rank here.
func policyStateRank(s validation.FieldPolicyState) int {
	switch s {
	case validation.FieldHidden:
		return 0
	case validation.FieldOptional:
		return 1
	case validation.FieldRecommended:
		return 2
	case validation.FieldCompanyMandatory:
		return 3
	default:
		return 1 // unknown value: treat as optional rather than favoring or penalizing it
	}
}

// hasRule reports whether a carries any per-field rule for fieldName — a
// policy state, an event narrowing, or both. The two are one rule authored
// on one editor row, so they are resolved as a unit (see winningRule).
func hasRule(a *SchemaFieldPolicy, fieldName string) bool {
	if a == nil {
		return false
	}
	if _, ok := a.Policy[fieldName]; ok {
		return true
	}
	_, ok := a.Events[fieldName]
	return ok
}

// winningRule picks the single assignment whose rule for fieldName applies to
// this vessel: the vessel's own if it has one, else the strictest covering
// group's, else the fleet's.
//
// Returning the whole assignment rather than a bare state is what keeps a
// field's policy state and its event list from ever resolving out of separate
// scopes. A vessel-scoped row saying "hidden" must not silently inherit the
// fleet row's "[Arrival, Departure]" narrowing — the vessel authored a
// complete replacement rule for that field, not a partial patch of one.
func winningRule(vesselA, fleetA *SchemaFieldPolicy, groupAs []*SchemaFieldPolicy, fieldName string) (*SchemaFieldPolicy, bool) {
	if hasRule(vesselA, fieldName) {
		return vesselA, true
	}
	if a, ok := strictestGroupRule(groupAs, fieldName); ok {
		return a, true
	}
	if hasRule(fleetA, fieldName) {
		return fleetA, true
	}
	return nil, false
}

// takeRule copies fieldName's whole rule — state and event narrowing — from
// the winning assignment into the resolved result, omitting either half the
// winner did not set.
func (p *SchemaFieldPolicy) takeRule(from *SchemaFieldPolicy, fieldName string) {
	if st, ok := from.Policy[fieldName]; ok {
		p.Policy[fieldName] = st
	}
	if events, ok := from.Events[fieldName]; ok {
		p.Events[fieldName] = events
	}
}

// strictestGroupRule returns the covering group assignment whose rule for
// fieldName wins: the strictest policy state (highest policyStateRank), since
// among disagreeing groups the option that shows or requires more data is the
// safer default. A group that narrows events without setting a state ranks
// below any group that sets one, and only wins when no covering group does.
func strictestGroupRule(groupAs []*SchemaFieldPolicy, fieldName string) (*SchemaFieldPolicy, bool) {
	var winner *SchemaFieldPolicy
	best := 0
	for _, a := range groupAs {
		if !hasRule(a, fieldName) {
			continue
		}
		rank := -1 // events-only rule: no state to rank, loses to any real state
		if st, ok := a.Policy[fieldName]; ok {
			rank = policyStateRank(st)
		}
		if winner == nil || rank > best {
			winner, best = a, rank
		}
	}
	return winner, winner != nil
}

// firstGroupPrefill returns the first covering group assignment's
// prefill class for fieldName. Prefill classes have no severity-like
// ordering to break ties with (unlike policy state), so among multiple
// conflicting groups this deterministically picks by assignment order
// rather than attempting to rank carryForward/computed/ghost against
// each other.
func firstGroupPrefill(groupAs []*SchemaFieldPolicy, fieldName string) (PrefillClass, bool) {
	for _, a := range groupAs {
		if c, ok := a.Prefill[fieldName]; ok {
			return c, true
		}
	}
	return "", false
}
