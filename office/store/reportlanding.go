// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/captv89/ovl/pkg/domain"
)

// UpsertReportVersion lands a pushed report version (architecture 11.2
// step 1). Upsert-in-place, not append-only — see report_versions'
// migration comment for why that's safe.
func (s *Store) UpsertReportVersion(ctx context.Context, vesselID string, r *domain.Report, schemaVersion string, receivedAt time.Time) error {
	fieldsJSON, err := json.Marshal(r.Fields)
	if err != nil {
		return fmt.Errorf("marshal fields for report %s v%d: %w", r.ReportID, r.VersionNo, err)
	}
	var submittedAt *time.Time
	if !r.SubmittedAt.IsZero() {
		submittedAt = &r.SubmittedAt
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO report_versions (
			vessel_id, report_id, version_no, schema_kind, schema_version, event_type, state, event_time, fields, submitted_at, received_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (vessel_id, report_id, version_no) DO UPDATE SET
			schema_kind    = EXCLUDED.schema_kind,
			schema_version = EXCLUDED.schema_version,
			event_type     = EXCLUDED.event_type,
			state          = EXCLUDED.state,
			event_time     = EXCLUDED.event_time,
			fields         = EXCLUDED.fields,
			submitted_at   = EXCLUDED.submitted_at,
			received_at    = EXCLUDED.received_at
	`, vesselID, r.ReportID, r.VersionNo, r.SchemaName, schemaVersion, r.EventType, string(r.State), r.EventTime, string(fieldsJSON), submittedAt, receivedAt)
	if err != nil {
		return fmt.Errorf("upsert report version %s v%d for vessel %s: %w", r.ReportID, r.VersionNo, vesselID, err)
	}
	return nil
}

// GetReportVersion returns one landed report version. Returns
// ErrNotFound if the office has never received it.
func (s *Store) GetReportVersion(ctx context.Context, vesselID, reportID string, versionNo int) (*domain.Report, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT report_id, version_no, schema_kind, event_type, state, event_time, fields, submitted_at
		FROM report_versions WHERE vessel_id = $1 AND report_id = $2 AND version_no = $3
	`, vesselID, reportID, versionNo)
	var (
		r           domain.Report
		state       string
		fieldsJSON  string
		submittedAt sql.NullTime
	)
	err := row.Scan(&r.ReportID, &r.VersionNo, &r.SchemaName, &r.EventType, &state, &r.EventTime, &fieldsJSON, &submittedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan report version: %w", err)
	}
	r.State = domain.State(state)
	if err := json.Unmarshal([]byte(fieldsJSON), &r.Fields); err != nil {
		return nil, fmt.Errorf("unmarshal fields: %w", err)
	}
	if submittedAt.Valid {
		r.SubmittedAt = submittedAt.Time
	}
	return &r, nil
}

// AppendReportAuditEvent lands one audit event (architecture 14: the
// audit trail spans both vessel and office). Append-only — see its
// migration comment. origin is "vessel" for events relayed from a
// vessel's outbox push, or "office" for events office computes/authors
// itself directly (cascade invalidation, a reviewer's remark set) —
// Phase 5 T6.3's Audit trail side filter, an office-storage concept not
// part of the shared pkg/domain.Event.
func (s *Store) AppendReportAuditEvent(ctx context.Context, vesselID string, e domain.Event, receivedAt time.Time, origin string) error {
	detail := e.Detail
	if detail == nil {
		detail = map[string]any{}
	}
	detailJSON, err := json.Marshal(detail)
	if err != nil {
		return fmt.Errorf("marshal detail for event on report %s v%d: %w", e.ReportID, e.VersionNo, err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO report_audit_events (vessel_id, report_id, version_no, event_type, actor, occurred_at, detail, received_at, origin)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, vesselID, e.ReportID, e.VersionNo, string(e.Type), e.Actor, e.At, string(detailJSON), receivedAt, origin)
	if err != nil {
		return fmt.Errorf("append report audit event for %s v%d, vessel %s: %w", e.ReportID, e.VersionNo, vesselID, err)
	}
	return nil
}

// ReportFilter narrows ListReports (design handoff B3's explorer filter
// bar: vessel, group, state, event type, schema, date range, has-
// remarks, invalidated-only). A nil field means "don't filter on this."
// PushFailedOnly (also named in B3) is deliberately absent: it needed a
// "push failed" concept for the since-cancelled Veracity batch-push
// integration (DNV declined API access) — office's data egress is now
// pull-based (external customers query via API key + GraphQL/CSV), which
// has no equivalent "push" step that can fail, so this filter has no
// future version to build either, rather than adding an inert filter
// field now. HasRemarks was in that same deferred state
// until Slice S5 landed the remarks table it depends on; wired here in
// Slice S6 (T6.2).
type ReportFilter struct {
	VesselID        *string
	GroupID         *string // vessel group tag, matched via vessels.groups containment
	State           *string
	EventType       *string
	SchemaName      *string
	DateFrom        *time.Time // inclusive, matched against event_time
	DateTo          *time.Time // inclusive, matched against event_time
	HasRemarks      *bool
	InvalidatedOnly *bool
	// Reviewed narrows to the report_reviews triage flag directly — added
	// for the Office UI rework's Dashboard (B1) "reports needing review"
	// KPI, which needs state=remarked AND reviewed=false as a server-side
	// count rather than a client-side filter over a possibly-large list.
	Reviewed *bool
	// Limit/Offset bound the result set — added for the external data API
	// (architecture 13.2): a GraphQL/CSV caller outside office's own UI
	// must never be able to trigger an unbounded full-table scan the way
	// office staff's own bounded fleet view always could safely do
	// without one. nil Limit means "no limit," preserving every existing
	// internal caller's behavior unchanged — those call sites never set
	// this field.
	Limit  *int
	Offset *int
}

// ReportListRow is one row of ListReports' result — the latest version
// of one report, with just enough vessel identity folded in for B3's
// "same row anatomy as A4 plus a vessel column." Reviewed reflects
// report_reviews (Slice S6's office-only "mark reviewed" triage
// annotation, open question 3) — never a lifecycle state, so it's
// folded in here as a plain bool rather than living on domain.Report.
type ReportListRow struct {
	VesselID    string
	VesselName  string
	VesselIMO   string
	ReportID    string
	VersionNo   int
	SchemaName  string
	EventType   string
	State       domain.State
	EventTime   time.Time
	SubmittedAt time.Time
	Reviewed    bool
	HasRemarks  bool
	// Fields carries the report's field values so callers can compute a
	// health/findings summary (design handoff B3's health cell) without a
	// second per-row fetch. Not sent to the client as-is.
	Fields map[string]any
}

// ListReports returns the latest version of every report matching
// filter, newest event time first (design handoff B3). "Latest version"
// is determined per (vessel_id, report_id) via a max-version-no join,
// mirroring vessel/store.ListChain's shape.
func (s *Store) ListReports(ctx context.Context, filter ReportFilter) ([]ReportListRow, error) {
	query := `
		SELECT rv.vessel_id, v.name, v.imo, rv.report_id, rv.version_no, rv.schema_kind, rv.event_type, rv.state, rv.event_time, rv.submitted_at,
			rr.vessel_id IS NOT NULL AS reviewed,
			EXISTS (SELECT 1 FROM remarks re WHERE re.vessel_id = rv.vessel_id AND re.report_id = rv.report_id) AS has_remarks,
			rv.fields
		FROM report_versions rv
		INNER JOIN (
			SELECT vessel_id, report_id, MAX(version_no) AS version_no
			FROM report_versions
			GROUP BY vessel_id, report_id
		) latest ON latest.vessel_id = rv.vessel_id AND latest.report_id = rv.report_id AND latest.version_no = rv.version_no
		INNER JOIN vessels v ON v.id = rv.vessel_id
		LEFT JOIN report_reviews rr ON rr.vessel_id = rv.vessel_id AND rr.report_id = rv.report_id
	`
	var conditions []string
	var args []any
	addCondition := func(cond string, arg any) {
		args = append(args, arg)
		conditions = append(conditions, fmt.Sprintf(cond, len(args)))
	}
	if filter.VesselID != nil {
		addCondition("rv.vessel_id = $%d", *filter.VesselID)
	}
	if filter.GroupID != nil {
		needleJSON, err := json.Marshal([]string{*filter.GroupID})
		if err != nil {
			return nil, fmt.Errorf("marshal group %q: %w", *filter.GroupID, err)
		}
		addCondition("v.groups @> $%d::jsonb", string(needleJSON))
	}
	if filter.State != nil {
		addCondition("rv.state = $%d", *filter.State)
	}
	if filter.EventType != nil {
		addCondition("rv.event_type = $%d", *filter.EventType)
	}
	if filter.SchemaName != nil {
		addCondition("rv.schema_kind = $%d", *filter.SchemaName)
	}
	if filter.DateFrom != nil {
		addCondition("rv.event_time >= $%d", *filter.DateFrom)
	}
	if filter.DateTo != nil {
		addCondition("rv.event_time <= $%d", *filter.DateTo)
	}
	if filter.InvalidatedOnly != nil && *filter.InvalidatedOnly {
		addCondition("rv.state = $%d", string(domain.StateInvalidated))
	}
	if filter.HasRemarks != nil && *filter.HasRemarks {
		conditions = append(conditions, "EXISTS (SELECT 1 FROM remarks re WHERE re.vessel_id = rv.vessel_id AND re.report_id = rv.report_id)")
	}
	if filter.Reviewed != nil {
		if *filter.Reviewed {
			conditions = append(conditions, "rr.vessel_id IS NOT NULL")
		} else {
			conditions = append(conditions, "rr.vessel_id IS NULL")
		}
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY rv.event_time DESC, rv.report_id DESC"
	if filter.Limit != nil {
		args = append(args, *filter.Limit)
		query += fmt.Sprintf(" LIMIT $%d", len(args))
	}
	if filter.Offset != nil {
		args = append(args, *filter.Offset)
		query += fmt.Sprintf(" OFFSET $%d", len(args)) // #nosec G202 -- only the placeholder index (len(args)) is interpolated, never a value; the actual offset is bound via args/QueryContext
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list reports: %w", err)
	}
	defer func() { _ = rows.Close() }() // rows.Err(), checked below, covers the meaningful failure

	var out []ReportListRow
	for rows.Next() {
		var (
			row         ReportListRow
			state       string
			submittedAt sql.NullTime
			fieldsJSON  string
		)
		if err := rows.Scan(&row.VesselID, &row.VesselName, &row.VesselIMO, &row.ReportID, &row.VersionNo,
			&row.SchemaName, &row.EventType, &state, &row.EventTime, &submittedAt, &row.Reviewed, &row.HasRemarks, &fieldsJSON); err != nil {
			return nil, fmt.Errorf("scan report list row: %w", err)
		}
		row.State = domain.State(state)
		if submittedAt.Valid {
			row.SubmittedAt = submittedAt.Time
		}
		if err := json.Unmarshal([]byte(fieldsJSON), &row.Fields); err != nil {
			return nil, fmt.Errorf("unmarshal fields for report list row: %w", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate report list: %w", err)
	}
	return out, nil
}

// ListReportVersions returns every version of one report, ascending by
// version number (design handoff B4's History tab).
func (s *Store) ListReportVersions(ctx context.Context, vesselID, reportID string) ([]*domain.Report, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT report_id, version_no, schema_kind, event_type, state, event_time, fields, submitted_at
		FROM report_versions WHERE vessel_id = $1 AND report_id = $2 ORDER BY version_no ASC
	`, vesselID, reportID)
	if err != nil {
		return nil, fmt.Errorf("list report versions for %s, vessel %s: %w", reportID, vesselID, err)
	}
	defer func() { _ = rows.Close() }() // rows.Err(), checked below, covers the meaningful failure

	var out []*domain.Report
	for rows.Next() {
		var (
			r           domain.Report
			state       string
			fieldsJSON  string
			submittedAt sql.NullTime
		)
		if err := rows.Scan(&r.ReportID, &r.VersionNo, &r.SchemaName, &r.EventType, &state, &r.EventTime, &fieldsJSON, &submittedAt); err != nil {
			return nil, fmt.Errorf("scan report version: %w", err)
		}
		r.State = domain.State(state)
		if err := json.Unmarshal([]byte(fieldsJSON), &r.Fields); err != nil {
			return nil, fmt.Errorf("unmarshal fields: %w", err)
		}
		if submittedAt.Valid {
			r.SubmittedAt = submittedAt.Time
		}
		out = append(out, &r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate report versions: %w", err)
	}
	return out, nil
}

// ReportAuditEventRow pairs a domain.Event with which side produced it
// (Phase 5 T6.3's Audit trail side filter). Origin is an office-storage
// concept — pkg/domain.Event itself carries no notion of vessel vs.
// office, since that struct is shared code the same lifecycle state
// machine uses on both apps.
type ReportAuditEventRow struct {
	domain.Event
	Origin string
}

// ListReportAuditEvents returns one report's full audit trail,
// chronological (architecture 14, design handoff B4's Audit trail tab —
// this table already spans both vessel- and office-originated events).
func (s *Store) ListReportAuditEvents(ctx context.Context, vesselID, reportID string) ([]ReportAuditEventRow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT report_id, version_no, event_type, actor, occurred_at, detail, origin
		FROM report_audit_events WHERE vessel_id = $1 AND report_id = $2 ORDER BY occurred_at ASC
	`, vesselID, reportID)
	if err != nil {
		return nil, fmt.Errorf("list report audit events for %s, vessel %s: %w", reportID, vesselID, err)
	}
	defer func() { _ = rows.Close() }() // rows.Err(), checked below, covers the meaningful failure

	var out []ReportAuditEventRow
	for rows.Next() {
		var (
			e          ReportAuditEventRow
			eventType  string
			detailJSON string
		)
		if err := rows.Scan(&e.ReportID, &e.VersionNo, &eventType, &e.Actor, &e.At, &detailJSON, &e.Origin); err != nil {
			return nil, fmt.Errorf("scan report audit event: %w", err)
		}
		e.Type = domain.EventType(eventType)
		if err := json.Unmarshal([]byte(detailJSON), &e.Detail); err != nil {
			return nil, fmt.Errorf("unmarshal detail: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate report audit events: %w", err)
	}
	return out, nil
}

// ListChain returns the latest version of every report for
// (vesselID, schemaName), ascending by event time — the shape
// pkg/validation's cascade revalidation (Revalidate) needs, mirroring
// vessel/store.ListChain's shape plus a vessel_id scope (one office
// holds many vessels' chains).
func (s *Store) ListChain(ctx context.Context, vesselID, schemaName string) ([]*domain.Report, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT rv.report_id, rv.version_no, rv.schema_kind, rv.event_type, rv.state, rv.event_time, rv.fields, rv.submitted_at
		FROM report_versions rv
		INNER JOIN (
			SELECT report_id, MAX(version_no) AS version_no
			FROM report_versions WHERE vessel_id = $1 AND schema_kind = $2
			GROUP BY report_id
		) latest ON latest.report_id = rv.report_id AND latest.version_no = rv.version_no
		WHERE rv.vessel_id = $1 AND rv.schema_kind = $2
		ORDER BY rv.event_time ASC
	`, vesselID, schemaName)
	if err != nil {
		return nil, fmt.Errorf("list chain for %s, vessel %s: %w", schemaName, vesselID, err)
	}
	defer func() { _ = rows.Close() }() // rows.Err(), checked below, covers the meaningful failure

	var out []*domain.Report
	for rows.Next() {
		var (
			r           domain.Report
			state       string
			fieldsJSON  string
			submittedAt sql.NullTime
		)
		if err := rows.Scan(&r.ReportID, &r.VersionNo, &r.SchemaName, &r.EventType, &state, &r.EventTime, &fieldsJSON, &submittedAt); err != nil {
			return nil, fmt.Errorf("scan chain report version: %w", err)
		}
		r.State = domain.State(state)
		if err := json.Unmarshal([]byte(fieldsJSON), &r.Fields); err != nil {
			return nil, fmt.Errorf("unmarshal fields: %w", err)
		}
		if submittedAt.Valid {
			r.SubmittedAt = submittedAt.Time
		}
		out = append(out, &r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate chain: %w", err)
	}
	return out, nil
}

// UpdateReportVersionState persists just the lifecycle state field of
// one report version — cascade revalidation's own write (architecture
// 8.3: a report can flip to invalidated because of a change elsewhere
// in the chain, with nothing else about it changing). A targeted UPDATE
// rather than routing through UpsertReportVersion's whole-row upsert,
// which would need (and risk clobbering) the schema_version/fields this
// call never has reason to touch.
func (s *Store) UpdateReportVersionState(ctx context.Context, vesselID, reportID string, versionNo int, state domain.State) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE report_versions SET state = $1 WHERE vessel_id = $2 AND report_id = $3 AND version_no = $4
	`, string(state), vesselID, reportID, versionNo)
	if err != nil {
		return fmt.Errorf("update report version state for %s v%d, vessel %s: %w", reportID, versionNo, vesselID, err)
	}
	return nil
}

// HasOutboxReceipt reports whether itemID has already been received
// from vesselID — PushOutbox's idempotency check (architecture 11.2:
// "retransmission after a dropped link is harmless").
func (s *Store) HasOutboxReceipt(ctx context.Context, vesselID, itemID string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM outbox_receipts WHERE vessel_id = $1 AND item_id = $2)
	`, vesselID, itemID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check outbox receipt %s for vessel %s: %w", itemID, vesselID, err)
	}
	return exists, nil
}

// RecordOutboxReceipt marks itemID as received from vesselID, so a
// retransmission of the same item is recognized as a no-op next time.
func (s *Store) RecordOutboxReceipt(ctx context.Context, vesselID, itemID string, sequenceNo int64, receivedAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO outbox_receipts (vessel_id, item_id, sequence_no, received_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (vessel_id, item_id) DO NOTHING
	`, vesselID, itemID, sequenceNo, receivedAt)
	if err != nil {
		return fmt.Errorf("record outbox receipt %s for vessel %s: %w", itemID, vesselID, err)
	}
	return nil
}

// LastReportEventTimeByVessel returns each vessel's most recent report
// event_time across every schema and report, keyed by vessel_id — design
// handoff B2's "last report" column and its overdue-by companion. One
// aggregate query rather than a per-vessel lookup: the fleet list shows
// every vessel at once, so N+1 here would mean N Postgres round trips on
// every page load.
func (s *Store) LastReportEventTimeByVessel(ctx context.Context) (map[string]time.Time, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT vessel_id, MAX(event_time) FROM report_versions GROUP BY vessel_id
	`)
	if err != nil {
		return nil, fmt.Errorf("last report event time by vessel: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[string]time.Time)
	for rows.Next() {
		var vesselID string
		var eventTime time.Time
		if err := rows.Scan(&vesselID, &eventTime); err != nil {
			return nil, fmt.Errorf("scan last report event time row: %w", err)
		}
		out[vesselID] = eventTime
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate last report event time rows: %w", err)
	}
	return out, nil
}

// LogAbstractFieldsRow is one vessel's most recent Log Abstract report's
// field values plus when that report's event occurred.
type LogAbstractFieldsRow struct {
	Fields    map[string]any
	EventTime time.Time
}

// LatestLogAbstractFieldsByVessel returns each vessel's most recent
// Log Abstract report's field values, keyed by vessel_id — design
// handoff B2·M's fleet map, the only schema that carries a position
// (Latitude/Longitude degree+minutes+hemisphere sub-fields; the office
// httpapi layer parses those out, not this query, since "what counts as
// a position" is a schema-content concern, not a storage one). "Most
// recent" is by event_time across every Log Abstract report the vessel
// has, not per report_id — a vessel's current position comes from
// whichever report mentioned it last, not from re-checking one specific
// report's own version history.
func (s *Store) LatestLogAbstractFieldsByVessel(ctx context.Context) (map[string]LogAbstractFieldsRow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT ON (rv.vessel_id) rv.vessel_id, rv.fields, rv.event_time
		FROM report_versions rv
		INNER JOIN (
			SELECT vessel_id, report_id, MAX(version_no) AS version_no
			FROM report_versions
			WHERE schema_kind = 'log-abstract'
			GROUP BY vessel_id, report_id
		) latest ON latest.vessel_id = rv.vessel_id AND latest.report_id = rv.report_id AND latest.version_no = rv.version_no
		WHERE rv.schema_kind = 'log-abstract'
		ORDER BY rv.vessel_id, rv.event_time DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("latest log abstract fields by vessel: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[string]LogAbstractFieldsRow)
	for rows.Next() {
		var (
			vesselID   string
			fieldsJSON string
			eventTime  time.Time
		)
		if err := rows.Scan(&vesselID, &fieldsJSON, &eventTime); err != nil {
			return nil, fmt.Errorf("scan latest log abstract fields row: %w", err)
		}
		var fields map[string]any
		if err := json.Unmarshal([]byte(fieldsJSON), &fields); err != nil {
			return nil, fmt.Errorf("unmarshal fields for vessel %s: %w", vesselID, err)
		}
		out[vesselID] = LogAbstractFieldsRow{Fields: fields, EventTime: eventTime}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate latest log abstract fields rows: %w", err)
	}
	return out, nil
}
