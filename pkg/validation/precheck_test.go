// SPDX-License-Identifier: AGPL-3.0-only

package validation

import (
	"testing"
	"time"
)

// TestEvaluateContinuity_ReportsOnlyTheCandidatesOwnBreaks checks the
// pre-submit half of continuity checking: a draft being edited is
// measured against the committed chain it would join, and only its own
// findings come back — the chain's other reports are cascade's business,
// not the health check's.
func TestEvaluateContinuity_ReportsOnlyTheCandidatesOwnBreaks(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ROBSeriesList = []ROBSeries{{Name: "hfo", ROBField: "HFO_ROB", ConsumptionFields: []string{"ME_Consumption_HFO"}}}
	base := mustTime(t, "2026-07-01 00:00")

	chain := []*Report{
		{ReportID: "r1", EventType: "Departure", EventTime: base, Fields: map[string]any{"HFO_ROB": 100.0}},
	}
	// 100 - 10 = 90 expected, but the draft says 80.
	draft := &Report{
		ReportID: "draft", EventType: "Noon (Position) - Sea passage", EventTime: base.Add(12 * time.Hour),
		Fields: map[string]any{"Time_Since_Previous_Report": 12.0, "HFO_ROB": 80.0, "ME_Consumption_HFO": 10.0},
	}

	findings := EvaluateContinuity(draft, chain, cfg)
	if len(findings.ByRule(RuleROBContinuity)) == 0 {
		t.Fatalf("no ROB continuity finding for the draft, got %+v", findings)
	}
}

// TestEvaluateContinuity_HealthyDraftIsClean is the companion negative
// case: a draft that continues the chain correctly produces nothing.
func TestEvaluateContinuity_HealthyDraftIsClean(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ROBSeriesList = []ROBSeries{{Name: "hfo", ROBField: "HFO_ROB", ConsumptionFields: []string{"ME_Consumption_HFO"}}}
	base := mustTime(t, "2026-07-01 00:00")

	chain := []*Report{
		{ReportID: "r1", EventType: "Departure", EventTime: base, Fields: map[string]any{"HFO_ROB": 100.0}},
	}
	draft := &Report{
		ReportID: "draft", EventType: "Noon (Position) - Sea passage", EventTime: base.Add(12 * time.Hour),
		Fields: map[string]any{"Time_Since_Previous_Report": 12.0, "HFO_ROB": 90.0, "ME_Consumption_HFO": 10.0},
	}
	if findings := EvaluateContinuity(draft, chain, cfg); len(findings) != 0 {
		t.Fatalf("healthy draft produced findings: %+v", findings)
	}
}

// TestEvaluateContinuity_TimestampCollisionIsCaughtBeforeSubmit is the
// case that used to be discovered only after the fact, by cascade
// invalidating both reports. Caught pre-submit it is an error-severity
// finding, which MarkReady refuses to move past — so the collision never
// enters the chain in the first place.
func TestEvaluateContinuity_TimestampCollisionIsCaughtBeforeSubmit(t *testing.T) {
	cfg := DefaultConfig()
	base := mustTime(t, "2026-07-01 12:00")
	chain := []*Report{{ReportID: "r1", EventTime: base}}
	draft := &Report{ReportID: "draft", EventTime: base.Add(10 * time.Second)}

	findings := EvaluateContinuity(draft, chain, cfg)
	collisions := findings.ByRule(RuleTimestampUniqueness)
	if len(collisions) == 0 {
		t.Fatalf("no timestamp uniqueness finding, got %+v", findings)
	}
	if !collisions.HasErrors() {
		t.Errorf("timestamp collision severity = %q, want error so it blocks submit", collisions[0].Severity)
	}
}

// TestEvaluateContinuity_IgnoresTheCandidatesOwnPriorVersion guards the
// self-comparison trap: a correction re-enters the chain under the same
// ReportID as the version already in it, and must be checked against its
// neighbors, not against itself.
func TestEvaluateContinuity_IgnoresTheCandidatesOwnPriorVersion(t *testing.T) {
	cfg := DefaultConfig()
	base := mustTime(t, "2026-07-01 12:00")
	chain := []*Report{
		{ReportID: "r1", VersionNo: 1, EventTime: base},
		{ReportID: "r2", VersionNo: 1, EventTime: base.Add(12 * time.Hour)},
	}
	correction := &Report{ReportID: "r2", VersionNo: 2, EventTime: base.Add(12 * time.Hour)}

	if findings := EvaluateContinuity(correction, chain, cfg); len(findings) != 0 {
		t.Fatalf("correction collided with its own prior version: %+v", findings)
	}
}
