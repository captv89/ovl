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

// sampleLogAbstractSchema exercises one field of every schema.FieldType,
// using real field names/labels/formats lifted from the OVD 3.13
// interface description's "Log abstract" sheet (IMO, Date_UTC, Time_UTC,
// ETA) plus representative stand-ins for the rest, so the golden file
// below is traceable back to that source rather than invented.
func sampleLogAbstractSchema() *schema.Schema {
	return &schema.Schema{
		SchemaName: "log-abstract",
		OvdVersion: "3.13",
		Version:    "test",
		Fields: []schema.Field{
			{Name: "IMO", Label: "Vessel (IMO Number)", Type: schema.FieldTypeWholeNumber},
			{Name: "Date_UTC", Label: "Date", Type: schema.FieldTypeDate},
			{Name: "Time_UTC", Label: "Time", Type: schema.FieldTypeTime},
			{Name: "Event", Label: "Event", Type: schema.FieldTypeEnum},
			{Name: "Voyage_From", Label: "Voyage from", Type: schema.FieldTypeText},
			{Name: "ETA", Label: "ETA", Type: schema.FieldTypeDateTime},
			{Name: "Time_Since_Previous_Report", Label: "Time since previous report", Type: schema.FieldTypeDecimal},
			{Name: "ME_1_Aux_Blower", Label: "Aux. blower", Type: schema.FieldTypeBoolean},
		},
	}
}

func TestGenerate_Golden(t *testing.T) {
	s := sampleLogAbstractSchema()
	reports := []*domain.Report{
		{
			ReportID:   "report-1",
			SchemaName: "log-abstract",
			Fields: map[string]any{
				"IMO":                        float64(9074729),
				"Date_UTC":                   "2026-07-12",
				"Time_UTC":                   "14:30",
				"Event":                      "Departure",
				"Voyage_From":                "Rotterdam, NL",
				"ETA":                        "2026-07-13 06:00",
				"Time_Since_Previous_Report": 12.5,
				"ME_1_Aux_Blower":            true,
			},
		},
		{
			// Deliberately omits Voyage_From and ETA to prove empty
			// fields stay truly empty rather than being zero-filled
			// (architecture 13.5).
			ReportID:   "report-2",
			SchemaName: "log-abstract",
			Fields: map[string]any{
				"IMO":                        float64(9074729),
				"Date_UTC":                   "2026-07-12",
				"Time_UTC":                   "18:00",
				"Event":                      "Arrival",
				"Time_Since_Previous_Report": 900.0,
				"ME_1_Aux_Blower":            false,
			},
		},
	}

	var buf bytes.Buffer
	if err := Generate(&buf, s, reports); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	golden := filepath.Join("testdata", "log_abstract_golden.csv")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(golden, buf.Bytes(), 0o644); err != nil {
			t.Fatalf("write golden file: %v", err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden file: %v", err)
	}
	if buf.String() != string(want) {
		t.Errorf("Generate() output does not match golden file %s\ngot:\n%s\nwant:\n%s", golden, buf.String(), string(want))
	}
}

func TestGenerate_HeaderMatchesSchemaFieldOrder(t *testing.T) {
	s := sampleLogAbstractSchema()
	var buf bytes.Buffer
	if err := Generate(&buf, s, nil); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	want := "IMO,Date_UTC,Time_UTC,Event,Voyage_From,ETA,Time_Since_Previous_Report,ME_1_Aux_Blower\n"
	if buf.String() != want {
		t.Errorf("Generate() header = %q, want %q", buf.String(), want)
	}
}

func TestGenerate_SchemaMismatchErrors(t *testing.T) {
	s := sampleLogAbstractSchema()
	reports := []*domain.Report{
		{ReportID: "report-1", SchemaName: "bunker-report", Fields: map[string]any{}},
	}
	var buf bytes.Buffer
	if err := Generate(&buf, s, reports); err == nil {
		t.Fatal("expected an error for a report whose SchemaName doesn't match the schema being exported, got nil")
	}
}
