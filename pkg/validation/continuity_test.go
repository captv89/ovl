// SPDX-License-Identifier: AGPL-3.0-only

package validation

import (
	"testing"
	"time"
)

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.Parse(dateTimeLayout, s)
	if err != nil {
		t.Fatalf("parse time %q: %v", s, err)
	}
	return tm
}

func TestEvaluateTimeChain(t *testing.T) {
	cfg := DefaultConfig()
	prev := &Report{ReportID: "prev", EventTime: mustTime(t, "2026-07-04 00:00")}

	t.Run("matches the actual elapsed time", func(t *testing.T) {
		r := &Report{ReportID: "cur", EventTime: mustTime(t, "2026-07-04 12:00"), Fields: map[string]any{"Time_Since_Previous_Report": 12.0}}
		if findings := EvaluateTimeChain(r, prev, cfg); len(findings) != 0 {
			t.Errorf("findings = %+v, want none", findings)
		}
	})

	t.Run("drifts from the actual elapsed time", func(t *testing.T) {
		r := &Report{ReportID: "cur", EventTime: mustTime(t, "2026-07-04 12:00"), Fields: map[string]any{"Time_Since_Previous_Report": 6.0}}
		findings := EvaluateTimeChain(r, prev, cfg)
		if len(findings) == 0 {
			t.Fatal("findings is empty, want a drift finding")
		}
	})

	t.Run("no previous report skips the check", func(t *testing.T) {
		r := &Report{ReportID: "cur", EventTime: mustTime(t, "2026-07-04 12:00"), Fields: map[string]any{"Time_Since_Previous_Report": 6.0}}
		if findings := EvaluateTimeChain(r, nil, cfg); len(findings) != 0 {
			t.Errorf("findings = %+v, want none", findings)
		}
	})
}

func TestEvaluateROBContinuity(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ROBSeriesList = []ROBSeries{{Name: "hfo", ROBField: "HFO_ROB", ConsumptionFields: []string{"ME_Consumption_HFO"}}}
	cfg.BunkeredAmounts = map[string]float64{"hfo": 50.0}

	prev := &Report{ReportID: "prev", Fields: map[string]any{"HFO_ROB": 100.0}}

	t.Run("matches expected ROB", func(t *testing.T) {
		r := &Report{ReportID: "cur", Fields: map[string]any{"HFO_ROB": 130.0, "ME_Consumption_HFO": 20.0}} // 100-20+50
		if findings := EvaluateROBContinuity(r, prev, cfg); len(findings) != 0 {
			t.Errorf("findings = %+v, want none", findings)
		}
	})

	t.Run("breaks ROB continuity", func(t *testing.T) {
		r := &Report{ReportID: "cur", Fields: map[string]any{"HFO_ROB": 500.0, "ME_Consumption_HFO": 20.0}}
		findings := EvaluateROBContinuity(r, prev, cfg)
		if len(findings) == 0 {
			t.Fatal("findings is empty, want a continuity break")
		}
	})
}

