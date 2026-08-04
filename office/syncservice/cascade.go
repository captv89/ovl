// SPDX-License-Identifier: AGPL-3.0-only

package syncservice

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/captv89/ovl/office/compliance"
	"github.com/captv89/ovl/office/store"
	"github.com/captv89/ovl/pkg/domain"
	"github.com/captv89/ovl/pkg/validation"
)

// runCascade re-checks every report in (vesselID, schemaName)'s chain
// against the continuity rules (architecture 8.3) after a report
// version lands, near-verbatim of vessel/httpapi.Server.runCascade —
// resolves Phase 5 open question 5: cascade runs synchronously right
// after landing, mirroring the vessel's own immediate-after-write
// pattern, since Revalidate is cheap and pure and a vessel's chain is
// short. Any newly (or differently) broken report is flipped to
// StateInvalidated, gets an audit event, and gets an invalidation_notices
// row for the vessel to later pull (Slice S4).
func runCascade(ctx context.Context, st *store.Store, vesselID, schemaName string) error {
	all, err := st.ListChain(ctx, vesselID, schemaName)
	if err != nil {
		return fmt.Errorf("list chain for cascade: %w", err)
	}
	// Committed versions only (domain.State.InChain), mirroring the
	// vessel's own filter. The office should never hold a draft in the
	// first place — nothing is enqueued before submit — so this is a
	// guard, not a behavior change here; it is written out rather than
	// assumed because the two sides must compute cascade over the same
	// chain to satisfy CLAUDE.md's "identical results on vessel and
	// office", and the vessel's chain does contain drafts.
	chain := make([]*domain.Report, 0, len(all))
	for _, r := range all {
		if r.State.InChain() {
			chain = append(chain, r)
		}
	}
	vchain := make([]*validation.Report, len(chain))
	for i, r := range chain {
		vchain[i] = r.ToValidation()
	}
	vessel, err := st.GetVessel(ctx, vesselID)
	if err != nil {
		return fmt.Errorf("load vessel %s for cascade: %w", vesselID, err)
	}
	cfg, err := ValidationConfigForVessel(ctx, st, schemaName, vesselID, vessel.Groups)
	if err != nil {
		return fmt.Errorf("resolve validation config for cascade: %w", err)
	}
	result := validation.Revalidate(vchain, cfg)
	now := time.Now().UTC()

	for _, report := range chain {
		rules, invalidated := result.Invalidated[report.ReportID]
		if !invalidated {
			continue
		}
		if report.State == domain.StateInvalidated {
			prev, err := st.GetLatestInvalidationNotice(ctx, vesselID, report.ReportID, report.VersionNo)
			if err != nil && !errors.Is(err, store.ErrNotFound) {
				return fmt.Errorf("check previous invalidation notice for %s: %w", report.ReportID, err)
			}
			if err == nil && stringsEqual(prev.BrokenRules, rules) {
				continue // already recorded with the same broken rules
			}
		}
		event := report.Invalidate(rules, now)
		if err := st.UpdateReportVersionState(ctx, vesselID, report.ReportID, report.VersionNo, report.State); err != nil {
			return fmt.Errorf("persist invalidation for %s: %w", report.ReportID, err)
		}
		if err := st.AppendReportAuditEvent(ctx, vesselID, event, now, "office"); err != nil {
			return fmt.Errorf("append invalidation event for %s: %w", report.ReportID, err)
		}
		if err := st.InsertInvalidationNotice(ctx, vesselID, report.ReportID, report.VersionNo, rules, now); err != nil {
			return fmt.Errorf("insert invalidation notice for %s: %w", report.ReportID, err)
		}
	}
	return nil
}

// ValidationConfigFor mirrors vessel/httpapi's own function of the same
// name (schemas.go); both now delegate to validation.LogAbstractConfig,
// the single shared curated config, rather than each hand-maintaining
// their own copy (a real correctness bug when they were, see
// LogAbstractConfig's own doc comment — vessel and office silently
// agreeing on the same too-narrow HFO-only list is exactly the kind of
// drift CLAUDE.md's "the engine is the source of truth, and it must
// produce identical results on vessel and office" rule exists to
// prevent). Exported (was package-private through Phase 5) so office/
// httpapi's B3 health cell (errors/warnings per report, Office UI rework
// Phase O3) can reuse the exact same config cascade already revalidates
// the chain with, rather than building a second copy.
func ValidationConfigFor(schemaName string) *validation.Config {
	if schemaName != "log-abstract" {
		return validation.DefaultConfig()
	}
	return validation.LogAbstractConfig()
}

// ValidationConfigForVessel returns ValidationConfigFor(schemaName) with
// the company's rule-severity overrides for this specific vessel layered
// on (compliance.EffectiveSeverities over the fleet/group/vessel
// rule_severity_assignments). This is what wires the previously-dead
// EffectiveSeverities into office's own evaluation (codebase audit
// 2026-07-22 §2): before this, office cascade revalidation and B3's health
// cell both ran against the hardcoded defaults, so the entire B7 "Rule
// severities" screen persisted rows nothing read. The curated series and
// tolerances (ValidationConfigFor) are unchanged — only per-rule severity
// is company-configurable (architecture 10.2).
func ValidationConfigForVessel(ctx context.Context, st *store.Store, schemaName, vesselID string, vesselGroups []string) (*validation.Config, error) {
	cfg := ValidationConfigFor(schemaName)
	assignments, err := st.ListRuleSeverityAssignments(ctx)
	if err != nil {
		return nil, fmt.Errorf("list rule severity assignments: %w", err)
	}
	cfg.Severities = compliance.EffectiveSeverities(assignments, vesselID, vesselGroups)
	return cfg, nil
}

// stringsEqual reports whether a and b contain the same rule IDs in the
// same order — cascade's own dedup guard.
func stringsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
