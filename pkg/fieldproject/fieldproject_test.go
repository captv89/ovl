// SPDX-License-Identifier: AGPL-3.0-only

package fieldproject

import (
	"testing"

	"github.com/captv89/ovl/pkg/schema"
)

func TestProject(t *testing.T) {
	tests := []struct {
		name     string
		f        schema.Field
		v        any
		wantKind Kind
	}{
		{"text", schema.Field{Type: schema.FieldTypeText}, "Departure", KindString},
		{"enum", schema.Field{Type: schema.FieldTypeEnum}, "Departure", KindString},
		{"date", schema.Field{Type: schema.FieldTypeDate}, "2026-07-12", KindString},
		{"time", schema.Field{Type: schema.FieldTypeTime}, "14:30", KindString},
		{"dateTime", schema.Field{Type: schema.FieldTypeDateTime}, "2026-07-12 14:30", KindString},
		{"wholeNumber", schema.Field{Type: schema.FieldTypeWholeNumber}, float64(9074729), KindNumber},
		{"decimal", schema.Field{Type: schema.FieldTypeDecimal}, 12.5, KindNumber},
		{"boolean", schema.Field{Type: schema.FieldTypeBoolean}, true, KindBool},
		{"nil is null regardless of type", schema.Field{Type: schema.FieldTypeWholeNumber}, nil, KindNull},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Project(tt.f, tt.v)
			if err != nil {
				t.Fatalf("Project() error = %v", err)
			}
			if got.Kind != tt.wantKind {
				t.Errorf("Project().Kind = %v, want %v", got.Kind, tt.wantKind)
			}
		})
	}
}

func TestProject_StringValue(t *testing.T) {
	got, err := Project(schema.Field{Name: "EventType", Type: schema.FieldTypeText}, "Departure")
	if err != nil {
		t.Fatalf("Project() error = %v", err)
	}
	if got.String != "Departure" {
		t.Errorf("String = %q, want %q", got.String, "Departure")
	}
}

func TestProject_NumberValue(t *testing.T) {
	got, err := Project(schema.Field{Name: "IMO", Type: schema.FieldTypeWholeNumber}, float64(9074729))
	if err != nil {
		t.Fatalf("Project() error = %v", err)
	}
	if got.Number != 9074729 {
		t.Errorf("Number = %v, want %v", got.Number, 9074729)
	}
}

func TestProject_BoolValue(t *testing.T) {
	got, err := Project(schema.Field{Name: "Flag", Type: schema.FieldTypeBoolean}, true)
	if err != nil {
		t.Fatalf("Project() error = %v", err)
	}
	if !got.Bool {
		t.Error("Bool = false, want true")
	}
}

func TestProject_WrongType(t *testing.T) {
	if _, err := Project(schema.Field{Name: "IMO", Type: schema.FieldTypeWholeNumber}, "not a number"); err == nil {
		t.Fatal("expected an error for a wholeNumber field holding a string, got nil")
	}
}

func TestProject_UnknownFieldType(t *testing.T) {
	if _, err := Project(schema.Field{Name: "Mystery", Type: "not-a-real-type"}, "x"); err == nil {
		t.Fatal("expected an error for an unknown field type, got nil")
	}
}
