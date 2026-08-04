// SPDX-License-Identifier: AGPL-3.0-only

package csvout

import (
	"testing"

	"github.com/captv89/ovl/pkg/schema"
)

func TestFormatValue(t *testing.T) {
	tests := []struct {
		name string
		f    schema.Field
		v    any
		want string
	}{
		{"text plain", schema.Field{Type: schema.FieldTypeText}, "Departure", "Departure"},
		{"text with comma is quoted", schema.Field{Type: schema.FieldTypeText}, "Rotterdam, NL", `"Rotterdam, NL"`},
		{"text with dot is quoted", schema.Field{Type: schema.FieldTypeText}, "St. Petersburg", `"St. Petersburg"`},
		{"text with both is quoted once", schema.Field{Type: schema.FieldTypeText}, "St. Petersburg, RU", `"St. Petersburg, RU"`},
		{"text missing value is empty", schema.Field{Type: schema.FieldTypeText}, nil, ""},
		{"enum value passes through like text", schema.Field{Type: schema.FieldTypeEnum}, "Departure", "Departure"},
		{"enum value with comma is quoted", schema.Field{Type: schema.FieldTypeEnum}, "a,b", `"a,b"`},

		{"whole number", schema.Field{Type: schema.FieldTypeWholeNumber}, float64(9074729), "9074729"},
		{"whole number zero formats as 0, not empty", schema.Field{Type: schema.FieldTypeWholeNumber}, float64(0), "0"},
		{"whole number missing value is empty, not zero", schema.Field{Type: schema.FieldTypeWholeNumber}, nil, ""},

		{"decimal keeps dot separator unquoted", schema.Field{Type: schema.FieldTypeDecimal}, 12.5, "12.5"},
		{"decimal with no fractional part", schema.Field{Type: schema.FieldTypeDecimal}, 900.0, "900"},
		{"decimal negative", schema.Field{Type: schema.FieldTypeDecimal}, -3.75, "-3.75"},
		{"decimal missing value is empty, not zero", schema.Field{Type: schema.FieldTypeDecimal}, nil, ""},

		{"boolean true", schema.Field{Type: schema.FieldTypeBoolean}, true, "1"},
		{"boolean false", schema.Field{Type: schema.FieldTypeBoolean}, false, "0"},
		{"boolean missing value is empty", schema.Field{Type: schema.FieldTypeBoolean}, nil, ""},

		{"date passes through, already yyyy-mm-dd", schema.Field{Type: schema.FieldTypeDate}, "2026-07-12", "2026-07-12"},
		{"time passes through, already hh:mm", schema.Field{Type: schema.FieldTypeTime}, "14:30", "14:30"},
		{"dateTime passes through, already yyyy-mm-dd hh:mm", schema.Field{Type: schema.FieldTypeDateTime}, "2026-07-12 14:30", "2026-07-12 14:30"},
		{"date missing value is empty", schema.Field{Type: schema.FieldTypeDate}, nil, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := formatValue(tt.f, tt.v)
			if err != nil {
				t.Fatalf("formatValue() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("formatValue() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatValue_WrongType(t *testing.T) {
	_, err := formatValue(schema.Field{Type: schema.FieldTypeWholeNumber}, "not a number")
	if err == nil {
		t.Fatal("expected an error for a wholeNumber field holding a string, got nil")
	}
}
