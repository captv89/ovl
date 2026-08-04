// SPDX-License-Identifier: AGPL-3.0-only

package syncservice

import (
	"context"
	"testing"

	"github.com/captv89/ovl/office/compliance"
	"github.com/captv89/ovl/pkg/validation"
)

// TestValidationConfigForVessel_AppliesEffectiveSeverities proves office's
// own validation config now honors company rule-severity overrides
// (codebase audit 2026-07-22 §2). Scoped to a vessel-level assignment on
// this test's own unique vessel so it is robust against whatever fleet-
// scope assignments other tests or demo data leave in the shared dev DB
// (§8) — a vessel-scoped override always wins for its own vessel, and the
// fleet/precedence behavior itself is unit-tested in
// compliance.EffectiveSeverities' own test.
func TestValidationConfigForVessel_AppliesEffectiveSeverities(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 77)

	vesselScope, _ := compliance.VesselScope(v.ID)
	vesselAssign, _ := compliance.NewRuleSeverityAssignment(vesselScope)
	_ = vesselAssign.SetSeverity(validation.RuleROBContinuity, validation.SeverityError)
	if err := st.SaveRuleSeverityAssignment(ctx, vesselAssign); err != nil {
		t.Fatalf("SaveRuleSeverityAssignment(vessel): %v", err)
	}

	cfg, err := ValidationConfigForVessel(ctx, st, "log-abstract", v.ID, v.Groups)
	if err != nil {
		t.Fatalf("ValidationConfigForVessel: %v", err)
	}
	if cfg.Severities[validation.RuleROBContinuity] != validation.SeverityError {
		t.Errorf("ROB severity = %q, want error (the vessel-scoped override must apply)",
			cfg.Severities[validation.RuleROBContinuity])
	}
	// The curated base config (series + tolerances) is preserved — only
	// severities are overlaid.
	if len(cfg.ROBSeriesList) == 0 {
		t.Error("ValidationConfigForVessel lost the curated ROB series")
	}
}
