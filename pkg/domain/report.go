// SPDX-License-Identifier: AGPL-3.0-only

package domain

import (
	"fmt"
	"maps"
	"reflect"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/captv89/ovl/pkg/validation"
)

// Report is the report aggregate (architecture 8.1/8.2): identity,
// schema-driven field data, and lifecycle state. It is the persistence-
// layer type pkg/validation.Report's own doc comment anticipates —
// ToValidation converts to the minimal view the rule engine needs,
// keeping the engine itself store- and lifecycle-agnostic.
type Report struct {
	ReportID  string // UUIDv7, stable across corrections
	VersionNo int    // starts at 1; a correction creates VersionNo+1

	SchemaName string // matches schema.Schema.SchemaName, e.g. "log-abstract"
	EventType  string // OVD event type, e.g. "Departure"
	EventTime  time.Time
	Fields     map[string]any

	State State
	// InvalidatedFrom is the state this report was in immediately before
	// cascade revalidation flipped it to StateInvalidated. Zero value
	// ("") when State != StateInvalidated.
	InvalidatedFrom  State
	InvalidatedRules []string // broken continuity rule IDs; set when State == StateInvalidated

	CreatedAt   time.Time
	CreatedBy   string
	UpdatedAt   time.Time
	SubmittedAt time.Time // zero value until Submit succeeds
	SubmittedBy string
}

// NewReport starts a new report version 1 in StateDraft. fields is
// copied defensively so the caller mutating its own map afterward can't
// alias the aggregate's state.
func NewReport(schemaName, eventType string, eventTime time.Time, fields map[string]any, createdBy string) (*Report, Event, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, Event{}, fmt.Errorf("generate report id: %w", err)
	}
	now := time.Now().UTC()
	r := &Report{
		ReportID:   id.String(),
		VersionNo:  1,
		SchemaName: schemaName,
		EventType:  eventType,
		EventTime:  eventTime.UTC(),
		Fields:     maps.Clone(fields),
		State:      StateDraft,
		CreatedAt:  now,
		CreatedBy:  createdBy,
		UpdatedAt:  now,
	}
	r.RecomputeEventTime() // Date_UTC/Time_UTC may already be prefilled
	event := Event{
		ReportID:  r.ReportID,
		VersionNo: r.VersionNo,
		Type:      EventCreated,
		At:        now,
		Actor:     createdBy,
		Detail:    map[string]any{"schemaName": schemaName, "eventType": eventType},
	}
	return r, event, nil
}

// ToValidation returns the minimal view pkg/validation's rule engine
// operates on. The returned Report shares the Fields map with r for
// read purposes; callers must not mutate it through this reference —
// use SaveSection instead.
func (r *Report) ToValidation() *validation.Report {
	return &validation.Report{
		ReportID:   r.ReportID,
		VersionNo:  r.VersionNo,
		SchemaName: r.SchemaName,
		EventType:  r.EventType,
		EventTime:  r.EventTime,
		Fields:     r.Fields,
	}
}

// NaturalKey returns the OVD natural key (IMO, Date_UTC, Time_UTC) used
// for per-vessel entry-time uniqueness and the OVD format's own
// natural-key overwrite semantics (architecture 8.2) — a property of the
// OVD standard itself, not specific to any one consumer of the data. It
// only applies to schemas carrying Date_UTC and
// Time_UTC fields — today that's log-abstract. The other four curated
// schemas each have IMO but not Date_UTC/Time_UTC, and the architecture
// doc does not define a natural key for them; ok is false for those
// until that's specified.
func (r *Report) NaturalKey() (imo, dateUTC, timeUTC string, ok bool) {
	dateVal, dateOk := r.Fields["Date_UTC"].(string)
	timeVal, timeOk := r.Fields["Time_UTC"].(string)
	imoVal, imoOk := r.Fields["IMO"].(float64)
	if !dateOk || !timeOk || !imoOk {
		return "", "", "", false
	}
	return strconv.FormatFloat(imoVal, 'f', 0, 64), dateVal, timeVal, true
}

// RecomputeEventTime updates EventTime from the report's own Date_UTC/
// Time_UTC fields (architecture 8.2/8.3: EventTime drives natural-key
// uniqueness and cascade chain ordering, and must track whatever
// timestamp the officer actually entered — not just whatever was true
// when the report was first created or last saved). It is a no-op when
// either field is absent or unparseable, e.g. schemas without Date_UTC/
// Time_UTC (NaturalKey's own scoping) or a report still mid-entry; in
// that case EventTime keeps its prior value rather than falling back to
// "now" or zero. Returns whether EventTime changed.
func (r *Report) RecomputeEventTime() bool {
	dateVal, dateOk := r.Fields["Date_UTC"].(string)
	timeVal, timeOk := r.Fields["Time_UTC"].(string)
	if !dateOk || !timeOk {
		return false
	}
	t, ok := validation.ParseEventTime(dateVal, timeVal)
	if !ok || t.Equal(r.EventTime) {
		return false
	}
	r.EventTime = t
	return true
}

// SaveSection applies changes to r.Fields and returns the resulting
// section_saved audit event (architecture 14), recording only the
// fields that actually changed. Saving a section is independent of
// other sections (architecture 9.5) and always allowed except once a
// report is locked post-submit — callers should reject saves against a
// submitted/synced/pushed/remarked/invalidated report before calling
// this (a correction must be started first via NewCorrection).
//
// If r was StateReady, a change demotes it back to StateDraft: an
// edited report's previous health check result is stale until
// re-checked via MarkReady.
func (r *Report) SaveSection(section string, changes map[string]any, actor string) Event {
	var deltas []FieldChange
	for name, newVal := range changes {
		oldVal := r.Fields[name]
		if reflect.DeepEqual(oldVal, newVal) {
			continue
		}
		deltas = append(deltas, FieldChange{Field: name, OldValue: oldVal, NewValue: newVal})
		r.Fields[name] = newVal
	}
	now := time.Now().UTC()
	if len(deltas) > 0 {
		r.UpdatedAt = now
		if r.State == StateReady {
			r.State = StateDraft
		}
		r.RecomputeEventTime()
	}
	return Event{
		ReportID:  r.ReportID,
		VersionNo: r.VersionNo,
		Type:      EventSectionSaved,
		At:        now,
		Actor:     actor,
		Detail:    map[string]any{"section": section, "changes": deltas},
	}
}