// TestEvaluateROBContinuity_SumsMultipleConsumptionFields is the
// regression test for 18.07.26 manual-test triage ("why limit the auto
// calculation to just these 2 fields... peripheral vision"): a single ROB
// tank is drawn down by more than one consumer on a real vessel (Main
// Engine and Auxiliary Engine both burn HFO from the same tank). Checking
// only one consumption field made continuity too lenient — it would have
// silently accepted the "breaks ROB continuity" case below as valid,
// since it never subtracted AE's share.
func TestEvaluateROBContinuity_SumsMultipleConsumptionFields(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ROBSeriesList = []ROBSeries{{
		Name:              "hfo",
		ROBField:          "HFO_ROB",
		ConsumptionFields: []string{"ME_Consumption_HFO", "AE_Consumption_HFO"},
	}}
	prev := &Report{ReportID: "prev", Fields: map[string]any{"HFO_ROB": 100.0}}

	t.Run("matches expected ROB when both consumers are summed", func(t *testing.T) {
		r := &Report{ReportID: "cur", Fields: map[string]any{"HFO_ROB": 75.0, "ME_Consumption_HFO": 20.0, "AE_Consumption_HFO": 5.0}} // 100-20-5
		if findings := EvaluateROBContinuity(r, prev, cfg); len(findings) != 0 {
			t.Errorf("findings = %+v, want none", findings)
		}
	})

	t.Run("catches drift a single-field check would have missed", func(t *testing.T) {
		// Only ME_Consumption_HFO is checked against — 100-20=80 "matches"
		// the reported 80 if AE's 5 is ignored, but the real expected value
		// is 75; the single-field version of this check would have missed
		// this entirely.
		r := &Report{ReportID: "cur", Fields: map[string]any{"HFO_ROB": 80.0, "ME_Consumption_HFO": 20.0, "AE_Consumption_HFO": 5.0}}
		findings := EvaluateROBContinuity(r, prev, cfg)
		if len(findings) == 0 {
			t.Fatal("findings is empty, want a continuity break (AE consumption unaccounted for)")
		}
	})
}

func TestEvaluateEventOrdering(t *testing.T) {
	cfg := DefaultConfig()
	base := mustTime(t, "2026-07-01 00:00")

	t.Run("valid Departure/Arrival alternation", func(t *testing.T) {
		chain := []*Report{
			{ReportID: "1", EventType: "Departure", EventTime: base},
			{ReportID: "2", EventType: "Arrival", EventTime: base.Add(24 * time.Hour)},
			{ReportID: "3", EventType: "Departure", EventTime: base.Add(48 * time.Hour)},
		}
		if out := EvaluateEventOrdering(chain, cfg); len(out) != 0 {
			t.Errorf("out = %+v, want none", out)
		}
	})

	t.Run("Arrival following Arrival without a Departure", func(t *testing.T) {
		chain := []*Report{
			{ReportID: "1", EventType: "Departure", EventTime: base},
			{ReportID: "2", EventType: "Arrival", EventTime: base.Add(24 * time.Hour)},
			{ReportID: "3", EventType: "Arrival", EventTime: base.Add(48 * time.Hour)},
		}
		out := EvaluateEventOrdering(chain, cfg)
		if _, ok := out["3"]; !ok {
			t.Fatalf("out = %+v, want report 3 flagged", out)
		}
		if _, ok := out["2"]; ok {
			t.Errorf("out = %+v, want report 2 not flagged", out)
		}
	})

	t.Run("unrelated event types are ignored", func(t *testing.T) {
		chain := []*Report{
			{ReportID: "1", EventType: "Noon (Position) - Sea passage", EventTime: base},
			{ReportID: "2", EventType: "Noon (Position) - Sea passage", EventTime: base.Add(24 * time.Hour)},
		}
		if out := EvaluateEventOrdering(chain, cfg); len(out) != 0 {
			t.Errorf("out = %+v, want none", out)
		}
	})
}

func TestEvaluateTimestampUniqueness(t *testing.T) {
	cfg := DefaultConfig()
	base := mustTime(t, "2026-07-01 12:00")

	t.Run("all unique timestamps", func(t *testing.T) {
		chain := []*Report{
			{ReportID: "1", EventTime: base},
			{ReportID: "2", EventTime: base.Add(time.Hour)},
		}
		if out := EvaluateTimestampUniqueness(chain, cfg); len(out) != 0 {
			t.Errorf("out = %+v, want none", out)
		}
	})

	t.Run("two reports share the same minute", func(t *testing.T) {
		chain := []*Report{
			{ReportID: "1", EventTime: base},
			{ReportID: "2", EventTime: base.Add(30 * time.Second)},
		}
		out := EvaluateTimestampUniqueness(chain, cfg)
		if len(out) != 2 {
			t.Fatalf("out = %+v, want both reports flagged", out)
		}
	})
}
