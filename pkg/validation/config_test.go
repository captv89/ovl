// SPDX-License-Identifier: AGPL-3.0-only

package validation

import "testing"

// TestLogAbstractConfig_TenWellFormedSeries is the structural regression
// test for 18.07.26 manual-test triage ("why limit the auto calculation
// to just these 2 fields... peripheral vision"): LogAbstractConfig used
// to be three independent hand-copies (vessel/httpapi, office/
// syncservice, cmd/ovl-validate), all only tracking HFO against
// ME_Consumption_HFO alone. This is now the one shared source; this test
// exists so a future edit to logAbstractFuelTypes can't silently drop a
// fuel type or a consumer field without a test failing.
func TestLogAbstractConfig_TenWellFormedSeries(t *testing.T) {
	cfg := LogAbstractConfig()

	if len(cfg.ROBSeriesList) != 10 {
		t.Fatalf("len(ROBSeriesList) = %d, want 10", len(cfg.ROBSeriesList))
	}

	seenROBFields := make(map[string]bool, len(cfg.ROBSeriesList))
	wantExtraConsumers := map[string]bool{"HFO": true, "LFO": true, "MGO": true, "MDO": true}
	for _, s := range cfg.ROBSeriesList {
		if s.Name == "" || s.ROBField == "" {
			t.Errorf("series %+v has an empty Name or ROBField", s)
		}
		if seenROBFields[s.ROBField] {
			t.Errorf("ROBField %q appears in more than one series", s.ROBField)
		}
		seenROBFields[s.ROBField] = true

		wantLen := 3
		if wantExtraConsumers[s.Name] {
			wantLen = 5
		}
		if len(s.ConsumptionFields) != wantLen {
			t.Errorf("series %s has %d consumption fields, want %d", s.Name, len(s.ConsumptionFields), wantLen)
		}

		seenConsumptionFields := make(map[string]bool, len(s.ConsumptionFields))
		for _, f := range s.ConsumptionFields {
			if seenConsumptionFields[f] {
				t.Errorf("series %s lists consumption field %q more than once", s.Name, f)
			}
			seenConsumptionFields[f] = true
		}
	}

	// FuelTypeConsumptionFields (used by the consumption-scheme-
	// exclusivity hard rule) must be exactly the union of every series'
	// consumption fields — this rule needs to see ANY fuel-type field in
	// use, not just HFO's, or a report using only e.g. AE_Consumption_MGO
	// would silently look like it wasn't using the fuel-type scheme at
	// all.
	var wantFuelFields int
	for _, s := range cfg.ROBSeriesList {
		wantFuelFields += len(s.ConsumptionFields)
	}
	if len(cfg.FuelTypeConsumptionFields) != wantFuelFields {
		t.Errorf("len(FuelTypeConsumptionFields) = %d, want %d (union of every series' consumption fields)", len(cfg.FuelTypeConsumptionFields), wantFuelFields)
	}
}
