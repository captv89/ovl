// SPDX-License-Identifier: AGPL-3.0-only

package domain

import "time"

// EventType is one of the audit trail event types (architecture 14).
// The event log is append-only and spans both vessel and office; nothing
// is ever deleted.
type EventType string

const (
	EventCreated           EventType = "created"
	EventSectionSaved      EventType = "section_saved"
	EventHealthCheckResult EventType = "health_check_result"
	EventSubmitted         EventType = "submitted"
	EventSynced            EventType = "synced"
	EventRemarked          EventType = "remarked"
	EventCorrectionStarted EventType = "correction_started"
	EventResubmitted       EventType = "resubmitted"
	EventInvalidated       EventType = "invalidated"
	EventRestoreApplied    EventType = "restore_applied"
	// EventFindingAcknowledged is not in architecture 14's original
	// vocabulary — added per the vessel UI rework's explicit requirement
	// that warning acknowledgements be audit-logged and synced to
	// office, not just client-local UI state (see PROJECT.md's "Vessel
	// UI rework" section, Phase 3). Detail carries "ruleId", "field"
	// (may be ""), "message", and "acknowledged" (bool) — the same
	// structured-Detail pattern EventSectionSaved already uses. Toggling
	// acknowledge/un-acknowledge is allowed; each flip is its own
	// append-only event, not an update to a prior one.
	EventFindingAcknowledged EventType = "finding_acknowledged"
)

// Event is one audit trail entry for a report version. Report's
// lifecycle methods return the Event a call produced; it is the
// caller's responsibility to persist it (SQLite on the vessel, Postgres
// in the office) — pkg/domain has no store dependency.
type Event struct {
	ReportID  string
	VersionNo int
	Type      EventType
	At        time.Time // UTC
	Actor     string    // user id/username; "" for system-generated events (e.g. cascade revalidation)
	Detail    map[string]any
}

// FieldChange is one field's before/after value, part of a
// section_saved event's Detail (architecture 14: "field-level delta").
type FieldChange struct {
	Field    string
	OldValue any
	NewValue any
}
