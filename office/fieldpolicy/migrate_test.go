// SPDX-License-Identifier: AGPL-3.0-only

package fieldpolicy

import (
	"slices"
	"testing"

	"github.com/captv89/ovl/office/compliance"
	"github.com/captv89/ovl/pkg/schema"
	"github.com/captv89/ovl/pkg/validation"
)

func TestMigrate(t *testing.T) {
	before := &schema.Schema{
		SchemaName: "log-abstract",
		Version:    "3.13",
		Fields: []schema.Field{
			{Name: "Kept_Field", Type: schema.FieldTypeText},
			{Name: "Removed_Field", Type: schema.FieldTypeText},
		},
	}
	after := &schema.Schema{
		SchemaName: "log-abstract",
		Version:    "3.14",
		Fields: []schema.Field{
			{Name: "Kept_Field", Type: schema.FieldTypeText},
			{Name: "New_Field", Type: schema.FieldTypeText},
		},
	}
	diff := schema.DiffSchemas(before, after)

	old, _ := New(compliance.FleetScope(), "log-abstract", "3.13")
	if err := old.SetPolicy("Kept_Field", validation.FieldRecommended, false); err != nil {
		t.Fatalf("SetPolicy: %v", err)
	}
	if err := old.SetPrefillClass("Kept_Field", PrefillGhost); err != nil {
		t.Fatalf("SetPrefillClass: %v", err)
	}
	if err := old.SetPolicy("Removed_Field", validation.FieldCompanyMandatory, false); err != nil {
		t.Fatalf("SetPolicy: %v", err)
	}

	result := Migrate(old, "3.14", diff)

	if result.NewPolicy.SchemaVersion != "3.14" {
		t.Errorf("NewPolicy.SchemaVersion = %q, want 3.14", result.NewPolicy.SchemaVersion)
	}
	if got := result.NewPolicy.Policy.StateFor("Kept_Field", false, ""); got != validation.FieldRecommended {
		t.Errorf("Kept_Field carried-over state = %q, want recommended", got)
	}
	if got := result.NewPolicy.PrefillFor("Kept_Field"); got != PrefillGhost {
		t.Errorf("Kept_Field carried-over prefill = %q, want ghost", got)
	}
	if _, ok := result.NewPolicy.Policy["Removed_Field"]; ok {
		t.Error("Removed_Field's policy override carried over, want dropped")
	}
	if !slices.Contains(result.RemovedFields, "Removed_Field") {
		t.Errorf("RemovedFields = %v, want to contain Removed_Field", result.RemovedFields)
	}
	if !slices.Contains(result.NewFields, "New_Field") {
		t.Errorf("NewFields = %v, want to contain New_Field", result.NewFields)
	}
	// New fields default to optional — nothing carried, nothing set.
	if got := result.NewPolicy.Policy.StateFor("New_Field", false, ""); got != validation.FieldOptional {
		t.Errorf("New_Field default state = %q, want optional", got)
	}
}
