// SPDX-License-Identifier: AGPL-3.0-only

package schema

import "testing"

func TestDiffSchemas(t *testing.T) {
	before := &Schema{Fields: []Field{
		{Name: "IMO", Type: FieldTypeWholeNumber, SchemaMandatory: true},
		{Name: "Removed_Field", Type: FieldTypeText},
		{Name: "Type_Changed", Type: FieldTypeText},
		{Name: "Mandatory_Changed", Type: FieldTypeText, SchemaMandatory: false},
		{Name: "Enum_Changed", Type: FieldTypeEnum, EnumRef: new("fuel-types")},
		{Name: "Unchanged", Type: FieldTypeDecimal, SchemaMandatory: true, EnumRef: nil},
	}}
	after := &Schema{Fields: []Field{
		{Name: "IMO", Type: FieldTypeWholeNumber, SchemaMandatory: true},
		{Name: "Type_Changed", Type: FieldTypeDecimal},
		{Name: "Mandatory_Changed", Type: FieldTypeText, SchemaMandatory: true},
		{Name: "Enum_Changed", Type: FieldTypeEnum, EnumRef: new("charter-types")},
		{Name: "Unchanged", Type: FieldTypeDecimal, SchemaMandatory: true, EnumRef: nil},
		{Name: "Added_Field", Type: FieldTypeText},
	}}

	d := DiffSchemas(before, after)

	if len(d.Added) != 1 || d.Added[0].Name != "Added_Field" {
		t.Errorf("Added = %+v, want [Added_Field]", d.Added)
	}
	if len(d.Removed) != 1 || d.Removed[0].Name != "Removed_Field" {
		t.Errorf("Removed = %+v, want [Removed_Field]", d.Removed)
	}
	if len(d.TypeChanged) != 1 || d.TypeChanged[0].Name != "Type_Changed" {
		t.Errorf("TypeChanged = %+v, want [Type_Changed]", d.TypeChanged)
	}
	if len(d.MandatorinessChanged) != 1 || d.MandatorinessChanged[0].Name != "Mandatory_Changed" {
		t.Errorf("MandatorinessChanged = %+v, want [Mandatory_Changed]", d.MandatorinessChanged)
	}
	if len(d.EnumChanged) != 1 || d.EnumChanged[0].Name != "Enum_Changed" {
		t.Errorf("EnumChanged = %+v, want [Enum_Changed]", d.EnumChanged)
	}
	if d.Empty() {
		t.Error("Empty() = true, want false")
	}
}

func TestDiffSchemas_NoChanges(t *testing.T) {
	s := &Schema{Fields: []Field{
		{Name: "IMO", Type: FieldTypeWholeNumber, SchemaMandatory: true},
	}}
	d := DiffSchemas(s, s)
	if !d.Empty() {
		t.Errorf("Diff of identical schemas = %+v, want Empty()", d)
	}
}

func TestEqualStringPtr(t *testing.T) {
	a, b := "x", "x"
	tests := []struct {
		name string
		a, b *string
		want bool
	}{
		{"both nil", nil, nil, true},
		{"one nil", &a, nil, false},
		{"equal values", &a, &b, true},
		{"different values", new("x"), new("y"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := equalStringPtr(tt.a, tt.b); got != tt.want {
				t.Errorf("equalStringPtr(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
