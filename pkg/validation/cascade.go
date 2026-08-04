// SPDX-License-Identifier: AGPL-3.0-only

package validation

import (
	"slices"
	"sort"
)

// CascadeResult is the outcome of cascade revalidation across a
// vessel's report chain (architecture 8.3): which report versions now
// violate a continuity rule at error severity, and which rules they
// broke. A report present in Invalidated should flip to the
// `invalidated` lifecycle state, carrying the broken rule names.
//
// Only error-severity violations land here. This used to be severity-
// agnostic on the reasoning that "invalidated" is a report-state concept
// distinct from whether a severity blocks submit — but architecture 10.2
// defines `warning` as "shown in the health check and allows submit",
// and `invalidated` locks the report and demands a correction, which is
// the opposite of allowing submit. A fleet that wants ROB or time-chain
// breaks to be chain-breaking configures them as errors in the bundle,
// and then they block submit and invalidate consistently.
type CascadeResult struct {
	Invalidated map[string][]string // reportID -> broken rule IDs, in first-seen order
}

// IsInvalidated reports whether reportID broke any continuity rule.
func (c *CascadeResult) IsInvalidated(reportID string) bool {
	return len(c.Invalidated[reportID]) > 0
}

// Revalidate runs the continuity rules (architecture 8.3: time chain,
// ROB continuity, event ordering, timestamp uniqueness) across chain
// and reports which report versions are now invalid.
//
// Architecture 8.3 describes revalidating "from [the changed] report
// forward until results stabilize" as an optimization for large
// chains. Every rule here is linear in chain size and depends only on
// a report's immediate neighbor (time chain, ROB continuity) or a
// single pass over the whole chain (event ordering, timestamp
// uniqueness) — none require an iterative fixed-point search — so this
// implementation simply recomputes over the full chain every time
// rather than tracking a "changed from" index. That is fast enough for
// a vessel's realistic report volume; callers with unusually large
// chains can slice chain themselves to bound the work.
func Revalidate(chain []*Report, cfg *Config) *CascadeResult {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	sorted := make([]*Report, len(chain))
	copy(sorted, chain)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].EventTime.Before(sorted[j].EventTime) })

	result := &CascadeResult{Invalidated: map[string][]string{}}
	record := func(reportID, ruleID string) {
		if slices.Contains(result.Invalidated[reportID], ruleID) {
			return
		}
		result.Invalidated[reportID] = append(result.Invalidated[reportID], ruleID)
	}

	var prev *Report
	for _, r := range sorted {
		if EvaluateTimeChain(r, prev, cfg).HasErrors() {
			record(r.ReportID, RuleTimeChain)
		}
		if EvaluateROBContinuity(r, prev, cfg).HasErrors() {
			record(r.ReportID, RuleROBContinuity)
		}
		prev = r
	}
	for reportID, findings := range EvaluateEventOrdering(sorted, cfg) {
		if findings.HasErrors() {
			record(reportID, RuleEventOrdering)
		}
	}
	for reportID, findings := range EvaluateTimestampUniqueness(sorted, cfg) {
		if findings.HasErrors() {
			record(reportID, RuleTimestampUniqueness)
		}
	}
	return result
}
