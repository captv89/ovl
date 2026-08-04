// SPDX-License-Identifier: AGPL-3.0-only

package validation

import (
	"fmt"
	"math"
	"slices"
)

// Continuity rule IDs — the subset of plausibility rules that span
// consecutive reports (architecture 8.3) and drive cascade
// revalidation.
const (
	RuleTimeChain           = "continuity.timeChain"
	RuleROBContinuity       = "continuity.robContinuity"
	RuleEventOrdering       = "continuity.eventOrdering"
	RuleTimestampUniqueness = "continuity.timestampUniqueness"
)

// EvaluateTimeChain checks that Time_Since_Previous_Report on r matches
// the actual timestamp delta to prev (architecture 8.3). prev may be
// nil for the first report in a chain, in which case there is nothing
// to check.
func EvaluateTimeChain(r *Report, prev *Report, cfg *Config) Findings {
	if prev == nil {
		return nil
	}
	tsp, ok := r.Float("Time_Since_Previous_Report")
	if !ok {
		return nil
	}
	actual := r.EventTime.Sub(prev.EventTime).Hours()
	if math.Abs(tsp-actual) > cfg.TimeChainToleranceHours {
		return Findings{{
			RuleID:   RuleTimeChain,
			Severity: cfg.severity(RuleTimeChain, SeverityWarning),
			Field:    "Time_Since_Previous_Report",
			Message:  fmt.Sprintf("Time_Since_Previous_Report is %.2fh, but %.2fh elapsed since the previous report", tsp, actual),
		}}
	}
	return nil
}

// EvaluateROBContinuity checks ROB(n) = ROB(n-1) - consumption(n) +
// bunkered(n) for every series in cfg.ROBSeriesList (architecture 8.3).
// prev may be nil, in which case there is nothing to check (the first
// report in a chain has no prior ROB to continue from).
func EvaluateROBContinuity(r *Report, prev *Report, cfg *Config) Findings {
	if prev == nil {
		return nil
	}
	var findings Findings
	for _, s := range cfg.ROBSeriesList {
		cur, curOk := r.Float(s.ROBField)
		prior, priorOk := prev.Float(s.ROBField)
		if !curOk || !priorOk {
			continue
		}
		var consumption float64
		for _, f := range s.ConsumptionFields {
			v, _ := r.Float(f)
			consumption += v
		}
		bunkered := cfg.BunkeredAmounts[s.Name]
		expected := prior - consumption + bunkered
		if math.Abs(cur-expected) > cfg.ROBToleranceMt {
			findings = append(findings, Finding{
				RuleID:   RuleROBContinuity,
				Severity: cfg.severity(RuleROBContinuity, SeverityWarning),
				Field:    s.ROBField,
				Message:  fmt.Sprintf("%s is %.2f, expected %.2f (previous %.2f - consumption %.2f + bunkered %.2f)", s.ROBField, cur, expected, prior, consumption, bunkered),
			})
		}
	}
	return findings
}

// EvaluateTimestampUniqueness checks that no two reports in chain share
// the same UTC timestamp at minute resolution (architecture 8.3, 10.1).
// It returns findings per offending report, keyed by ReportID; chain
// need not be sorted.
func EvaluateTimestampUniqueness(chain []*Report, cfg *Config) map[string]Findings {
	byMinute := make(map[int64][]*Report)
	for _, r := range chain {
		key := r.EventTime.Unix() / 60
		byMinute[key] = append(byMinute[key], r)
	}
	out := make(map[string]Findings)
	for _, reports := range byMinute {
		if len(reports) < 2 {
			continue
		}
		for _, r := range reports {
			out[r.ReportID] = append(out[r.ReportID], Finding{
				RuleID:   RuleTimestampUniqueness,
				Severity: cfg.severity(RuleTimestampUniqueness, SeverityError),
				Message:  fmt.Sprintf("another report shares the same timestamp %s at minute resolution", r.EventTime.Format(dateTimeLayout)),
			})
		}
	}
	return out
}

// EvaluateEventOrdering checks that events in chain alternate correctly
// within each of cfg.EventGroups (architecture 10.1: "Arrival cannot
// follow Arrival without a Departure", and similar pairs). chain must be
// ordered by EventTime. It returns findings per offending report, keyed
// by ReportID.
func EvaluateEventOrdering(chain []*Report, cfg *Config) map[string]Findings {
	out := make(map[string]Findings)
	for _, g := range cfg.EventGroups {
		stageOf := func(eventType string) int {
			if slices.Contains(g.Stage0, eventType) {
				return 0
			}
			if slices.Contains(g.Stage1, eventType) {
				return 1
			}
			return -1
		}
		lastStage := -1
		for _, r := range chain {
			stage := stageOf(r.EventType)
			if stage == -1 {
				continue
			}
			if lastStage == stage {
				out[r.ReportID] = append(out[r.ReportID], Finding{
					RuleID:   RuleEventOrdering,
					Severity: cfg.severity(RuleEventOrdering, SeverityWarning),
					Message:  fmt.Sprintf("%q cannot follow another event in the same %q sequence without alternating", r.EventType, g.Name),
				})
			}
			lastStage = stage
		}
	}
	return out
}
