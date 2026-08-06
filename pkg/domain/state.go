// SPDX-License-Identifier: AGPL-3.0-only

package domain

// State is a report version's position in the lifecycle (architecture
// 8.1). A report is in exactly one State at a time; Invalidated and
// Remarked replace whatever state a report was previously in (the
// pre-invalidation state is kept on Report.InvalidatedFrom so the audit
// trail and UI don't lose it) rather than being separate overlay flags —
// the architecture doc's own lifecycle diagram is not fully explicit on
// this point, so this is a documented default, not a literal spec
// transcription.
type State string

const (
	// StateDraft: being edited. Sections independently editable.
	StateDraft State = "draft"
	// StateReady: health check passes (no errors). Warnings allowed.
	StateReady State = "ready"
	// StateSubmitted: a submit-permitted user pressed Submit. Locked on
	// the vessel and queued in the outbox.
	StateSubmitted State = "submitted"
	// StateSynced: office confirmed receipt. Set by Phase 4 sync code;
	// no Report method sets it yet.
	StateSynced State = "synced"
	// StatePushed: reserved for a future "handed off to an external
	// consumer" concept. Originally meant "included in a Veracity batch
	// with a success response" — that integration is cancelled (DNV
	// declined API access), and the new pull-based data API (external
	// customers query via API key + GraphQL/CSV, nothing is pushed to
	// them) doesn't naturally produce a one-time state transition the
	// way a batch push did. Currently vestigial: no Report method sets
	// it, and no immediate replacement semantics have been decided —
	// left in place rather than removed outright since deleting a
	// lifecycle state ripples into every frontend State union/label map,
	// and nothing forces that decision right now.
	StatePushed State = "pushed"
	// StateRemarked: office Reviewer flagged fields with comments. Set
	// by Phase 5 review-loop code; no Report method sets it yet.
	StateRemarked State = "remarked"
	// StateInvalidated: cascade revalidation found this report now
	// violates a continuity rule because an earlier report changed.
	// Only ever reachable from a committed state (see InChain) — a
	// report still being drafted is never invalidated.
	StateInvalidated State = "invalidated"
)

// InChain reports whether s is a committed state: the report has been
// submitted, so it is part of the official report chain that cascade
// revalidation (architecture 8.3) operates on and that the office holds
// a row for.
//
// Draft and ready are excluded, and that exclusion is the whole point of
// this helper. A draft is incomplete by definition — half its ROB and
// time fields aren't entered yet — so measuring it against its neighbors
// produces breaks that mean nothing, and flipping it to
// StateInvalidated locks the officer out of the report they are in the
// middle of writing. It also made the two engines structurally unable to
// agree: the office only ever receives submitted-and-later versions, so
// a vessel chain including drafts computed a different cascade result
// from the same data, which CLAUDE.md's "identical results on vessel and
// office" rule forbids. Continuity feedback for a draft comes from
// validation.EvaluateContinuity in the health check instead.
func (s State) InChain() bool {
	return s != StateDraft && s != StateReady
}
