// SPDX-License-Identifier: AGPL-3.0-only

package schema

import "testing"

func TestParse(t *testing.T) {
	data := []byte(`{
		"schemaName": "log-abstract",
		"ovdVersion": "3.13",
		"version": "3.13",
		"sections": ["header"],
		"fields": [
			{
				"name": "IMO",
				"label": "IMO Number",
				"type": "wholeNumber",
				"schemaMandatory": true,
				"relevance": "mandatory for MRV&DCS",
				"section": "header",
				"appliesToEvents": ["*"]
			}
		]
	}`)

	s, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if s.SchemaName != "log-abstract" {
		t.Errorf("SchemaName = %q, want log-abstract", s.SchemaName)
	}
	if len(s.Fields) != 1 || s.Fields[0].Name != "IMO" {
		t.Fatalf("Fields = %+v, want one field named IMO", s.Fields)
	}
	if s.Fields[0].Type != FieldTypeWholeNumber {
		t.Errorf("Fields[0].Type = %q, want %q", s.Fields[0].Type, FieldTypeWholeNumber)
	}
}

func TestParse_Invalid(t *testing.T) {
	if _, err := Parse([]byte(`not json`)); err == nil {
		t.Fatal("Parse of invalid JSON: got nil error, want an error")
	}
}

func TestSchema_FieldByName(t *testing.T) {
	s := &Schema{Fields: []Field{{Name: "IMO"}, {Name: "Event"}}}

	tests := []struct {
		name string
		want bool
	}{
		{"IMO", true},
		{"Event", true},
		{"Missing", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, ok := s.FieldByName(tt.name)
			if ok != tt.want {
				t.Fatalf("FieldByName(%q) ok = %v, want %v", tt.name, ok, tt.want)
			}
			if ok && f.Name != tt.name {
				t.Errorf("FieldByName(%q).Name = %q", tt.name, f.Name)
			}
		})
	}
}
