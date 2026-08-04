// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"time"

	"github.com/captv89/ovl/internal/httpjson"
	"github.com/captv89/ovl/office/compliance"
	"github.com/captv89/ovl/office/fieldpolicy"
	"github.com/captv89/ovl/office/schemaversions"
	"github.com/captv89/ovl/office/store"
	"github.com/captv89/ovl/pkg/schema"
	"github.com/captv89/ovl/pkg/validation"
	ovlschemas "github.com/captv89/ovl/schemas"
)

// fieldPolicyMigrationView describes design handoff B6's migration
// assistant state when it applies: "carried-over policies shown
// normally, new fields grouped on top flagged 'review', removed fields
// listed for acknowledgment."
type fieldPolicyMigrationView struct {
	FromVersion   string   `json:"fromVersion"`
	NewFields     []string `json:"newFields"`
	RemovedFields []string `json:"removedFields"`
}

// fieldPolicyView is B6's whole-screen payload for one scope: the
// current schema version's fields plus that scope's policy/prefill
// overrides, and (only when relevant) a pending migration proposal.
type fieldPolicyView struct {
	SchemaName string            `json:"schemaName"`
	Version    string            `json:"version"`
	Scope      scopeView         `json:"scope"`
	Fields     []schemaFieldView `json:"fields"`
	Policy     map[string]string `json:"policy"`
	Prefill    map[string]string `json:"prefill"`
	// Events narrows which voyage event types each field's policy applies
	// to; a field absent from the map applies to every event, which is the
	// default the editor shows for every unconfigured row.
	Events map[string][]string `json:"events"`
	// EventTypes is the curated event-type vocabulary the editor's
	// applies-to-events control offers. Empty for a schema with no event
	// concept of its own (bunker-report, edn-report, commercial-period,
	// cargo-nomination) — the editor hides the control entirely for those
	// rather than offering a narrowing that could never match a report.
	EventTypes []string                  `json:"eventTypes"`
	Migration  *fieldPolicyMigrationView `json:"migration,omitempty"`
}

// eventVocabulary returns the curated voyage event-type codes a schema's
// field policy may be narrowed to, and a lookup for validating them on save.
// Both are nil for a schema with no event-types enum field — its reports
// carry no event type, so narrowing a field to one could only ever hide it.
//
// Detected from the curated schema's own enumRef rather than a hardcoded
// schema-name list, so a future curated schema that gains an event field
// picks this up without a code change here.
func eventVocabulary(parsed *schema.Schema) ([]string, map[string]bool, error) {
	eventful := false
	for _, f := range parsed.Fields {
		if f.EnumRef != nil && *f.EnumRef == eventTypesEnumRef {
			eventful = true
			break
		}
	}
	if !eventful {
		return nil, nil, nil
	}
	types, err := schema.LoadEventTypes(ovlschemas.FS, "ovd-3.13/enums/event-types.json")
	if err != nil {
		return nil, nil, fmt.Errorf("load event types: %w", err)
	}
	codes := make([]string, len(types))
	valid := make(map[string]bool, len(types))
	for i, t := range types {
		codes[i] = t.Code
		valid[t.Code] = true
	}
	return codes, valid, nil
}

// eventTypesEnumRef is the curated enumRef that marks a schema field as
// carrying a voyage event type (schemas/ovd-3.13/enums/event-types.json).
const eventTypesEnumRef = "event-types"

func stringifyEvents(e validation.FieldEvents) map[string][]string {
	out := make(map[string][]string, len(e))
	maps.Copy(out, e)
	return out
}

