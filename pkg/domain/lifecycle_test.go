// SPDX-License-Identifier: AGPL-3.0-only

package domain

import (
	"testing"
	"time"

	"github.com/captv89/ovl/pkg/validation"
)

func newTestReport(t *testing.T, state State) *Report {
	t.Helper()
	r, _, err := NewReport("log-abstract", "Departure", time.Now(), map[string]any{"IMO": 1.0}, "master")
	if err != nil {
		t.Fatalf("NewReport: %v", err)
	}
	r.State = state
	return r
}

func TestReport_MarkReady(t *testing.T) {
	tests := []struct {
		name      string
		fromState State
		findings  validation.Findings
		wantState State
		wantErr   bool
	}{
		{
			name:      "no findings from draft becomes ready",
			fromState: StateDraft,
			findings:  nil,
			wantState: StateReady,
		},
		{
			name:      "warnings only still becomes ready",
			fromState: StateDraft,
			findings:  validation.Findings{{Severity: validation.SeverityWarning}},
			wantState: StateReady,
		},
		{
			name:      "an error keeps it in draft",
			fromState: StateDraft,
			findings:  validation.Findings{{Severity: validation.SeverityError}},
			wantState: StateDraft,
			wantErr:   true,
		},
		{
			name:      "re-checking from ready with a new error demotes to draft",
			fromState: StateReady,
			findings:  validation.Findings{{Severity: validation.SeverityError}},
			wantState: StateDraft,
			wantErr:   true,
		},
		{
			name:      "cannot health-check a submitted report",
			fromState: StateSubmitted,
			findings:  nil,
			wantState: StateSubmitted,
			wantErr:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestReport(t, tt.fromState)
			_, err := r.MarkReady(tt.findings)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if r.State != tt.wantState {
				t.Errorf("State = %q, want %q", r.State, tt.wantState)
			}
		})
	}
}

func TestReport_Submit(t *testing.T) {
	t.Run("from ready succeeds", func(t *testing.T) {
		r := newTestReport(t, StateReady)
		event, err := r.Submit("master")
		if err != nil {
			t.Fatalf("Submit: %v", err)
		}
		if r.State != StateSubmitted {
			t.Errorf("State = %q, want %q", r.State, StateSubmitted)
		}
		if r.SubmittedBy != "master" || r.SubmittedAt.IsZero() {
			t.Errorf("SubmittedBy/At = %q/%v, want master/non-zero", r.SubmittedBy, r.SubmittedAt)
		}
		if event.Type != EventSubmitted || event.Actor != "master" {
			t.Errorf("event = %+v, want type=submitted actor=master", event)
		}
	})

	for _, from := range []State{StateDraft, StateSubmitted, StateInvalidated} {
		t.Run("from "+string(from)+" fails", func(t *testing.T) {
			r := newTestReport(t, from)
			if _, err := r.Submit("master"); err == nil {
				t.Fatalf("Submit from %q: got nil error, want an error", from)
			}
			if r.State != from {
				t.Errorf("State = %q after a failed Submit, want unchanged %q", r.State, from)
			}
		})
	}
}

func TestReport_Submit_ResubmitBranch(t *testing.T) {
	t.Run("v1 emits EventSubmitted", func(t *testing.T) {
		r := newTestReport(t, StateReady)
		event, err := r.Submit("master")
		if err != nil {
			t.Fatalf("Submit: %v", err)
		}
		if event.Type != EventSubmitted {
			t.Errorf("event.Type = %q, want %q", event.Type, EventSubmitted)
		}
	})

	t.Run("v2+ emits EventResubmitted", func(t *testing.T) {
		r := newTestReport(t, StateReady)
		r.VersionNo = 2
		event, err := r.Submit("master")
		if err != nil {
			t.Fatalf("Submit: %v", err)
		}
		if event.Type != EventResubmitted {
			t.Errorf("event.Type = %q, want %q", event.Type, EventResubmitted)
		}
		if r.State != StateSubmitted {
			t.Errorf("State = %q, want %q", r.State, StateSubmitted)
		}
	})
}

func TestReport_MarkSynced(t *testing.T) {
	t.Run("from submitted succeeds", func(t *testing.T) {
		r := newTestReport(t, StateSubmitted)
		at := time.Now().UTC()
		event, ok := r.MarkSynced(at)
		if !ok {
			t.Fatal("MarkSynced from submitted: got ok=false, want true")
		}
		if r.State != StateSynced {
			t.Errorf("State = %q, want %q", r.State, StateSynced)
		}
		if event.Type != EventSynced {
			t.Errorf("event.Type = %q, want %q", event.Type, EventSynced)
		}
	})

	// A report office has already gone on to remark/invalidate, or a
	// version already synced/pushed, has nothing new for MarkSynced to
	// record — ok=false, not an error, so the caller (pushOutboxBatch's
	// ack loop) can just skip it.
	for _, from := range []State{StateDraft, StateReady, StateSynced, StatePushed, StateRemarked, StateInvalidated} {
		t.Run("from "+string(from)+" is a no-op", func(t *testing.T) {
			r := newTestReport(t, from)
			_, ok := r.MarkSynced(time.Now().UTC())
			if ok {
				t.Fatalf("MarkSynced from %q: got ok=true, want false", from)
			}
			if r.State != from {
				t.Errorf("State = %q after a no-op MarkSynced, want unchanged %q", r.State, from)
			}
		})
	}
}

