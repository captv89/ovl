// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/captv89/ovl/pkg/domain"
)

// ErrNotFound is returned by lookups that find no matching row.
var ErrNotFound = errors.New("store: not found")

const timeLayout = time.RFC3339Nano

// SaveReport inserts a new report version or updates an existing one
// (identified by ReportID+VersionNo). Updating a version whose data
// (fields, event time/type) is locked because it left draft/ready is
// rejected by the reports_locked_after_submit trigger — see
// migrations/00001_init.sql — and surfaces here as an error, not a
// panic or silent no-op.
func (s *Store) SaveReport(ctx context.Context, r *domain.Report) error {
	fieldsJSON, err := json.Marshal(r.Fields)
	if err != nil {
		return fmt.Errorf("marshal fields: %w", err)
	}
	rulesJSON, err := json.Marshal(r.InvalidatedRules)
	if err != nil {
		return fmt.Errorf("marshal invalidated rules: %w", err)
	}
	var submittedAt sql.NullString
	if !r.SubmittedAt.IsZero() {
		submittedAt = sql.NullString{String: r.SubmittedAt.UTC().Format(timeLayout), Valid: true}
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO reports (
			report_id, version_no, schema_name, event_type, event_time, fields,
			state, invalidated_from, invalidated_rules,
			created_at, created_by, updated_at, submitted_at, submitted_by
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (report_id, version_no) DO UPDATE SET
			schema_name       = excluded.schema_name,
			event_type        = excluded.event_type,
			event_time        = excluded.event_time,
			fields            = excluded.fields,
			state             = excluded.state,
			invalidated_from  = excluded.invalidated_from,
			invalidated_rules = excluded.invalidated_rules,
			updated_at        = excluded.updated_at,
			submitted_at      = excluded.submitted_at,
			submitted_by      = excluded.submitted_by
	`,
		r.ReportID, r.VersionNo, r.SchemaName, r.EventType, r.EventTime.UTC().Format(timeLayout), string(fieldsJSON),
		string(r.State), string(r.InvalidatedFrom), string(rulesJSON),
		r.CreatedAt.UTC().Format(timeLayout), r.CreatedBy, r.UpdatedAt.UTC().Format(timeLayout), submittedAt, r.SubmittedBy,
	)
	if err != nil {
		return fmt.Errorf("save report %s v%d: %w", r.ReportID, r.VersionNo, err)
	}
	return nil
}

// DeleteReport permanently removes reportID (all versions, its audit
// trail, and every table that references it — invalidation notices,
// remarks, chat messages, attachment metadata) and records one row in
// deleted_report_log first, capturing enough of the deleted report to
// explain what was removed. Callers must have already confirmed reportID
// is still draft/ready (vessel/httpapi's loadEditableReport does this) —
// this method itself doesn't re-check state, matching this store's
// existing convention of the HTTP layer owning lifecycle policy and the
// store layer just executing it. The attachment blobs themselves are
// left in pkg/attachmentstore untouched, same as DeleteAttachment's own
// reasoning: content-addressed and possibly shared, nothing here is safe
// to assume is uniquely this report's.
//
// One transaction: a report with attachments/notices/remarks partially
// deleted (e.g. on a mid-way failure) would be worse than not deleting
// it at all — an orphaned reports row with no way back into the normal
// edit flow.
func (s *Store) DeleteReport(ctx context.Context, reportID, deletedBy string) error {
	latest, err := s.GetLatestVersion(ctx, reportID)
	if errors.Is(err, ErrNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("load report %s before delete: %w", reportID, err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete report %s: %w", reportID, err)
	}
	defer func() { _ = tx.Rollback() }() // no-op once Commit succeeds

	now := time.Now().UTC()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO deleted_report_log (report_id, schema_name, event_type, event_time, state, deleted_by, deleted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, latest.ReportID, latest.SchemaName, latest.EventType, latest.EventTime.UTC().Format(timeLayout), string(latest.State), deletedBy, now.Format(timeLayout))
	if err != nil {
		return fmt.Errorf("log deletion of report %s: %w", reportID, err)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM attachments WHERE report_id = ?`, reportID); err != nil {
		return fmt.Errorf("delete report %s attachments: %w", reportID, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM chat_messages WHERE report_id = ?`, reportID); err != nil {
		return fmt.Errorf("delete report %s chat messages: %w", reportID, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM remarks WHERE report_id = ?`, reportID); err != nil {
		return fmt.Errorf("delete report %s remarks: %w", reportID, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM invalidation_notices WHERE report_id = ?`, reportID); err != nil {
		return fmt.Errorf("delete report %s invalidation notices: %w", reportID, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM report_events WHERE report_id = ?`, reportID); err != nil {
		return fmt.Errorf("delete report %s events: %w", reportID, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM reports WHERE report_id = ?`, reportID); err != nil {
		return fmt.Errorf("delete report %s: %w", reportID, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete report %s: %w", reportID, err)
	}
	return nil
}

// GetReport returns one specific report version.
func (s *Store) GetReport(ctx context.Context, reportID string, versionNo int) (*domain.Report, error) {
	row := s.db.QueryRowContext(ctx, reportColumns+` FROM reports WHERE report_id = ? AND version_no = ?`, reportID, versionNo)
	return scanReport(row)
}

// GetLatestVersion returns the highest VersionNo report for reportID.
func (s *Store) GetLatestVersion(ctx context.Context, reportID string) (*domain.Report, error) {
	row := s.db.QueryRowContext(ctx, reportColumns+`
		FROM reports WHERE report_id = ? ORDER BY version_no DESC LIMIT 1`, reportID)
	return scanReport(row)
}

// ListLatestReports returns the latest version of every report for
// schemaName, ordered by event time descending — the shape the vessel
// Home screen's recent reports list (architecture 9.4) needs.
func (s *Store) ListLatestReports(ctx context.Context, schemaName string) ([]*domain.Report, error) {
	rows, err := s.db.QueryContext(ctx, reportColumns+`
		FROM reports r
		WHERE schema_name = ?
		  AND version_no = (SELECT MAX(version_no) FROM reports WHERE report_id = r.report_id)
		ORDER BY event_time DESC
	`, schemaName)
	if err != nil {
		return nil, fmt.Errorf("list latest reports for %s: %w", schemaName, err)
	}
	defer func() { _ = rows.Close() }() // rows.Err(), checked below, covers the meaningful failure
	return scanReports(rows)
}

// ListCommittedChain returns the latest *submitted-or-later* version of
// every report for schemaName, ordered by event time ascending — the
// chain pkg/validation's cascade revalidation (Revalidate) and the
// pre-submit continuity check (EvaluateContinuity) operate on.
//
// Two differences from ListChain, both deliberate. Reports whose only
// versions are draft/ready are absent entirely: a report still being
// written is not part of the chain, and letting one in is what used to
// invalidate and lock half-entered reports. And a report currently under
// correction contributes its last committed version rather than its
// in-progress draft — the version the office holds — so a correction in
// progress leaves no hole in the chain for its neighbors to be
// revalidated across.
//
// The state list here is domain.State.InChain's, spelled in SQL so the
// filter can ride along with the per-report MAX(version_no); that
// method's doc comment is the definition, this is a copy of it, and the
// two must move together (same arrangement as 00001_init.sql's
// reports_locked_after_submit trigger).
func (s *Store) ListCommittedChain(ctx context.Context, schemaName string) ([]*domain.Report, error) {
	rows, err := s.db.QueryContext(ctx, reportColumns+`
		FROM reports r
		WHERE schema_name = ?
		  AND state NOT IN ('draft', 'ready')
		  AND version_no = (
		      SELECT MAX(version_no) FROM reports
		      WHERE report_id = r.report_id AND state NOT IN ('draft', 'ready')
		  )
		ORDER BY event_time ASC
	`, schemaName)
	if err != nil {
		return nil, fmt.Errorf("list committed report chain for %s: %w", schemaName, err)
	}
	defer func() { _ = rows.Close() }() // rows.Err(), checked below, covers the meaningful failure
	return scanReports(rows)
}

// ListChain returns the latest version of every report for schemaName,
// ordered by event time ascending — every report the vessel holds,
// drafts included. Continuity work wants ListCommittedChain instead.
func (s *Store) ListChain(ctx context.Context, schemaName string) ([]*domain.Report, error) {
	rows, err := s.db.QueryContext(ctx, reportColumns+`
		FROM reports r
		WHERE schema_name = ?
		  AND version_no = (SELECT MAX(version_no) FROM reports WHERE report_id = r.report_id)
		ORDER BY event_time ASC
	`, schemaName)
	if err != nil {
		return nil, fmt.Errorf("list report chain for %s: %w", schemaName, err)
	}
	defer func() { _ = rows.Close() }() // rows.Err(), checked below, covers the meaningful failure
	return scanReports(rows)
}

// ListVersions returns every version of reportID, ordered oldest first
// — the shape design handoff A7's History tab (Phase 5 T6.3) needs to
// diff consecutive versions. Returns an empty slice, not an error, for
// an unknown reportID.
func (s *Store) ListVersions(ctx context.Context, reportID string) ([]*domain.Report, error) {
	rows, err := s.db.QueryContext(ctx, reportColumns+`
		FROM reports WHERE report_id = ? ORDER BY version_no ASC
	`, reportID)
	if err != nil {
		return nil, fmt.Errorf("list versions for %s: %w", reportID, err)
	}
	defer func() { _ = rows.Close() }() // rows.Err(), checked below, covers the meaningful failure
	return scanReports(rows)
}

const reportColumns = `SELECT
	report_id, version_no, schema_name, event_type, event_time, fields,
	state, invalidated_from, invalidated_rules,
	created_at, created_by, updated_at, submitted_at, submitted_by`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanReport(row rowScanner) (*domain.Report, error) {
	var (
		r                               domain.Report
		state, invalidatedFrom          string
		fieldsJSON, rulesJSON           string
		eventTime, createdAt, updatedAt string
		submittedAt                     sql.NullString
	)
	err := row.Scan(
		&r.ReportID, &r.VersionNo, &r.SchemaName, &r.EventType, &eventTime, &fieldsJSON,
		&state, &invalidatedFrom, &rulesJSON,
		&createdAt, &r.CreatedBy, &updatedAt, &submittedAt, &r.SubmittedBy,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan report: %w", err)
	}
	if err := populateReportTimesAndJSON(&r, state, invalidatedFrom, fieldsJSON, rulesJSON, eventTime, createdAt, updatedAt, submittedAt); err != nil {
		return nil, err
	}
	return &r, nil
}

func scanReports(rows *sql.Rows) ([]*domain.Report, error) {
	var out []*domain.Report
	for rows.Next() {
		r, err := scanReport(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate reports: %w", err)
	}
	return out, nil
}

func populateReportTimesAndJSON(r *domain.Report, state, invalidatedFrom, fieldsJSON, rulesJSON, eventTime, createdAt, updatedAt string, submittedAt sql.NullString) error {
	r.State = domain.State(state)
	r.InvalidatedFrom = domain.State(invalidatedFrom)

	if err := json.Unmarshal([]byte(fieldsJSON), &r.Fields); err != nil {
		return fmt.Errorf("unmarshal fields: %w", err)
	}
	if err := json.Unmarshal([]byte(rulesJSON), &r.InvalidatedRules); err != nil {
		return fmt.Errorf("unmarshal invalidated rules: %w", err)
	}

	var err error
	if r.EventTime, err = time.Parse(timeLayout, eventTime); err != nil {
		return fmt.Errorf("parse event_time: %w", err)
	}
	if r.CreatedAt, err = time.Parse(timeLayout, createdAt); err != nil {
		return fmt.Errorf("parse created_at: %w", err)
	}
	if r.UpdatedAt, err = time.Parse(timeLayout, updatedAt); err != nil {
		return fmt.Errorf("parse updated_at: %w", err)
	}
	if submittedAt.Valid {
		if r.SubmittedAt, err = time.Parse(timeLayout, submittedAt.String); err != nil {
			return fmt.Errorf("parse submitted_at: %w", err)
		}
	}
	return nil
}
