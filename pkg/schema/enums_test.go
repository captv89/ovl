// SPDX-License-Identifier: AGPL-3.0-only

package schema

import (
	"testing"
	"testing/fstest"
)

func TestResolveEnum(t *testing.T) {
	fsys := fstest.MapFS{
		"ovd-3.13/enums/operational-modes.json": &fstest.MapFile{Data: []byte(`{
			"enumName": "operational-modes",
			"ovdVersion": "3.13",
			"values": [{"code": "InPort"}, {"code": "AtSea"}, {"code": "Sailing"}]
		}`)},
		"ovd-3.13/enums/charter-types.json": &fstest.MapFile{Data: []byte(`{
			"enumName": "charter-types",
			"ovdVersion": "3.13",
			"values": [{"code": "TC", "label": "Time Charter"}, {"code": "VC", "label": "Voyage Charter"}]
		}`)},
	}

	tests := []struct {
		name    string
		enumRef string
		want    []string
		wantErr bool
	}{
		{name: "operational-modes", enumRef: "operational-modes", want: []string{"InPort", "AtSea", "Sailing"}},
		{name: "charter-types ignores label field", enumRef: "charter-types", want: []string{"TC", "VC"}},
		{name: "enumRef with no generic resolver", enumRef: "offshore-modes", wantErr: true},
		{name: "known enumRef missing from filesystem", enumRef: "event-types", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveEnum(fsys, tt.enumRef)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ResolveEnum(%q) = %v, want error", tt.enumRef, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveEnum(%q) unexpected error: %v", tt.enumRef, err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("ResolveEnum(%q) = %v, want %v", tt.enumRef, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("ResolveEnum(%q)[%d] = %q, want %q", tt.enumRef, i, got[i], tt.want[i])
				}
			}
		})
	}
}
