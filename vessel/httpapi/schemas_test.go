// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/captv89/ovl/pkg/configwire"
	"github.com/captv89/ovl/pkg/domain"
	"github.com/captv89/ovl/vessel/store"
)

func TestHandleGetSchema(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)

	rec := c.do(http.MethodGet, "/api/schemas/log-abstract", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/schemas/log-abstract: status %d, body %s", rec.Code, rec.Body)
	}
	got := decodeBody[schemaConfigResponse](t, rec)
	if got.Schema == nil || got.Schema.SchemaName != "log-abstract" || len(got.Schema.Fields) == 0 {
		t.Fatalf("Schema = %+v, want a populated log-abstract schema", got.Schema)
	}
	if got.FieldPolicy["Voyage_Number"] != "companyMandatory" {
		t.Errorf("FieldPolicy[Voyage_Number] = %q, want companyMandatory", got.FieldPolicy["Voyage_Number"])
	}
	if got.FieldPolicy["O_ROB"] != "hidden" {
		t.Errorf("FieldPolicy[O_ROB] = %q, want hidden", got.FieldPolicy["O_ROB"])
	}
	// No report history yet in this test's fresh store, so HFO_ROB (a
	// real computed prefill sourced from the last submitted report, see
	// TestHandleGetSchema_RealComputedProbFromLastSubmittedReport) has no
	// fake fallback value — left unprefilled, same as carryForward fields.
	if prefill := got.Prefill["HFO_ROB"]; prefill != nil {
		t.Errorf("Prefill[HFO_ROB] = %+v, want absent with no report history", prefill)
	}
	// 18.07.26 manual-test triage ("why limit the auto calculation to just
	// these 2 fields... peripheral vision"): the curated log-abstract
	// schema has ten fuel-type ROB series, not just HFO, and each is drawn
	// down by more than one consumer field (Main Engine, Auxiliary Engine,
	// Boiler, and — for HFO/LFO/MGO/MDO — Inert Gas Generator and Cargo
	// Heating too). See validation.LogAbstractConfig's own doc comment.
	if len(got.RobSeries) != 10 {
		t.Fatalf("len(RobSeries) = %d, want 10 (one per curated fuel type)", len(got.RobSeries))
	}
	byField := make(map[string][]string, len(got.RobSeries))
	for _, s := range got.RobSeries {
		byField[s.ROBField] = s.ConsumptionFields
	}
	hfo, ok := byField["HFO_ROB"]
	if !ok {
		t.Fatal("RobSeries has no HFO_ROB entry")
	}
	for _, want := range []string{"ME_Consumption_HFO", "AE_Consumption_HFO", "Boiler_Consumption_HFO", "Inert_gas_Consumption_HFO", "Cargo_heating_Consumption_HFO"} {
		if !slices.Contains(hfo, want) {
			t.Errorf("HFO_ROB.ConsumptionFields = %v, want it to contain %q", hfo, want)
		}
	}
	// Methanol/Ethanol/Other have irregular ROB field names (not
	// "<suffix>_ROB") and no Inert Gas Generator/Cargo Heating consumer —
	// the two cases most likely to have a typo in the curated table.
	if m, ok := byField["Methanol_ROB"]; !ok || !slices.Contains(m, "ME_Consumption_M") || slices.Contains(m, "Inert_gas_Consumption_M") {
		t.Errorf("Methanol_ROB.ConsumptionFields = %v, want ME_Consumption_M present and no Inert_gas_Consumption_M", m)
	}

	// Every field name referenced by the demo policy/prefill/ROB-series
	// config must actually exist in the schema — a typo here would
	// silently no-op.
	names := make(map[string]bool, len(got.Schema.Fields))
	for _, f := range got.Schema.Fields {
		names[f.Name] = true
	}
	for name := range got.FieldPolicy {
		if !names[name] {
			t.Errorf("FieldPolicy references %q, which is not a field in the schema", name)
		}
	}
	for name := range got.Prefill {
		if !names[name] {
			t.Errorf("Prefill references %q, which is not a field in the schema", name)
		}
	}
	for _, s := range got.RobSeries {
		if !names[s.ROBField] {
			t.Errorf("RobSeries references ROB field %q, which is not a field in the schema", s.ROBField)
		}
		for _, f := range s.ConsumptionFields {
			if !names[f] {
				t.Errorf("RobSeries[%s] references consumption field %q, which is not a field in the schema", s.Name, f)
			}
		}
	}
}

