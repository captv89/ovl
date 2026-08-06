// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/captv89/ovl/internal/httpjson"
	"github.com/captv89/ovl/office/apikey"
	"github.com/captv89/ovl/office/store"
	"github.com/captv89/ovl/pkg/csvout"
	"github.com/captv89/ovl/pkg/domain"
	"github.com/captv89/ovl/pkg/schema"
)

// handleReportsCSV serves architecture 13.3's CSV export — the bulk/
// compliance-style download offered alongside GraphQL, behind the same
// API-key gate (codebase audit 2026-07-22 §5: pkg/csvout existed and was
// golden-file-tested since Phase 1 but had no serving surface, so the spec
// overstated what shipped). csvout.Generate produces one
// file per schema (a single schema's columns), so the schema query
// parameter is required; the remaining filters mirror the GraphQL Reports
// query, and the API key's own group scope always wins over a groupId
// argument, exactly as office/graphql's Reports resolver enforces — a
// scoped key must never read outside its vessel group by omitting groupId.
func (s *Server) handleReportsCSV(w http.ResponseWriter, r *http.Request) {
	key, ok := s.authenticatedAPIKey(w, r)
	if !ok {
		return
	}
	if err := s.st.RecordAPIKeyEvent(r.Context(), key.ID, "usedCSV", time.Now().UTC()); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	q := r.URL.Query()
	schemaName := q.Get("schema")
	if schemaName == "" {
		httpjson.WriteError(w, http.StatusBadRequest, "schema query parameter is required (one CSV file per schema)")
		return
	}

	filter, err := csvReportFilter(q, schemaName, key)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	latest, err := s.st.LatestSchemaVersion(r.Context(), schemaName)
	if err != nil {
		httpjson.WriteError(w, http.StatusNotFound, fmt.Sprintf("unknown schema %q", schemaName))
		return
	}
	sch, err := schema.Parse(latest.Content)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	rows, err := s.st.ListReports(r.Context(), filter)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	reports := make([]*domain.Report, len(rows))
	for i, row := range rows {
		// csvout.Generate reads only ReportID/SchemaName/Fields.
		reports[i] = &domain.Report{ReportID: row.ReportID, SchemaName: row.SchemaName, Fields: row.Fields}
	}

	// Buffer before writing so a formatting failure yields a clean error
	// status rather than a truncated download with a 200 already sent —
	// report volumes here are modest (the GraphQL path buffers all rows in
	// memory too).
	var buf bytes.Buffer
	if err := csvout.Generate(&buf, sch, reports); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, schemaName+".csv"))
	_, _ = w.Write(buf.Bytes())
}

// csvReportFilter builds the report query for a CSV export: schema-scoped
// (required, since a CSV is one schema's columns) plus the same optional
// filters the GraphQL Reports query accepts, with the key's group scope
// forced to win over any groupId argument.
func csvReportFilter(q url.Values, schemaName string, key *apikey.APIKey) (store.ReportFilter, error) {
	f := store.ReportFilter{SchemaName: &schemaName}
	if v := q.Get("vesselId"); v != "" {
		f.VesselID = &v
	}
	if v := q.Get("group"); v != "" {
		f.GroupID = &v
	}
	if v := q.Get("state"); v != "" {
		f.State = &v
	}
	if v := q.Get("eventType"); v != "" {
		f.EventType = &v
	}
	if v := q.Get("dateFrom"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return f, fmt.Errorf("invalid dateFrom %q: %w", v, err)
		}
		f.DateFrom = &t
	}
	if v := q.Get("dateTo"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return f, fmt.Errorf("invalid dateTo %q: %w", v, err)
		}
		f.DateTo = &t
	}
	// The key's own group scope always wins over the request argument.
	if key.GroupID != nil {
		f.GroupID = key.GroupID
	}
	return f, nil
}
