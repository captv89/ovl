// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/captv89/ovl/internal/httpjson"
	"github.com/captv89/ovl/office/fieldpolicy"
	"github.com/captv89/ovl/office/store"
	"github.com/captv89/ovl/office/syncservice"
	"github.com/captv89/ovl/pkg/domain"
	"github.com/captv89/ovl/pkg/schema"
	"github.com/captv89/ovl/pkg/validation"
)

// reportView is the JSON shape for one report version — design handoff
// B4's Report tab. No value-write endpoint exists for this shape and
// none ever will (B4's absolute no-office-edits rule); every route in
// this file is read-only.
type reportView struct {
	ReportID    string         `json:"reportId"`
	VersionNo   int            `json:"versionNo"`
	SchemaName  string         `json:"schemaName"`
	EventType   string         `json:"eventType"`
	EventTime   time.Time      `json:"eventTime"`
	Fields      map[string]any `json:"fields"`
	State       domain.State   `json:"state"`
	SubmittedAt *time.Time     `json:"submittedAt,omitempty"`
}

func toReportView(r *domain.Report) reportView {
	v := reportView{
		ReportID: r.ReportID, VersionNo: r.VersionNo, SchemaName: r.SchemaName, EventType: r.EventType,
		EventTime: r.EventTime, Fields: r.Fields, State: r.State,
	}
	if !r.SubmittedAt.IsZero() {
		t := r.SubmittedAt
		v.SubmittedAt = &t
	}
	return v
}

// healthView is design handoff B3's compact per-row health cell
// (errors/warnings/pass). Deliberately just counts, not the findings
// themselves — the full finding detail already exists per-field on the
// vessel's own "Check report" panel; office's list view only ever needs
// "does this need attention," not the reasons why.
type healthView struct {
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
}

// reportListItemView is one row of design handoff B3's fleet-wide
// reports explorer — the vessel identity columns folded in alongside
// A4's usual report row anatomy.
type reportListItemView struct {
	VesselID    string       `json:"vesselId"`
	VesselName  string       `json:"vesselName"`
	VesselIMO   string       `json:"vesselImo"`
	ReportID    string       `json:"reportId"`
	VersionNo   int          `json:"versionNo"`
	SchemaName  string       `json:"schemaName"`
	EventType   string       `json:"eventType"`
	State       domain.State `json:"state"`
	EventTime   time.Time    `json:"eventTime"`
	SubmittedAt *time.Time   `json:"submittedAt,omitempty"`
	Reviewed    bool         `json:"reviewed"`
	HasRemarks  bool         `json:"hasRemarks"`
	Health      healthView   `json:"health"`
}

func toReportListItemView(row store.ReportListRow, health healthView) reportListItemView {
	v := reportListItemView{
		VesselID: row.VesselID, VesselName: row.VesselName, VesselIMO: row.VesselIMO,
		ReportID: row.ReportID, VersionNo: row.VersionNo, SchemaName: row.SchemaName,
		EventType: row.EventType, State: row.State, EventTime: row.EventTime, Reviewed: row.Reviewed,
		HasRemarks: row.HasRemarks, Health: health,
	}
	if !row.SubmittedAt.IsZero() {
		t := row.SubmittedAt
		v.SubmittedAt = &t
	}
	return v
}

// schemaHealthContext is one schema's parsed schema + field policy +
// validation config — everything evaluateListRowHealth needs besides the
// row's own field values. Built once per unique schema name per
// ListReports request (healthContextCache below), not once per row: the
// reports explorer commonly lists hundreds of rows sharing a handful of
// schemas, and LatestSchemaVersion/LoadFieldPolicy are each a Postgres
// round trip.
type schemaHealthContext struct {
	schema *schema.Schema
	policy validation.FieldPolicy
	// events is the resolved per-voyage-event applicability that goes with
	// policy — carried alongside it so office recomputes the same health a
	// vessel does for a report whose event type the company scoped fields
	// away from, rather than warning about a field that never rendered.
	events validation.FieldEvents
	cfg    *validation.Config
	err    error
}

