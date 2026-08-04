// SPDX-License-Identifier: AGPL-3.0-only

package validation

import (
	"testing"
	"time"
)

// TestRevalidate_HealthyChain exercises a synthetic, internally
// consistent report chain: no report should be invalidated.
func TestRevalidate_HealthyChain(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ROBSeriesList = []ROBSeries{{Name: "hfo", ROBField: "HFO_ROB", ConsumptionFields: []string{"ME_Consumption_HFO"}}}
	base := mustTime(t, "2026-07-01 00:00")

	chain := []*Report{
		{
			ReportID: "r1", EventType: "Departure", EventTime: base,
			Fields: map[string]any{"HFO_ROB": 100.0},
		},
		{
			ReportID: "r2", EventType: "Noon (Position) - Sea passage", EventTime: base.Add(12 * time.Hour),
			Fields: map[string]any{"Time_Since_Previous_Report": 12.0, "HFO_ROB": 90.0, "ME_Consumption_HFO": 10.0},
		},
		{
			ReportID: "r3", EventType: "Arrival", EventTime: base.Add(24 * time.Hour),
			Fields: map[string]any{"Time_Since_Previous_Report": 12.0, "HFO_ROB": 80.0, "ME_Consumption_HFO": 10.0},
		},
	}

	result := Revalidate(chain, cfg)
	for _, r := range chain {
		if result.IsInvalidated(r.ReportID) {
			t.Errorf("report %s invalidated unexpectedly: %v", r.ReportID, result.Invalidated[r.ReportID])
		}
	}
}

// TestRevalidate_CorrectionBreaksDownstreamROB simulates the scenario
// architecture 8.3 describes: r1/r2/r3 start out fully ROB-continuous.
// A correction to r2 (its ROB reading is revised from 90 to 95 — e.g.
// the crew re-measured and resubmitted) invalidates r2 itself against
// r1, and cascades forward to invalidate r3 as well, since r3 was
// computed against r2's old ROB value. r1 is untouched.
//
// ROB continuity defaults to warning severity, which no longer
// invalidates (see TestRevalidate_WarningSeverityDoesNotInvalidate), so
// this is the fleet that configured it as an error — the premise the
// scenario needs to be about cascade at all.
func TestRevalidate_CorrectionBreaksDownstreamROB(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Severities = map[string]Severity{RuleROBContinuity: SeverityError}
	cfg.ROBSeriesList = []ROBSeries{{Name: "hfo", ROBField: "HFO_ROB", ConsumptionFields: []string{"ME_Consumption_HFO"}}}
	base := mustTime(t, "2026-07-01 00:00")

	chain := []*Report{
		{ReportID: "r1", EventType: "Departure", EventTime: base, Fields: map[string]any{"HFO_ROB": 100.0}},
		{
			ReportID: "r2", EventType: "Noon (Position) - Sea passage", EventTime: base.Add(12 * time.Hour),
			// Corrected: ROB reading revised from 90 (100-10+0) to 95.
			Fields: map[string]any{"Time_Since_Previous_Report": 12.0, "HFO_ROB": 95.0, "ME_Consumption_HFO": 10.0},
		},
		{
			ReportID: "r3", EventType: "Arrival", EventTime: base.Add(24 * time.Hour),
			// Unchanged: still computed against r2's original ROB of 90.
			Fields: map[string]any{"Time_Since_Previous_Report": 12.0, "HFO_ROB": 80.0, "ME_Consumption_HFO": 10.0},
		},
	}

	result := Revalidate(chain, cfg)

	if result.IsInvalidated("r1") {
		t.Errorf("r1 invalidated unexpectedly: %v", result.Invalidated["r1"])
	}
	for _, id := range []string{"r2", "r3"} {
		if !result.IsInvalidated(id) {
			t.Fatalf("%s was not invalidated, want a ROB continuity break", id)
		}
		found := false
		for _, rule := range result.Invalidated[id] {
			if rule == RuleROBContinuity {
				found = true
			}
		}
		if !found {
			t.Errorf("%s broken rules = %v, want %s among them", id, result.Invalidated[id], RuleROBContinuity)
		}
	}
}

