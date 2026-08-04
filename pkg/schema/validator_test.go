// SPDX-License-Identifier: AGPL-3.0-only

package schema

import (
	"testing"
	"testing/fstest"
)

const validMinimalSchema = `{
	"schemaName": "log-abstract",
	"ovdVersion": "3.13",
	"version": "3.13",
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
}`

func TestValidator_Validate(t *testing.T) {
	v := newRealValidator(t)

	tests := []struct {
		name    string
		doc     string
		wantErr bool
	}{
		{
			name: "valid minimal schema",
			doc:  validMinimalSchema,
		},
		{
			name:    "unknown schemaName rejected",
			doc:     `{"schemaName":"not-a-schema","ovdVersion":"3.13","version":"3.13","fields":[{"name":"IMO","label":"IMO","type":"wholeNumber","schemaMandatory":true,"relevance":"x","section":"header","appliesToEvents":["*"]}]}`,
			wantErr: true,
		},
		{
			name:    "missing required top-level field rejected",
			doc:     `{"ovdVersion":"3.13","version":"3.13","fields":[]}`,
			wantErr: true,
		},
		{
			name:    "unknown field type rejected",
			doc:     `{"schemaName":"log-abstract","ovdVersion":"3.13","version":"3.13","fields":[{"name":"X","label":"X","type":"currency","schemaMandatory":false,"relevance":"x","section":"header","appliesToEvents":["*"]}]}`,
			wantErr: true,
		},
		{
			name:    "enum type without enumRef rejected",
			doc:     `{"schemaName":"log-abstract","ovdVersion":"3.13","version":"3.13","fields":[{"name":"X","label":"X","type":"enum","schemaMandatory":false,"relevance":"x","section":"header","appliesToEvents":["*"]}]}`,
			wantErr: true,
		},
		{
			name:    "dateTime and boolean types accepted",
			doc:     `{"schemaName":"log-abstract","ovdVersion":"3.13","version":"3.13","fields":[{"name":"ETA","label":"ETA","type":"dateTime","schemaMandatory":false,"relevance":"x","section":"header","appliesToEvents":["*"]},{"name":"Flag","label":"Flag","type":"boolean","schemaMandatory":false,"relevance":"x","section":"header","appliesToEvents":["*"]}]}`,
			wantErr: false,
		},
		{
			name:    "additional top-level property rejected",
			doc:     `{"schemaName":"log-abstract","ovdVersion":"3.13","version":"3.13","fields":[],"extra":true}`,
			wantErr: true,
		},
		{
			name:    "malformed JSON rejected",
			doc:     `not json`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate([]byte(tt.doc))
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewValidator_MissingFile(t *testing.T) {
	if _, err := NewValidator(fstest.MapFS{}, "does-not-exist.json"); err == nil {
		t.Fatal("NewValidator with a missing file: got nil error, want an error")
	}
}