// loadSchemaHealthContext fetches and parses everything a health/finding
// evaluation for schemaName needs, resolved for one specific vessel: the
// latest published schema version, the company's effective field policy
// (fieldpolicy.EffectiveFieldPolicy over the fleet/group/vessel
// assignments), and the validation config with the vessel's effective rule
// severities (syncservice.ValidationConfigForVessel). Shared by
// healthEvaluator (which caches it per (schema, vessel) across a batch) and
// office/httpapi/commercial.go's single-report creation check.
//
// Resolving per vessel, not fleet-scope, is codebase audit 2026-07-22 §2:
// a vessel with group/vessel-scope field-policy or severity overrides was
// previously health-checked against the wrong (fleet-only) config, so its
// B3 health cell could disagree with what the vessel itself computes.
func loadSchemaHealthContext(ctx context.Context, st *store.Store, schemaName, vesselID string, vesselGroups []string) *schemaHealthContext {
	hc := &schemaHealthContext{}
	latest, err := st.LatestSchemaVersion(ctx, schemaName)
	if err != nil {
		hc.err = err
		return hc
	}
	if hc.schema, err = schema.Parse(latest.Content); err != nil {
		hc.err = err
		return hc
	}
	assignments, err := st.ListFieldPolicyAssignments(ctx, schemaName, latest.Version)
	if err != nil {
		hc.err = err
		return hc
	}
	eff := fieldpolicy.EffectiveFieldPolicy(assignments, vesselID, vesselGroups, schemaName, latest.Version)
	hc.policy, hc.events = eff.Policy, eff.Events
	cfg, err := syncservice.ValidationConfigForVessel(ctx, st, schemaName, vesselID, vesselGroups)
	if err != nil {
		hc.err = err
		return hc
	}
	hc.cfg = cfg
	return hc
}

// healthEvaluator computes B3 health cells for a batch of report rows,
// resolving each row's effective (per-vessel) field policy and rule
// severities. It caches both the per-(schema, vessel) health context and
// per-vessel group lookups across the batch, since a reports list commonly
// has hundreds of rows sharing a handful of (schema, vessel) pairs and each
// resolution is several Postgres round trips.
type healthEvaluator struct {
	st       *store.Store
	contexts map[string]*schemaHealthContext
	groups   map[string][]string
}

func newHealthEvaluator(st *store.Store) *healthEvaluator {
	return &healthEvaluator{
		st:       st,
		contexts: map[string]*schemaHealthContext{},
		groups:   map[string][]string{},
	}
}

// vesselGroups returns vesselID's group tags, cached. A load failure yields
// nil groups (the vessel then resolves at fleet scope only) rather than an
// error — this is a display-only health cell, no worse than the pre-§2
// fleet-only behavior for that one row.
func (e *healthEvaluator) vesselGroups(ctx context.Context, vesselID string) []string {
	if g, ok := e.groups[vesselID]; ok {
		return g
	}
	var g []string
	if v, err := e.st.GetVessel(ctx, vesselID); err == nil {
		g = v.Groups
	}
	e.groups[vesselID] = g
	return g
}

// row computes one report row's health cell (design handoff B3): the same
// two rule families vessel/httpapi.evaluateReport runs for "Check report"
// (field rules + plausibility), against the schema's current latest
// published version and this row's vessel's effective field policy — office
// is the authoritative source of both since Phase 3 B6. Continuity findings
// are deliberately excluded: those need a whole chain, not one row, and
// already surface as the Invalidated state via cascade
// (office/syncservice/cascade.go) rather than an error/warning count. Falls
// back to zero on any load failure (missing schema, bad field policy) — a
// health cell showing "pass" for a report office couldn't evaluate is
// misleading but no worse than the row not rendering at all, and this is
// display-only, not a gate.
func (e *healthEvaluator) row(ctx context.Context, row store.ReportListRow) healthView {
	key := row.SchemaName + "\x00" + row.VesselID
	hc, ok := e.contexts[key]
	if !ok {
		hc = loadSchemaHealthContext(ctx, e.st, row.SchemaName, row.VesselID, e.vesselGroups(ctx, row.VesselID))
		e.contexts[key] = hc
	}
	if hc.err != nil {
		return healthView{}
	}

	vr := &validation.Report{
		ReportID: row.ReportID, VersionNo: row.VersionNo, SchemaName: row.SchemaName,
		EventType: row.EventType, EventTime: row.EventTime, Fields: row.Fields,
	}
	var findings validation.Findings
	findings = append(findings, validation.EvaluateFieldRules(vr, hc.schema, hc.policy, hc.events)...)
	findings = append(findings, validation.EvaluatePlausibilityRules(vr, hc.cfg, nil)...)

	var health healthView
	for _, f := range findings {
		switch f.Severity {
		case validation.SeverityError:
			health.Errors++
		case validation.SeverityWarning:
			health.Warnings++
		}
	}
	return health
}