// fieldPolicyAssignmentView is the lightweight per-scope summary B7-
// style "current assignments" lists use (see RegulatoryProfilesPanel's
// equivalent) — one entry per scope that has ever been saved for a
// schema, across every schema version that scope was ever authored
// against.
type fieldPolicyAssignmentView struct {
	Scope     scopeView `json:"scope"`
	Version   string    `json:"version"`
	Policy    int       `json:"policyCount"`
	Prefill   int       `json:"prefillCount"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// scopeFromQuery parses ?scopeType=&scopeKey= into a compliance.Scope,
// defaulting to fleet-wide when scopeType is absent — this keeps a
// plain `GET /api/field-policies/{name}` (no scope params at all)
// behaving exactly as it did before field policy gained scope, matching
// what every existing caller already sends.
func scopeFromQuery(r *http.Request) (compliance.Scope, error) {
	t := r.URL.Query().Get("scopeType")
	if t == "" {
		return compliance.FleetScope(), nil
	}
	return scopeView{Type: t, Key: r.URL.Query().Get("scopeKey")}.toScope()
}

// handleGetFieldPolicy serves design handoff B6: the field table for a
// schema's latest published version plus the given scope's saved
// overrides (fleet-wide by default — see scopeFromQuery).
//
// Migration assistant: if this scope has zero saved overrides against
// the latest version and an older version exists, this proposes (but
// does not persist) this same scope's older-version policy carried
// forward via fieldpolicy.Migrate — exactly architecture 5.3/6.1's
// "carry forward policy onto the new version" flow design handoff B6
// calls the migration assistant. Once this scope has saved anything at
// all against the latest version (including an empty save), migration
// is no longer offered for it — the admin has already made an explicit
// choice for that scope and version.
func (s *Server) handleGetFieldPolicy(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	name := r.PathValue("name")
	scope, err := scopeFromQuery(r)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	latest, err := s.st.LatestSchemaVersion(r.Context(), name)
	if errors.Is(err, store.ErrNotFound) {
		httpjson.WriteError(w, http.StatusNotFound, "schema not found")
		return
	}
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	fieldsView, err := toSchemaDetailView(latest)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	saved, err := s.st.LoadFieldPolicy(r.Context(), scope, name, latest.Version)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	parsed, err := schema.Parse(latest.Content)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	eventTypes, _, err := eventVocabulary(parsed)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	view := fieldPolicyView{
		SchemaName: name, Version: latest.Version, Scope: toScopeView(scope), Fields: fieldsView.Fields,
		Policy: stringifyPolicy(saved.Policy), Prefill: stringifyPrefill(saved.Prefill),
		Events: stringifyEvents(saved.Events), EventTypes: eventTypes,
	}

	if len(saved.Policy) == 0 && len(saved.Prefill) == 0 && len(saved.Events) == 0 {
		history, err := s.st.ListSchemaVersions(r.Context(), name)
		if err != nil {
			httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if len(history) >= 2 {
			previous := history[1] // history is newest-first; history[0] == latest
			migrated, migration, err := s.buildMigration(r.Context(), scope, name, latest, previous)
			if err != nil {
				httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
				return
			}
			view.Policy = stringifyPolicy(migrated.Policy)
			view.Prefill = stringifyPrefill(migrated.Prefill)
			view.Events = stringifyEvents(migrated.Events)
			view.Migration = migration
		}
	}

	httpjson.WriteJSON(w, http.StatusOK, view)
}

// handleListFieldPolicyAssignments serves the "current assignments"
// summary across every scope that has ever saved a field policy for
// name, at any schema version — the field-policy equivalent of
// handleListProfileAssignments/handleListCadenceRules/
// handleListRuleSeverityAssignments. Unlike those three (whose bundle
// content is always "every scope, current state" with no version
// dimension), field policy is authored per schema *version* as well as
// per scope, so this lists across every version this schema has ever
// had rather than just the latest — a scope's most recent save is
// whatever it saved against whichever version was latest at the time.
func (s *Server) handleListFieldPolicyAssignments(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	name := r.PathValue("name")
	versions, err := s.st.ListSchemaVersions(r.Context(), name)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Non-nil so a schema with no saved assignments serialises as [] and
	// not JSON null — every sibling list handler (handleListProfile-
	// Assignments etc.) uses make() for the same reason, and the frontend
	// reads assignments.length directly.
	out := []fieldPolicyAssignmentView{}
	for _, v := range versions {
		assignments, err := s.st.ListFieldPolicyAssignments(r.Context(), name, v.Version)
		if err != nil {
			httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		for _, a := range assignments {
			out = append(out, fieldPolicyAssignmentView{
				Scope: toScopeView(a.Scope), Version: a.SchemaVersion,
				Policy: len(a.Policy), Prefill: len(a.Prefill), UpdatedAt: a.UpdatedAt,
			})
		}
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}

// buildMigration proposes carrying scope's saved field policy for
// previous forward onto latest, via fieldpolicy.Migrate — design
// handoff B6's migration assistant. Returns the proposed (unsaved)
// policy plus the view describing what changed.
func (s *Server) buildMigration(
	ctx context.Context, scope compliance.Scope, name string, latest, previous *schemaversions.SchemaVersion,
) (*fieldpolicy.SchemaFieldPolicy, *fieldPolicyMigrationView, error) {
	oldParsed, err := schema.Parse(previous.Content)
	if err != nil {
		return nil, nil, fmt.Errorf("parse previous schema version %s@%s: %w", name, previous.Version, err)
	}
	newParsed, err := schema.Parse(latest.Content)
	if err != nil {
		return nil, nil, fmt.Errorf("parse latest schema version %s@%s: %w", name, latest.Version, err)
	}
	oldPolicy, err := s.st.LoadFieldPolicy(ctx, scope, name, previous.Version)
	if err != nil {
		return nil, nil, fmt.Errorf("load field policy for %s/%s@%s: %w", scope.Type, name, previous.Version, err)
	}
	diff := schema.DiffSchemas(oldParsed, newParsed)
	result := fieldpolicy.Migrate(oldPolicy, latest.Version, diff)
	newFields, removedFields := result.NewFields, result.RemovedFields
	if newFields == nil {
		newFields = []string{}
	}
	if removedFields == nil {
		removedFields = []string{}
	}
	return result.NewPolicy, &fieldPolicyMigrationView{
		FromVersion: previous.Version, NewFields: newFields, RemovedFields: removedFields,
	}, nil
}

func stringifyPolicy(p validation.FieldPolicy) map[string]string {
	out := make(map[string]string, len(p))
	for k, v := range p {
		out[k] = string(v)
	}
	return out
}

func stringifyPrefill(p map[string]fieldpolicy.PrefillClass) map[string]string {
	out := make(map[string]string, len(p))
	for k, v := range p {
		out[k] = string(v)
	}
	return out
}

type saveFieldPolicyRequest struct {
	Scope   scopeView           `json:"scope"`
	Policy  map[string]string   `json:"policy"`
	Prefill map[string]string   `json:"prefill"`
	Events  map[string][]string `json:"events"`
}

// handleSaveFieldPolicy persists a full replacement of one scope's
// field policy and prefill overrides for the current schema version
// (design handoff B6) — fleet-wide when the request carries no scope
// (or an explicit fleet scope), or a specific vessel group/vessel
// otherwise, letting a company give e.g. a "DP" vessel group a policy
// that shows DP-specific fields while the fleet-wide default (and every
// other group) keeps them hidden. Config-Manager-only. schemaMandatory
// fields are silently skipped rather than erroring — the UI locks that
// control, so a well-behaved client never sends one, and a stale/
// replayed request for a field the schema has since made
// schemaMandatory shouldn't fail the whole save.
func (s *Server) handleSaveFieldPolicy(w http.ResponseWriter, r *http.Request) {
	if !s.requireConfigManager(w, r) {
		return
	}
	name := r.PathValue("name")
	latest, err := s.st.LatestSchemaVersion(r.Context(), name)
	if errors.Is(err, store.ErrNotFound) {
		httpjson.WriteError(w, http.StatusNotFound, "schema not found")
		return
	}
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	parsed, err := schema.Parse(latest.Content)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var req saveFieldPolicyRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	scope := compliance.FleetScope()
	if req.Scope.Type != "" {
		scope, err = req.Scope.toScope()
		if err != nil {
			httpjson.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	p, err := fieldpolicy.New(scope, name, latest.Version)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	for fieldName, state := range req.Policy {
		f, ok := parsed.FieldByName(fieldName)
		if !ok {
			continue // field no longer exists in this version; drop silently
		}
		if f.SchemaMandatory {
			continue // locked by the schema, never a company override
		}
		if err := p.SetPolicy(fieldName, validation.FieldPolicyState(state), false); err != nil {
			httpjson.WriteError(w, http.StatusBadRequest, fmt.Sprintf("field %s: %s", fieldName, err))
			return
		}
	}
	for fieldName, class := range req.Prefill {
		if err := p.SetPrefillClass(fieldName, fieldpolicy.PrefillClass(class)); err != nil {
			httpjson.WriteError(w, http.StatusBadRequest, fmt.Sprintf("field %s: %s", fieldName, err))
			return
		}
	}
	eventTypes, validEvents, err := eventVocabulary(parsed)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for fieldName, events := range req.Events {
		f, ok := parsed.FieldByName(fieldName)
		if !ok {
			continue // field no longer exists in this version; drop silently
		}
		if f.SchemaMandatory {
			// Architecture 6.1: schema mandatoriness is immutable. Letting an
			// event list suppress it would let company config emit OVD output
			// the standard itself rejects, so this is skipped for the same
			// reason a schemaMandatory policy state is.
			continue
		}
		if validEvents == nil {
			httpjson.WriteError(w, http.StatusBadRequest,
				fmt.Sprintf("field %s: schema %s has no voyage event types to narrow policy by", fieldName, name))
			return
		}
		if err := p.SetEvents(fieldName, events, validEvents); err != nil {
			httpjson.WriteError(w, http.StatusBadRequest, fmt.Sprintf("field %s: %s", fieldName, err))
			return
		}
	}

	if err := s.st.SaveFieldPolicy(r.Context(), p); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	fieldsView, err := toSchemaDetailView(latest)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, fieldPolicyView{
		SchemaName: name, Version: latest.Version, Scope: toScopeView(scope), Fields: fieldsView.Fields,
		Policy: stringifyPolicy(p.Policy), Prefill: stringifyPrefill(p.Prefill),
		Events: stringifyEvents(p.Events), EventTypes: eventTypes,
	})
}
