// SPDX-License-Identifier: AGPL-3.0-only

package configwire

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// sampleBundle is the canonical fixture the golden file pins. It exercises
// every field so a shape change (a renamed JSON tag, a dropped field) is
// caught by the golden diff.
func sampleBundle() *Bundle {
	return &Bundle{
		WireVersion: WireVersion,
		BundleID:    "0190a1b2-c3d4-7e5f-8000-000000000001",
		VersionNo:   7,
		PublishedAt: time.Date(2026, 7, 23, 10, 30, 0, 0, time.UTC),
		Schemas: []SchemaConfig{
			{
				SchemaName: "log-abstract",
				Version:    "3.13",
				Policy: map[string]string{
					"Voyage_Number": "companyMandatory",
					"O_ROB":         "hidden",
					"Wind_Force_Kn": "recommended",
				},
				Prefill: map[string]string{
					"HFO_ROB":       "computed",
					"Voyage_Number": "carryForward",
				},
			},
		},
		RegulatoryProfiles: []string{"mrv", "cii"},
		MaxGapHours:        8,
		RuleSeverities:     map[string]string{"ROB_CONTINUITY": "error"},
		DefaultRoleNames:   []string{"master", "officer"},
	}
}

func TestBundle_GoldenWireShape(t *testing.T) {
	got, err := json.MarshalIndent(sampleBundle(), "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got = append(got, '\n')

	goldenPath := filepath.Join("testdata", "bundle.golden.json")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
			t.Fatalf("update golden: %v", err)
		}
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (run with UPDATE_GOLDEN=1 to create): %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("wire shape drifted from golden.\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestDecode_RoundTrip(t *testing.T) {
	orig := sampleBundle()
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !reflect.DeepEqual(orig, got) {
		t.Errorf("round trip mismatch:\n got %+v\nwant %+v", got, orig)
	}
}

func TestDecode_RejectsUnknownVersion(t *testing.T) {
	// A pre-configwire raw bundle has no wireVersion field, so it decodes
	// to 0 — must be rejected, not silently applied.
	if _, err := Decode([]byte(`{"bundleId":"x"}`)); err == nil {
		t.Fatal("expected error for wireVersion 0, got nil")
	}
	// A future format must also be rejected so an old vessel degrades to
	// its defaults instead of misreading a newer shape.
	if _, err := Decode([]byte(`{"wireVersion":999}`)); err == nil {
		t.Fatal("expected error for future wireVersion, got nil")
	}
	if _, err := Decode([]byte(`{not json`)); err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func TestSchemaConfigFor(t *testing.T) {
	b := sampleBundle()
	if sc := b.SchemaConfigFor("log-abstract"); sc == nil || sc.Policy["O_ROB"] != "hidden" {
		t.Fatalf("SchemaConfigFor(log-abstract) = %+v, want the log-abstract config", sc)
	}
	if sc := b.SchemaConfigFor("nonexistent"); sc != nil {
		t.Fatalf("SchemaConfigFor(nonexistent) = %+v, want nil", sc)
	}
}
