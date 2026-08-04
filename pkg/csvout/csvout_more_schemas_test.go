// SPDX-License-Identifier: AGPL-3.0-only

package csvout

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/captv89/ovl/pkg/domain"
	"github.com/captv89/ovl/pkg/schema"
)

// The four schemas below carry over sampleLogAbstractSchema's own
// convention (real field names/labels/types/enum codes lifted from
// schemas/ovd-3.13/*.json — which is itself curated from the OVD 3.13
// interface description — not invented), completing golden-file
// coverage for architecture 13.5's CSV export across every curated
// schema. Each picks a representative subset of that schema's real
// fields (not the full field list, matching sampleLogAbstractSchema's
// own "one report is deliberately partial" shape to also prove empty
// fields stay empty rather than zero-filled) — none of these four
// schemas has a boolean field in the real interface description, unlike
// log-abstract's own ME_1_Aux_Blower, so boolean encoding stays covered
// by that existing golden test and TestFormatValue rather than an
// invented field here.

func sampleBunkerReportSchema() *schema.Schema {
	return &schema.Schema{
		SchemaName: "bunker-report",
		OvdVersion: "3.13",
		Version:    "test",
		Fields: []schema.Field{
			{Name: "IMO", Label: "IMO number", Type: schema.FieldTypeWholeNumber},
			{Name: "BDN_Number", Label: "Bunker Delivery Note Number", Type: schema.FieldTypeText},
			{Name: "Bunker_Delivery_Date", Label: "Bunker Delivery Date", Type: schema.FieldTypeDate},
			{Name: "Bunker_Delivery_Time", Label: "Bunker Delivery Time", Type: schema.FieldTypeTime},
			{Name: "Fuel_Type", Label: "Fuel Type", Type: schema.FieldTypeEnum},
			{Name: "Mass", Label: "Mass", Type: schema.FieldTypeDecimal},
			{Name: "Sustainability", Label: "Sustainability", Type: schema.FieldTypeText},
		},
	}
}

func TestGenerate_Golden_BunkerReport(t *testing.T) {
	s := sampleBunkerReportSchema()
	reports := []*domain.Report{
		{
			ReportID:   "report-1",
			SchemaName: "bunker-report",
			Fields: map[string]any{
				"IMO":                  float64(9074729),
				"BDN_Number":           "BDN-2026-0451",
				"Bunker_Delivery_Date": "2026-07-12",
				"Bunker_Delivery_Time": "09:15",
				"Fuel_Type":            "HFO",
				"Mass":                 450.5,
				"Sustainability":       "ISCC EU, batch 22",
			},
		},
		{
			// Sustainability omitted — proves an optional text field
			// stays empty rather than zero-filled (architecture 13.5).
			ReportID:   "report-2",
			SchemaName: "bunker-report",
			Fields: map[string]any{
				"IMO":                  float64(9074729),
				"BDN_Number":           "BDN-2026-0452",
				"Bunker_Delivery_Date": "2026-07-14",
				"Bunker_Delivery_Time": "17:40",
				"Fuel_Type":            "LFO",
				"Mass":                 120.0,
			},
		},
	}

	var buf bytes.Buffer
	if err := Generate(&buf, s, reports); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	compareGolden(t, "bunker_report_golden.csv", buf.Bytes())
}

func sampleCargoNominationSchema() *schema.Schema {
	return &schema.Schema{
		SchemaName: "cargo-nomination",
		OvdVersion: "3.13",
		Version:    "test",
		Fields: []schema.Field{
			{Name: "Cargo_Nomination_Id", Label: "Cargo nomination id", Type: schema.FieldTypeText},
			{Name: "IMO", Label: "IMO number", Type: schema.FieldTypeWholeNumber},
			{Name: "Bill_Of_Lading_Date", Label: "Bill of Lading date", Type: schema.FieldTypeDate},
			{Name: "Inco_Term", Label: "Incoterm", Type: schema.FieldTypeEnum},
			{Name: "Seller_Name", Label: "Seller name", Type: schema.FieldTypeText},
			{Name: "Cargo_Mt", Label: "Cargo (mt)", Type: schema.FieldTypeDecimal},
			{Name: "Cargo_TEU", Label: "Cargo (TEU)", Type: schema.FieldTypeWholeNumber},
			{Name: "Carbon_Offset_Comment", Label: "Carbon offset comment", Type: schema.FieldTypeText},
		},
	}
}

func TestGenerate_Golden_CargoNomination(t *testing.T) {
	s := sampleCargoNominationSchema()
	reports := []*domain.Report{
		{
			ReportID:   "report-1",
			SchemaName: "cargo-nomination",
			Fields: map[string]any{
				"Cargo_Nomination_Id": "CN-2026-104",
				"IMO":                 float64(9074729),
				"Bill_Of_Lading_Date": "2026-07-10",
				"Inco_Term":           "FOB",
				"Seller_Name":         "Acme Bunkering, Ltd",
				"Cargo_Mt":            32000.75,
				"Cargo_TEU":           float64(0),
			},
		},
		{
			// Cargo_TEU and Carbon_Offset_Comment omitted — proves both
			// a numeric and a text field stay truly empty when absent.
			ReportID:   "report-2",
			SchemaName: "cargo-nomination",
			Fields: map[string]any{
				"Cargo_Nomination_Id": "CN-2026-105",
				"IMO":                 float64(9074729),
				"Bill_Of_Lading_Date": "2026-07-15",
				"Inco_Term":           "EXW",
				"Seller_Name":         "Rotterdam Fuels",
				"Cargo_Mt":            18250.0,
			},
		},
	}

	var buf bytes.Buffer
	if err := Generate(&buf, s, reports); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	compareGolden(t, "cargo_nomination_golden.csv", buf.Bytes())
}

