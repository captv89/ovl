// SPDX-License-Identifier: AGPL-3.0-only

package validation

import "sort"

// EvaluateContinuity checks one report against the committed chain it
// belongs to (or would join) and returns only that report's own
// findings — the pre-submit half of architecture 8.3's continuity rules,
// where Revalidate is the post-submit half.
//
// The two halves exist because a continuity break has to be reachable
// while the officer can still fix it. Revalidate answers "which reports
// already in the chain does this change break", which is a question
// about committed reports and produces a lifecycle state flip. This
// answers "does the report in my hands continue the chain", which is a
// question about a draft and produces health-check findings the officer
// acts on before submitting. Running only the second half over drafts is
// what keeps a half-entered report from being invalidated and locked
// mid-entry.
//
// chain is the committed chain, in any order; any entry sharing r's
// ReportID is r's own earlier version and is replaced by r rather than
// compared against it (a correction continues the chain in its
// predecessor's slot). Findings carry the same rule IDs and configured
// severities Revalidate gates on, so a rule a fleet made blocking blocks
// here too.
func EvaluateContinuity(r *Report, chain []*Report, cfg *Config) Findings {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	merged := make([]*Report, 0, len(chain)+1)
	for _, other := range chain {
		if other.ReportID == r.ReportID {
			continue
		}
		merged = append(merged, other)
	}
	merged = append(merged, r)
	sort.SliceStable(merged, func(i, j int) bool { return merged[i].EventTime.Before(merged[j].EventTime) })

	var findings Findings
	var prev *Report
	for _, candidate := range merged {
		if candidate.ReportID == r.ReportID {
			findings = append(findings, EvaluateTimeChain(r, prev, cfg)...)
			findings = append(findings, EvaluateROBContinuity(r, prev, cfg)...)
			break
		}
		prev = candidate
	}
	findings = append(findings, EvaluateEventOrdering(merged, cfg)[r.ReportID]...)
	findings = append(findings, EvaluateTimestampUniqueness(merged, cfg)[r.ReportID]...)
	return findings
}
