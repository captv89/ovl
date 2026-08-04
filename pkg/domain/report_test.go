// SPDX-License-Identifier: AGPL-3.0-only

package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewReport(t *testing.T) {
	fields := map[string]any{"IMO": 9876543.0}
	eventTime := time.Date(2026, 1, 10, 6, 0, 0, 0, time.UTC)

	r, event, err := NewReport("log-abstract", "Departure", eventTime, fields, "master")
	if err != nil {
		t.Fatalf("NewReport: %v", err)
	}
	if _, err := uuid.Parse(r.ReportID); err != nil {
		t.Errorf("ReportID = %q is not a valid UUID: %v", r.ReportID, err)
	}
	if r.VersionNo != 1 {
		t.Errorf("VersionNo = %d, want 1", r.VersionNo)
	}
	if r.State != StateDraft {
		t.Errorf("State = %q, want %q", r.State, StateDraft)
	}
	if r.CreatedBy != "master" {
		t.Errorf("CreatedBy = %q, want master", r.CreatedBy)
	}
	if !r.EventTime.Equal(eventTime) {
		t.Errorf("EventTime = %v, want %v", r.EventTime, eventTime)
	}
	if event.Type != EventCreated || event.ReportID != r.ReportID || event.VersionNo != 1 {
		t.Errorf("created event = %+v, want type=created reportID=%s versionNo=1", event, r.ReportID)
	}

	// Defensive copy: mutating the caller's map must not affect r.
	fields["IMO"] = 1.0
	if got, _ := r.Fields["IMO"].(float64); got != 9876543.0 {
		t.Errorf("Fields[IMO] = %v after caller mutation, want unchanged 9876543.0 (defensive copy broken)", got)
	}
}

func TestReport_ToValidation(t *testing.T) {
	r, _, err := NewReport("log-abstract", "Departure", time.Now(), map[string]any{"IMO": 1.0}, "master")
	if err != nil {
		t.Fatalf("NewReport: %v", err)
	}
	v := r.ToValidation()
	if v.ReportID != r.ReportID || v.VersionNo != r.VersionNo || v.SchemaName != r.SchemaName ||
		v.EventType != r.EventType || !v.EventTime.Equal(r.EventTime) {
		t.Errorf("ToValidation() = %+v, want fields matching %+v", v, r)
	}
}

func TestReport_NaturalKey(t *testing.T) {
	tests := []struct {
		name   string
		fields map[string]any
		wantOK bool
		imo    string
		date   string
		tim    string
	}{
		{
			name:   "complete",
			fields: map[string]any{"IMO": 9876543.0, "Date_UTC": "2026-01-10", "Time_UTC": "06:00"},
			wantOK: true,
			imo:    "9876543",
			date:   "2026-01-10",
			tim:    "06:00",
		},
		{
			name:   "missing Date_UTC (e.g. bunker-report)",
			fields: map[string]any{"IMO": 9876543.0},
			wantOK: false,
		},
		{
			name:   "IMO wrong type",
			fields: map[string]any{"IMO": "not-a-number", "Date_UTC": "2026-01-10", "Time_UTC": "06:00"},
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Report{Fields: tt.fields}
			imo, date, tim, ok := r.NaturalKey()
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if imo != tt.imo || date != tt.date || tim != tt.tim {
				t.Errorf("NaturalKey() = (%q, %q, %q), want (%q, %q, %q)", imo, date, tim, tt.imo, tt.date, tt.tim)
			}
		})
	}
}

func TestReport_SaveSection(t *testing.T) {
	r, _, err := NewReport("log-abstract", "Departure", time.Now(), map[string]any{"Distance": 10.0}, "master")
	if err != nil {
		t.Fatalf("NewReport: %v", err)
	}
	r.State = StateReady // simulate having already passed a health check

	event := r.SaveSection("distanceAndSpeed", map[string]any{
		"Distance": 12.0,     // changed
		"Mode":     "InPort", // new field, was absent
	}, "2/O")

	if r.Fields["Distance"] != 12.0 || r.Fields["Mode"] != "InPort" {
		t.Errorf("Fields after save = %+v, want Distance=12.0 Mode=InPort", r.Fields)
	}
	if r.State != StateDraft {
		t.Errorf("State = %q after an edit, want demotion to %q", r.State, StateDraft)
	}
	if event.Type != EventSectionSaved || event.Actor != "2/O" {
		t.Errorf("event = %+v, want type=section_saved actor=2/O", event)
	}
	changes, ok := event.Detail["changes"].([]FieldChange)
	if !ok || len(changes) != 2 {
		t.Fatalf("event.Detail[changes] = %v, want 2 FieldChange entries", event.Detail["changes"])
	}

	// Saving with no actual change must not touch UpdatedAt or State.
	r2, _, _ := NewReport("log-abstract", "Departure", time.Now(), map[string]any{"Distance": 10.0}, "master")
	r2.State = StateReady
	before := r2.UpdatedAt
	noopEvent := r2.SaveSection("distanceAndSpeed", map[string]any{"Distance": 10.0}, "2/O")
	if r2.State != StateReady {
		t.Errorf("State = %q after a no-op save, want unchanged %q", r2.State, StateReady)
	}
	if !r2.UpdatedAt.Equal(before) {
		t.Errorf("UpdatedAt changed on a no-op save")
	}
	if changes, _ := noopEvent.Detail["changes"].([]FieldChange); len(changes) != 0 {
		t.Errorf("no-op save event has %d changes, want 0", len(changes))
	}
}

func TestReport_SaveSection_RecomputesEventTime(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) // "now" at creation, before Date_UTC/Time_UTC are known
	r, _, err := NewReport("log-abstract", "Departure", created, map[string]any{}, "master")
	if err != nil {
		t.Fatalf("NewReport: %v", err)
	}
	if !r.EventTime.Equal(created) {
		t.Fatalf("EventTime = %v before Date_UTC/Time_UTC are set, want the creation-time fallback %v", r.EventTime, created)
	}

	r.SaveSection("header", map[string]any{"Date_UTC": "2026-01-10", "Time_UTC": "06:00"}, "2/O")
	want := time.Date(2026, 1, 10, 6, 0, 0, 0, time.UTC)
	if !r.EventTime.Equal(want) {
		t.Errorf("EventTime after filling Date_UTC/Time_UTC = %v, want %v", r.EventTime, want)
	}

	// A later correction to the logged time must be picked up too, not
	// just the first time the fields are filled in.
	r.SaveSection("header", map[string]any{"Time_UTC": "07:30"}, "2/O")
	want = time.Date(2026, 1, 10, 7, 30, 0, 0, time.UTC)
	if !r.EventTime.Equal(want) {
		t.Errorf("EventTime after correcting Time_UTC = %v, want %v", r.EventTime, want)
	}

	// Schemas without Date_UTC/Time_UTC (e.g. bunker-report) must keep
	// whatever EventTime they were given — NaturalKey's own scoping.
	r2, _, err := NewReport("bunker-report", "Bunkering", created, map[string]any{"IMO": 1.0}, "master")
	if err != nil {
		t.Fatalf("NewReport: %v", err)
	}
	r2.SaveSection("header", map[string]any{"IMO": 2.0}, "2/O")
	if !r2.EventTime.Equal(created) {
		t.Errorf("EventTime = %v for a schema with no Date_UTC/Time_UTC, want unchanged %v", r2.EventTime, created)
	}
}