func TestHandleGetSchema_UnknownSchema(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	rec := c.do(http.MethodGet, "/api/schemas/not-a-real-schema", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleGetSchema_PrefillsIMOFromVesselIdentity(t *testing.T) {
	s, c := configuredTestServer(t)
	if err := s.storeOrNil().SaveVesselIdentity(t.Context(), &store.VesselIdentity{Name: "MV Testship", IMO: "9074729"}); err != nil {
		t.Fatalf("SaveVesselIdentity: %v", err)
	}

	for _, schemaName := range []string{"log-abstract", "bunker-report", "edn-report"} {
		rec := c.do(http.MethodGet, "/api/schemas/"+schemaName, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /api/schemas/%s: status %d, body %s", schemaName, rec.Code, rec.Body)
		}
		got := decodeBody[schemaConfigResponse](t, rec)
		prefill := got.Prefill["IMO"]
		if prefill == nil || prefill.Class != "carryForward" || prefill.Value != "9074729" {
			t.Errorf("%s Prefill[IMO] = %+v, want a carryForward entry with value 9074729", schemaName, prefill)
		}
	}
}

func TestHandleGetSchema_NoIMOPrefillWithoutVesselIdentity(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	rec := c.do(http.MethodGet, "/api/schemas/log-abstract", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/schemas/log-abstract: status %d, body %s", rec.Code, rec.Body)
	}
	got := decodeBody[schemaConfigResponse](t, rec)
	if _, ok := got.Prefill["IMO"]; ok {
		t.Errorf("Prefill[IMO] present with no enrolled vessel identity, want absent")
	}
}

func TestHandleGetSchema_NoRealCarryForwardWithoutReportHistory(t *testing.T) {
	_, c := configuredTestServer(t)
	rec := c.do(http.MethodGet, "/api/schemas/log-abstract", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/schemas/log-abstract: status %d, body %s", rec.Code, rec.Body)
	}
	got := decodeBody[schemaConfigResponse](t, rec)
	for _, name := range []string{"Voyage_Number", "Voyage_From", "Voyage_To", "Distance_To_Go"} {
		if _, ok := got.Prefill[name]; ok {
			t.Errorf("Prefill[%s] present with no report history, want absent", name)
		}
	}
}

func TestHandleGetSchema_RealCarryForwardFromLastSubmittedReport(t *testing.T) {
	s, c := configuredTestServer(t)
	st := s.storeOrNil()

	report, _, err := domain.NewReport("log-abstract", "Departure",
		time.Date(2026, 1, 10, 6, 0, 0, 0, time.UTC),
		map[string]any{
			"Voyage_Number":  "V-2026-099",
			"Voyage_From":    "SGSIN",
			"Voyage_To":      "NLRTM",
			"Distance_To_Go": 4213.5,
			// Position isn't in the old curated realCarryForwardFields
			// list — proves carry-forward now covers "literally all the
			// fields" (2026-07-14 p2 manual-test feedback), not just the
			// four voyage-identity fields this session first shipped.
			"Position": "01 15.0N 103 50.0E",
			// Wind_Force_Kn has its own hardcoded "ghost" prefill
			// (fieldConfigFor) — the last submitted report having a real
			// value for it must NOT override that (see below).
			"Wind_Force_Kn": 9.1,
		}, "master")
	if err != nil {
		t.Fatalf("domain.NewReport: %v", err)
	}
	if _, err := report.MarkReady(nil); err != nil {
		t.Fatalf("MarkReady: %v", err)
	}
	if _, err := report.Submit("master"); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if err := st.SaveReport(t.Context(), report); err != nil {
		t.Fatalf("SaveReport: %v", err)
	}

	rec := c.do(http.MethodGet, "/api/schemas/log-abstract", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/schemas/log-abstract: status %d, body %s", rec.Code, rec.Body)
	}
	got := decodeBody[schemaConfigResponse](t, rec)

	cases := map[string]any{
		"Voyage_Number":  "V-2026-099",
		"Voyage_From":    "SGSIN",
		"Voyage_To":      "NLRTM",
		"Distance_To_Go": 4213.5,
		"Position":       "01 15.0N 103 50.0E",
	}
	for name, want := range cases {
		entry := got.Prefill[name]
		if entry == nil || entry.Class != "carryForward" || entry.Value != want {
			t.Errorf("Prefill[%s] = %+v, want a carryForward entry with value %v", name, entry, want)
		}
	}

	if entry := got.Prefill["Wind_Force_Kn"]; entry == nil || entry.Class != "ghost" || entry.Value != 12.4 {
		t.Errorf("Prefill[Wind_Force_Kn] = %+v, want the untouched ghost demo entry (class ghost, value 12.4)", entry)
	}
}

// TestHandleGetSchema_AppliedBundleAbsentPrefillNotCarriedForward is the
// root-cause regression for the 2026-08-01 report: with no bundle applied,
// handleGetSchema's blanket "carry forward every field the last report
// had" fallback (TestHandleGetSchema_RealCarryForwardFromLastSubmittedReport,
// above) is correct — it's the pre-config-bundle stand-in the 2026-07-14
// decision was made for, when there was no other way to control prefill.
// But once a real bundle is applied, fieldConfigFromBundle's own contract
// (configbundle.go: "that field simply takes its default") and the office
// Field Policy editor's own encoding (fieldPolicyLogic.ts effectivePrefill:
// an absent key defaults to "none", and setFieldPrefill/applyBulkPrefill
// delete the key for "none" rather than writing it) both say a field
// absent from the bundle's Prefill map should get NO prefill — not
// carryForward. Time_Elapsed_Sailing here is a period-length quantity
// (hours since the *previous* report) that the office bundle deliberately
// leaves unconfigured; the last submitted report had a real value for it,
// and prior to this fix the blanket fallback ignored the applied bundle
// entirely and carried that stale, wrong-period value forward anyway —
// which is what produced the "buckets sum to 312h but Time_Since_Previous_
// Report is 171h" plausibility warning on a freshly opened, unedited form.
func TestHandleGetSchema_AppliedBundleAbsentPrefillNotCarriedForward(t *testing.T) {
	s, c := configuredTestServer(t)
	st := s.storeOrNil()

	bundleContent, err := json.Marshal(configwire.Bundle{
		WireVersion: configwire.WireVersion,
		BundleID:    "bundle-prefill-1",
		VersionNo:   1,
		PublishedAt: time.Now().UTC(),
		Schemas: []configwire.SchemaConfig{{
			SchemaName: "log-abstract",
			Version:    "3.13",
			// Voyage_Number is explicitly configured; Time_Elapsed_Sailing
			// is deliberately left out, meaning "no prefill" per the
			// office UI's own encoding.
			Prefill: map[string]string{"Voyage_Number": "carryForward"},
		}},
	})
	if err != nil {
		t.Fatalf("marshal configwire bundle: %v", err)
	}
	if err := st.ApplyConfigBundle(t.Context(), store.PulledConfigBundle{
		BundleID:    "bundle-prefill-1",
		VersionNo:   1,
		Content:     bundleContent,
		PublishedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("ApplyConfigBundle: %v", err)
	}

	report, _, err := domain.NewReport("log-abstract", "Departure",
		time.Date(2026, 1, 10, 6, 0, 0, 0, time.UTC),
		map[string]any{
			"Voyage_Number":        "V-2026-099",
			"Time_Elapsed_Sailing": 312.0,
		}, "master")
	if err != nil {
		t.Fatalf("domain.NewReport: %v", err)
	}
	if _, err := report.MarkReady(nil); err != nil {
		t.Fatalf("MarkReady: %v", err)
	}
	if _, err := report.Submit("master"); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if err := st.SaveReport(t.Context(), report); err != nil {
		t.Fatalf("SaveReport: %v", err)
	}

	rec := c.do(http.MethodGet, "/api/schemas/log-abstract", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/schemas/log-abstract: status %d, body %s", rec.Code, rec.Body)
	}
	got := decodeBody[schemaConfigResponse](t, rec)

	if entry := got.Prefill["Voyage_Number"]; entry == nil || entry.Class != "carryForward" || entry.Value != "V-2026-099" {
		t.Errorf("Prefill[Voyage_Number] = %+v, want the bundle-configured carryForward entry", entry)
	}
	if entry, ok := got.Prefill["Time_Elapsed_Sailing"]; ok {
		t.Errorf("Prefill[Time_Elapsed_Sailing] = %+v, want absent (bundle applied, field not configured ⇒ none)", entry)
	}
}

// 18.07.26 manual-test items 3/6/8/10: HFO_ROB/Time_Since_Previous_Report
// used to be fixed constants (812.4/24.0) on every report regardless of
// history, which made cascade revalidation (runCascade) compare real
// entered consumption/elapsed-time against a baseline that never moved —
// tripping continuity findings and cascade-invalidating reports that were
// never actually inconsistent. Proves the real fix: both are now sourced
// from the vessel's own last submitted report.
func TestHandleGetSchema_RealComputedProbFromLastSubmittedReport(t *testing.T) {
	s, c := configuredTestServer(t)
	st := s.storeOrNil()

	priorEventTime := time.Date(2026, 1, 10, 6, 0, 0, 0, time.UTC)
	report, _, err := domain.NewReport("log-abstract", "Departure", priorEventTime,
		map[string]any{"HFO_ROB": 812.4}, "master")
	if err != nil {
		t.Fatalf("domain.NewReport: %v", err)
	}
	if _, err := report.MarkReady(nil); err != nil {
		t.Fatalf("MarkReady: %v", err)
	}
	if _, err := report.Submit("master"); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if err := st.SaveReport(t.Context(), report); err != nil {
		t.Fatalf("SaveReport: %v", err)
	}

	rec := c.do(http.MethodGet, "/api/schemas/log-abstract", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/schemas/log-abstract: status %d, body %s", rec.Code, rec.Body)
	}
	got := decodeBody[schemaConfigResponse](t, rec)

	robEntry := got.Prefill["HFO_ROB"]
	if robEntry == nil || robEntry.Class != "computed" || robEntry.Value != 812.4 || robEntry.Formula == "" {
		t.Errorf("Prefill[HFO_ROB] = %+v, want a computed entry carrying the previous report's real value (812.4)", robEntry)
	}

	tspEntry := got.Prefill["Time_Since_Previous_Report"]
	if tspEntry == nil || tspEntry.Class != "computed" || tspEntry.Formula == "" {
		t.Fatalf("Prefill[Time_Since_Previous_Report] = %+v, want a computed entry", tspEntry)
	}
	hours, ok := tspEntry.Value.(float64)
	wantHours := time.Since(priorEventTime).Hours()
	if !ok || hours < wantHours-0.1 || hours > wantHours+0.1 {
		t.Errorf("Prefill[Time_Since_Previous_Report].Value = %v, want ~%.2f (real elapsed hours since the previous report)", tspEntry.Value, wantHours)
	}

	if got.LastReportEventTime == nil || !got.LastReportEventTime.Equal(priorEventTime) {
		t.Errorf("LastReportEventTime = %v, want %v", got.LastReportEventTime, priorEventTime)
	}
}

func TestHandleGetSchema_OtherSchemaHasEmptyConfig(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	rec := c.do(http.MethodGet, "/api/schemas/bunker-report", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body)
	}
	got := decodeBody[schemaConfigResponse](t, rec)
	if len(got.FieldPolicy) != 0 || len(got.Prefill) != 0 {
		t.Errorf("bunker-report FieldPolicy/Prefill = %v / %v, want both empty (no demo config curated for it)", got.FieldPolicy, got.Prefill)
	}
}