// parseReportFilter reads B3's filter bar query parameters into a
// store.ReportFilter. Every parameter is optional; an invalid dateFrom/
// dateTo/invalidatedOnly value is the one case worth a 400, since a
// silently-ignored bad filter would show the reviewer an unfiltered list
// without any indication why.
func parseReportFilter(r *http.Request) (store.ReportFilter, error) {
	q := r.URL.Query()
	var filter store.ReportFilter
	if v := q.Get("vesselId"); v != "" {
		filter.VesselID = &v
	}
	if v := q.Get("group"); v != "" {
		filter.GroupID = &v
	}
	if v := q.Get("state"); v != "" {
		filter.State = &v
	}
	if v := q.Get("eventType"); v != "" {
		filter.EventType = &v
	}
	if v := q.Get("schema"); v != "" {
		filter.SchemaName = &v
	}
	if v := q.Get("dateFrom"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return filter, fmt.Errorf("invalid dateFrom %q: %w", v, err)
		}
		filter.DateFrom = &t
	}
	if v := q.Get("dateTo"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return filter, fmt.Errorf("invalid dateTo %q: %w", v, err)
		}
		filter.DateTo = &t
	}
	if v := q.Get("invalidatedOnly"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return filter, fmt.Errorf("invalid invalidatedOnly %q: %w", v, err)
		}
		filter.InvalidatedOnly = &b
	}
	if v := q.Get("hasRemarks"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return filter, fmt.Errorf("invalid hasRemarks %q: %w", v, err)
		}
		filter.HasRemarks = &b
	}
	return filter, nil
}

// handleListReports serves design handoff B3's reports explorer. Viewable
// by any authenticated office user — reading the fleet's reports is not a
// value-editing action.
func (s *Server) handleListReports(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	filter, err := parseReportFilter(r)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	rows, err := s.st.ListReports(r.Context(), filter)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	ev := newHealthEvaluator(s.st)
	out := make([]reportListItemView, len(rows))
	for i, row := range rows {
		health := ev.row(r.Context(), row)
		out[i] = toReportListItemView(row, health)
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}

// reportDetailView is design handoff B4's Report tab response: the
// latest version plus enough vessel identity and version-number history
// to drive a version selector. The full per-version content lives behind
// the separate /versions endpoint (B4's History tab), not duplicated
// here.
type reportDetailView struct {
	VesselID   string     `json:"vesselId"`
	VesselName string     `json:"vesselName"`
	VesselIMO  string     `json:"vesselImo"`
	Latest     reportView `json:"latest"`
	Versions   []int      `json:"versions"`
}

// handleGetReport returns one report's latest version and version-number
// history. 404s if the office has never landed any version of this
// report for this vessel.
func (s *Server) handleGetReport(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	vesselID, reportID := r.PathValue("vesselId"), r.PathValue("reportId")
	versions, err := s.st.ListReportVersions(r.Context(), vesselID, reportID)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(versions) == 0 {
		httpjson.WriteError(w, http.StatusNotFound, "report not found")
		return
	}
	v, err := s.st.GetVessel(r.Context(), vesselID)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	latest := versions[len(versions)-1]
	versionNos := make([]int, len(versions))
	for i, ver := range versions {
		versionNos[i] = ver.VersionNo
	}
	httpjson.WriteJSON(w, http.StatusOK, reportDetailView{
		VesselID: v.ID, VesselName: v.Name, VesselIMO: v.IMO,
		Latest: toReportView(latest), Versions: versionNos,
	})
}

// handleListReportEvents returns one report's full audit trail,
// chronological (architecture 14, design handoff B4's Audit trail tab).
func (s *Server) handleListReportEvents(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	events, err := s.st.ListReportAuditEvents(r.Context(), r.PathValue("vesselId"), r.PathValue("reportId"))
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toEventViews(events))
}

// eventView is the JSON shape for one pkg/domain.Event, mirroring
// vessel/httpapi's own eventView shape plus Origin — office-only (Phase
// 5 T6.3's Audit trail side filter; see store.ReportAuditEventRow's own
// doc comment on why this isn't part of the shared domain.Event).
type eventView struct {
	VersionNo int              `json:"versionNo"`
	Type      domain.EventType `json:"type"`
	At        time.Time        `json:"at"`
	Actor     string           `json:"actor,omitempty"`
	Detail    map[string]any   `json:"detail,omitempty"`
	Origin    string           `json:"origin"`
}

func toEventViews(events []store.ReportAuditEventRow) []eventView {
	out := make([]eventView, len(events))
	for i, e := range events {
		out[i] = eventView{VersionNo: e.VersionNo, Type: e.Type, At: e.At, Actor: e.Actor, Detail: e.Detail, Origin: e.Origin}
	}
	return out
}

// handleListReportVersions returns every version of one report, ascending
// — design handoff B4's History tab.
func (s *Server) handleListReportVersions(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	versions, err := s.st.ListReportVersions(r.Context(), r.PathValue("vesselId"), r.PathValue("reportId"))
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]reportView, len(versions))
	for i, v := range versions {
		out[i] = toReportView(v)
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}
