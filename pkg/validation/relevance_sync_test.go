// SPDX-License-Identifier: AGPL-3.0-only

package validation

import "testing"

// TestProfileRelevance_RecognizedStringsSnapshot pins the exact set of
// curated relevance strings profileRelevance recognizes. That set is
// hand-mirrored in two TypeScript files for the vessel/office UIs'
// pre-checks:
//
//	web/vessel/src/screens/report-form/fieldPolicy.ts   (GHG_RELEVANT_STRINGS)
//	web/office/src/screens/configuration/fieldPolicyLogic.ts (GHG_RELEVANT_STRINGS)
//
// CLAUDE.md requires the engine and UI to produce identical results, and
// this list living in three hand-synced places is a real drift hazard
// (codebase audit 2026-07-22 §6). If you add, remove, or rename a
// recognized relevance string in profileRelevance, this test fails — update
// BOTH TS mirrors (and the wantCount below) so all three stay in lockstep.
func TestProfileRelevance_RecognizedStringsSnapshot(t *testing.T) {
	recognized := []string{
		"mandatory for MRV&DCS",
		"recommended for MRV&DCS",
		"mandatory for MRV",
		"voluntary wrt MRV",
		"for CII correction, voluntary wrt MRV",
		"for CII correction",
		"DSC only, voluntary wrt MRV",
		"mandatory for FEUM and in case of no fuel consumption for any verification",
		// The "verfication" typo is faithfully copied from the source OVD
		// xlsx and must match byte-for-byte across all three files.
		"recommended for voyage level verfication schemes",
	}
	const wantCount = 9
	if len(recognized) != wantCount {
		t.Fatalf("recognized list has %d entries, want %d — keep the TS mirrors in sync", len(recognized), wantCount)
	}
	for _, s := range recognized {
		if !GHGRelevant(s) {
			t.Errorf("relevance %q is no longer recognized by profileRelevance — a case was removed/renamed; update the two TS mirrors too", s)
		}
	}
	// Strings outside the set must stay unrecognized (the loud-failure
	// design: unknown wording falls through to "not GHG-relevant").
	for _, s := range []string{"optional input", "not in use", "out of scope for GHG verification", "some new OVD wording"} {
		if GHGRelevant(s) {
			t.Errorf("relevance %q unexpectedly recognized — a case was added; update this snapshot and the two TS mirrors", s)
		}
	}
}