func sampleCommercialPeriodSchema() *schema.Schema {
	return &schema.Schema{
		SchemaName: "commercial-period",
		OvdVersion: "3.13",
		Version:    "test",
		Fields: []schema.Field{
			{Name: "IMO", Label: "Vessel", Type: schema.FieldTypeWholeNumber},
			{Name: "Period_Id", Label: "Period identifier", Type: schema.FieldTypeText},
			{Name: "Exclude_From_Period", Label: "Period identifier of the parent period", Type: schema.FieldTypeText},
			{Name: "Period_Start", Label: "Period start", Type: schema.FieldTypeDateTime},
			{Name: "Period_End", Label: "Period end", Type: schema.FieldTypeDateTime},
			{Name: "Description", Label: "Description of the commercial period", Type: schema.FieldTypeText},
		},
	}
}

func TestGenerate_Golden_CommercialPeriod(t *testing.T) {
	s := sampleCommercialPeriodSchema()
	reports := []*domain.Report{
		{
			ReportID:   "report-1",
			SchemaName: "commercial-period",
			Fields: map[string]any{
				"IMO":          float64(9074729),
				"Period_Id":    "CP-2026-03",
				"Period_Start": "2026-07-01 00:00",
				"Period_End":   "2026-07-15 00:00",
				"Description":  "Time charter, leg 1",
			},
		},
		{
			// Exclude_From_Period and Description omitted — this
			// schema's every field beyond IMO/Period_Id/the two
			// timestamps is optional.
			ReportID:   "report-2",
			SchemaName: "commercial-period",
			Fields: map[string]any{
				"IMO":          float64(9074729),
				"Period_Id":    "CP-2026-04",
				"Period_Start": "2026-07-15 00:00",
				"Period_End":   "2026-07-31 00:00",
			},
		},
	}

	var buf bytes.Buffer
	if err := Generate(&buf, s, reports); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	compareGolden(t, "commercial_period_golden.csv", buf.Bytes())
}

func sampleEdnReportSchema() *schema.Schema {
	return &schema.Schema{
		SchemaName: "edn-report",
		OvdVersion: "3.13",
		Version:    "test",
		Fields: []schema.Field{
			{Name: "IMO", Label: "IMO number", Type: schema.FieldTypeWholeNumber},
			{Name: "EDN_Number", Label: "Electricity Delivery Note Number", Type: schema.FieldTypeText},
			{Name: "Electricity_Delivery_Date", Label: "Electricity Delivery Date", Type: schema.FieldTypeDate},
			{Name: "Electricity_Delivery_Time", Label: "Electricity Delivery Time", Type: schema.FieldTypeTime},
			{Name: "Electrical_Work", Label: "Electrical work delivered", Type: schema.FieldTypeDecimal},
			{Name: "Sustainability", Label: "Sustainability", Type: schema.FieldTypeText},
		},
	}
}

func TestGenerate_Golden_EdnReport(t *testing.T) {
	s := sampleEdnReportSchema()
	reports := []*domain.Report{
		{
			ReportID:   "report-1",
			SchemaName: "edn-report",
			Fields: map[string]any{
				"IMO":                       float64(9074729),
				"EDN_Number":                "EDN-2026-0012",
				"Electricity_Delivery_Date": "2026-07-12",
				"Electricity_Delivery_Time": "22:00",
				"Electrical_Work":           125.3,
				"Sustainability":            "Shore power, grid mix",
			},
		},
		{
			// Sustainability omitted.
			ReportID:   "report-2",
			SchemaName: "edn-report",
			Fields: map[string]any{
				"IMO":                       float64(9074729),
				"EDN_Number":                "EDN-2026-0013",
				"Electricity_Delivery_Date": "2026-07-13",
				"Electricity_Delivery_Time": "06:30",
				"Electrical_Work":           40.0,
			},
		},
	}

	var buf bytes.Buffer
	if err := Generate(&buf, s, reports); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	compareGolden(t, "edn_report_golden.csv", buf.Bytes())
}

// compareGolden mirrors TestGenerate_Golden's own golden-file comparison
// (same UPDATE_GOLDEN=1 regeneration escape hatch), factored out once a
// second schema needed the identical logic.
func compareGolden(t *testing.T, filename string, got []byte) {
	t.Helper()
	golden := filepath.Join("testdata", filename)
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(golden, got, 0o644); err != nil {
			t.Fatalf("write golden file: %v", err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden file: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("Generate() output does not match golden file %s\ngot:\n%s\nwant:\n%s", golden, string(got), string(want))
	}
}
