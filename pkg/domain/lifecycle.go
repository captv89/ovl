// SPDX-License-Identifier: AGPL-3.0-only

package domain

import (
	"fmt"
	"maps"
	"time"

	"github.com/captv89/ovl/pkg/validation"
)

// MarkReady records a health check result (architecture 14) and, if
// findings has no error-severity entries, transitions r to StateReady.
// Callable from StateDraft or StateReady (re-checking after further
// edits is normal — see SaveSection). If findings has errors, r moves
// to (or stays at) StateDraft and a non-nil error is returned; the
// event is still valid and should still be persisted by the caller.
func (r *Report) MarkReady(findings validation.Findings) (Event, error) {
	if r.State != StateDraft && r.State != StateReady {
		return Event{}, fmt.Errorf("cannot run health check from state %q", r.State)
	}
	var errs, warnings, infos int
	for _, f := range findings {
		switch f.Severity {
		case validation.SeverityError:
			errs++
		case validation.SeverityWarning:
			warnings++
		default:
			infos++
		}
	}
	now := time.Now().UTC()
	r.UpdatedAt = now
	event := Event{
		ReportID:  r.ReportID,
		VersionNo: r.VersionNo,
		Type:      EventHealthCheckResult,
		At:        now,
		Detail:    map[string]any{"errors": errs, "warnings": warnings, "info": infos},
	}
	if errs > 0 {
		r.State = StateDraft
		return event, fmt.Errorf("health check has %d error(s), cannot mark ready", errs)
	}
	r.State = StateReady
	return event, nil
}

// Submit transitions r from StateReady to StateSubmitted. Submit
// permission (architecture 9.3: Master always, plus any user flagged
// canSubmit) is an authorization check the caller must make before
// calling Submit — this method only enforces the lifecycle
// precondition, not who is allowed to invoke it.
//
// A version beyond the first (VersionNo > 1, i.e. this is a
// correction) emits EventResubmitted instead of EventSubmitted — this
// matches the frontend's existing resubmitted-badge heuristic
// (versionNo > 1) rather than requiring a narrower "resubmitted only
// after a remark" signal the domain doesn't track.
func (r *Report) Submit(actor string) (Event, error) {
	if r.State != StateReady {
		return Event{}, fmt.Errorf("cannot submit from state %q, must be %q", r.State, StateReady)
	}
	eventType := EventSubmitted
	if r.VersionNo > 1 {
		eventType = EventResubmitted
	}
	now := time.Now().UTC()
	r.State = StateSubmitted
	r.SubmittedAt = now
	r.SubmittedBy = actor
	r.UpdatedAt = now
	return Event{
		ReportID:  r.ReportID,
		VersionNo: r.VersionNo,
		Type:      eventType,
		At:        now,
		Actor:     actor,
	}, nil
}

// MarkSynced flips r to StateSynced because office confirmed receipt of
// this exact version (vessel/httpapi's pushOutboxBatch, on a real
// PushOutbox ack — architecture 11.2). Valid only from StateSubmitted:
// a report office has already gone on to remark or invalidate no longer
// means "synced" the same way, and a version already synced or pushed
// has nothing new to record — ok is false (not an error) in either case
// so the caller can skip the transition without treating it as a
// failure. 18.07.26 manual-test item 2: StateSynced was defined and
// even had a matching EventSynced/AUDIT_EVENT_TYPE_SYNCED already wired
// into pkg/syncproto's conversion table, but no Report method ever set
// it — a report stayed "Submitted" in the vessel's own UI forever, even
// after office had it.
func (r *Report) MarkSynced(at time.Time) (Event, bool) {
	if r.State != StateSubmitted {
		return Event{}, false
	}
	r.State = StateSynced
	r.UpdatedAt = at
	return Event{
		ReportID:  r.ReportID,
		VersionNo: r.VersionNo,
		Type:      EventSynced,
		At:        at,
	}, true
}

// MarkRemarked flips r to StateRemarked because an office Reviewer
// flagged one or more fields with comments (architecture 12.3). Valid
// only from a locked/submitted-or-later state — mirrors NewCorrection's
// state guard, since a report still in StateDraft or StateReady hasn't
// been submitted for review yet. Idempotent-safe: remarking an
// already-remarked report (e.g. a second remark set before the vessel
// has corrected the first) just re-emits the event without erroring.
// at is the moment the remark set was actually authored — the caller's
// own "now" where this event truly originates. Office passes its local
// time.Now() (matching the timestamp it already stamps the Remark
// records with); the vessel's inbox-pull path, which re-derives this
// same transition locally from a synced Remark, passes that Remark's
// own CreatedAt instead of its own pull-time now — otherwise the
// vessel's audit trail would show whenever it happened to sync next,
// not when the reviewer actually acted (manual-test review item 9).
func (r *Report) MarkRemarked(fields []string, actor string, at time.Time) (Event, error) {
	switch r.State {
	case StateDraft, StateReady:
		return Event{}, fmt.Errorf("cannot remark a report in state %q; it must be submitted first", r.State)
	}
	r.State = StateRemarked
	r.UpdatedAt = at
	return Event{
		ReportID:  r.ReportID,
		VersionNo: r.VersionNo,
		Type:      EventRemarked,
		At:        at,
		Actor:     actor,
		Detail:    map[string]any{"fields": fields},
	}, nil
}

// Invalidate flips r to StateInvalidated because cascade revalidation
// (architecture 8.3) found it now violates a continuity rule, carrying
// the broken rule IDs. It is valid from any state — an earlier report's
// correction can invalidate a report that is draft, submitted, synced,
// or already pushed — and is idempotent: invalidating an already-
// invalidated report just replaces InvalidatedRules, without stacking a
// second InvalidatedFrom. at is the moment cascade revalidation actually
// ran and computed this — see MarkRemarked's own doc comment for why
// this is a parameter rather than an internal time.Now() call.
func (r *Report) Invalidate(brokenRules []string, at time.Time) Event {
	if r.State != StateInvalidated {
		r.InvalidatedFrom = r.State
	}
	r.State = StateInvalidated
	r.InvalidatedRules = brokenRules
	r.UpdatedAt = at
	return Event{
		ReportID:  r.ReportID,
		VersionNo: r.VersionNo,
		Type:      EventInvalidated,
		At:        at,
		Detail:    map[string]any{"brokenRules": brokenRules, "fromState": r.InvalidatedFrom},
	}
}

// NewCorrection starts version r.VersionNo+1 in StateDraft, seeded with
// r's current field values (architecture 8.2: "Corrections re-enter the
// flow at draft"). Only valid once r is locked post-submit — a report
// still in StateDraft or StateReady should just keep being edited
// directly via SaveSection, not corrected.
func (r *Report) NewCorrection(actor string) (*Report, Event, error) {
	switch r.State {
	case StateDraft, StateReady:
		return nil, Event{}, fmt.Errorf("cannot correct a report in state %q; edit it directly instead", r.State)
	}
	now := time.Now().UTC()
	next := &Report{
		ReportID:   r.ReportID,
		VersionNo:  r.VersionNo + 1,
		SchemaName: r.SchemaName,
		EventType:  r.EventType,
		EventTime:  r.EventTime,
		Fields:     maps.Clone(r.Fields),
		State:      StateDraft,
		CreatedAt:  now,
		CreatedBy:  actor,
		UpdatedAt:  now,
	}
	event := Event{
		ReportID:  r.ReportID,
		VersionNo: r.VersionNo,
		Type:      EventCorrectionStarted,
		At:        now,
		Actor:     actor,
		Detail:    map[string]any{"newVersionNo": next.VersionNo},
	}
	return next, event, nil
}