// TestRevalidate_LateReportInsertedBreaksEventOrdering simulates a late
// report inserted between existing timestamps (architecture 8.3) that
// breaks event ordering: two Arrivals in a row once the late report is
// placed in chain order.
// Event ordering defaults to warning severity; as in
// TestRevalidate_CorrectionBreaksDownstreamROB this is the fleet that
// configured it as an error.
func TestRevalidate_LateReportInsertedBreaksEventOrdering(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Severities = map[string]Severity{RuleEventOrdering: SeverityError}
	base := mustTime(t, "2026-07-01 00:00")

	chain := []*Report{
		{ReportID: "r1", EventType: "Departure", EventTime: base},
		{ReportID: "r2", EventType: "Arrival", EventTime: base.Add(48 * time.Hour)},
	}
	result := Revalidate(chain, cfg)
	if result.IsInvalidated("r2") {
		t.Fatalf("r2 invalidated before the late report was inserted: %v", result.Invalidated["r2"])
	}

	// A late report is inserted between r1 and r2, out of submission
	// order but between them in timestamp order.
	late := &Report{ReportID: "late", EventType: "Arrival", EventTime: base.Add(24 * time.Hour)}
	chain = append(chain, late)

	result = Revalidate(chain, cfg)
	if !result.IsInvalidated("r2") {
		t.Fatal("r2 was not invalidated after the late report broke event ordering")
	}
}

func TestRevalidate_TimestampCollision(t *testing.T) {
	cfg := DefaultConfig()
	base := mustTime(t, "2026-07-01 12:00")
	chain := []*Report{
		{ReportID: "r1", EventTime: base},
		{ReportID: "r2", EventTime: base.Add(10 * time.Second)},
	}
	result := Revalidate(chain, cfg)
	for _, id := range []string{"r1", "r2"} {
		if !result.IsInvalidated(id) {
			t.Errorf("report %s was not invalidated, want a timestamp uniqueness break", id)
		}
	}
}

// TestRevalidate_WarningSeverityDoesNotInvalidate pins the severity
// contract from architecture 10.2: a `warning` "is shown in the health
// check and allows submit". A rule the company left at warning therefore
// cannot flip a report to `invalidated` — that state locks the report and
// demands a correction, which is the opposite of "allows submit". Only a
// rule configured (or defaulting) to `error` invalidates.
func TestRevalidate_WarningSeverityDoesNotInvalidate(t *testing.T) {
	base := mustTime(t, "2026-07-01 00:00")
	chain := []*Report{
		{ReportID: "r1", EventType: "Departure", EventTime: base, Fields: map[string]any{"HFO_ROB": 100.0}},
		{
			ReportID: "r2", EventType: "Noon (Position) - Sea passage", EventTime: base.Add(12 * time.Hour),
			Fields: map[string]any{"Time_Since_Previous_Report": 12.0, "HFO_ROB": 80.0, "ME_Consumption_HFO": 10.0},
		},
	}
	robSeries := []ROBSeries{{Name: "hfo", ROBField: "HFO_ROB", ConsumptionFields: []string{"ME_Consumption_HFO"}}}

	warn := DefaultConfig()
	warn.ROBSeriesList = robSeries
	if result := Revalidate(chain, warn); result.IsInvalidated("r2") {
		t.Errorf("r2 invalidated on a warning-severity ROB break: %v", result.Invalidated["r2"])
	}

	strict := DefaultConfig()
	strict.ROBSeriesList = robSeries
	strict.Severities = map[string]Severity{RuleROBContinuity: SeverityError}
	if result := Revalidate(chain, strict); !result.IsInvalidated("r2") {
		t.Error("r2 not invalidated once the ROB rule is configured as an error")
	}
}