func TestReport_MarkRemarked(t *testing.T) {
	t.Run("from submitted succeeds", func(t *testing.T) {
		r := newTestReport(t, StateSubmitted)
		event, err := r.MarkRemarked([]string{"Cargo_MT"}, "reviewer1", time.Now())
		if err != nil {
			t.Fatalf("MarkRemarked: %v", err)
		}
		if r.State != StateRemarked {
			t.Errorf("State = %q, want %q", r.State, StateRemarked)
		}
		if event.Type != EventRemarked || event.Actor != "reviewer1" {
			t.Errorf("event = %+v, want type=remarked actor=reviewer1", event)
		}
	})

	t.Run("from draft fails", func(t *testing.T) {
		r := newTestReport(t, StateDraft)
		if _, err := r.MarkRemarked([]string{"Cargo_MT"}, "reviewer1", time.Now()); err == nil {
			t.Fatal("MarkRemarked from draft: got nil error, want an error")
		}
		if r.State != StateDraft {
			t.Errorf("State = %q after a failed MarkRemarked, want unchanged %q", r.State, StateDraft)
		}
	})

	t.Run("repeat remark on an already-remarked report is idempotent-safe", func(t *testing.T) {
		r := newTestReport(t, StateRemarked)
		event, err := r.MarkRemarked([]string{"Cargo_MT"}, "reviewer1", time.Now())
		if err != nil {
			t.Fatalf("MarkRemarked: %v", err)
		}
		if r.State != StateRemarked {
			t.Errorf("State = %q, want %q", r.State, StateRemarked)
		}
		if event.Type != EventRemarked {
			t.Errorf("event.Type = %q, want %q", event.Type, EventRemarked)
		}
	})
}

func TestReport_Invalidate(t *testing.T) {
	r := newTestReport(t, StateSubmitted)
	event := r.Invalidate([]string{"continuity.robContinuity"}, time.Now())
	if r.State != StateInvalidated {
		t.Fatalf("State = %q, want %q", r.State, StateInvalidated)
	}
	if r.InvalidatedFrom != StateSubmitted {
		t.Errorf("InvalidatedFrom = %q, want %q", r.InvalidatedFrom, StateSubmitted)
	}
	if event.Type != EventInvalidated {
		t.Errorf("event.Type = %q, want %q", event.Type, EventInvalidated)
	}

	// Invalidating an already-invalidated report must not overwrite
	// InvalidatedFrom with StateInvalidated itself.
	r.Invalidate([]string{"continuity.timeChain"}, time.Now())
	if r.InvalidatedFrom != StateSubmitted {
		t.Errorf("InvalidatedFrom after a second Invalidate = %q, want unchanged %q", r.InvalidatedFrom, StateSubmitted)
	}
	if len(r.InvalidatedRules) != 1 || r.InvalidatedRules[0] != "continuity.timeChain" {
		t.Errorf("InvalidatedRules = %v, want the latest broken rules only", r.InvalidatedRules)
	}
}

func TestReport_NewCorrection(t *testing.T) {
	for _, from := range []State{StateSubmitted, StateSynced, StatePushed, StateRemarked, StateInvalidated} {
		t.Run("from "+string(from)+" succeeds", func(t *testing.T) {
			r := newTestReport(t, from)
			next, event, err := r.NewCorrection("2/O")
			if err != nil {
				t.Fatalf("NewCorrection: %v", err)
			}
			if next.ReportID != r.ReportID {
				t.Errorf("next.ReportID = %q, want %q (same report)", next.ReportID, r.ReportID)
			}
			if next.VersionNo != r.VersionNo+1 {
				t.Errorf("next.VersionNo = %d, want %d", next.VersionNo, r.VersionNo+1)
			}
			if next.State != StateDraft {
				t.Errorf("next.State = %q, want %q", next.State, StateDraft)
			}
			if event.Type != EventCorrectionStarted || event.VersionNo != r.VersionNo {
				t.Errorf("event = %+v, want type=correction_started versionNo=%d", event, r.VersionNo)
			}

			// Fields must be an independent copy.
			next.Fields["IMO"] = 42.0
			if r.Fields["IMO"] == 42.0 {
				t.Errorf("mutating next.Fields affected the original report's Fields (copy broken)")
			}
		})
	}

	for _, from := range []State{StateDraft, StateReady} {
		t.Run("from "+string(from)+" fails", func(t *testing.T) {
			r := newTestReport(t, from)
			if _, _, err := r.NewCorrection("2/O"); err == nil {
				t.Fatalf("NewCorrection from %q: got nil error, want an error", from)
			}
		})
	}
}
