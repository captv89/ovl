// SPDX-License-Identifier: AGPL-3.0-only

package configbundle

import (
	"testing"

	"github.com/captv89/ovl/office/compliance"
	"github.com/captv89/ovl/office/fieldpolicy"
	"github.com/captv89/ovl/pkg/validation"
)

func TestPublish(t *testing.T) {
	tests := []struct {
		name        string
		composed    *ConfigBundle
		publishedBy string
		wantErr     bool
	}{
		{"valid", &ConfigBundle{}, "user-1", false},
		{"nil composed rejected", nil, "user-1", true},
		{"missing publishedBy rejected", &ConfigBundle{}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := Publish(tt.composed, "My Bundle", tt.publishedBy)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Publish() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if b.ID == "" {
				t.Error("ID is empty")
			}
			if b.PublishedAt.IsZero() {
				t.Error("PublishedAt is zero")
			}
			if b.Label != "My Bundle" {
				t.Errorf("Label = %q, want %q", b.Label, "My Bundle")
			}
		})
	}
}

func TestCompose_DeepCopiesMutableState(t *testing.T) {
	fp, _ := fieldpolicy.New(compliance.FleetScope(), "log-abstract", "3.13")
	_ = fp.SetPolicy("Some_Field", validation.FieldCompanyMandatory, false)

	composed := Compose(nil, []*fieldpolicy.SchemaFieldPolicy{fp}, nil, nil, nil, []string{"master"})

	// Mutating the source object after Compose must not affect the
	// already-composed bundle — this is the whole point of Compose
	// deep-copying rather than aliasing.
	_ = fp.SetPolicy("Some_Field", validation.FieldHidden, false)

	if composed.FieldPolicies[0].Policy["Some_Field"] != validation.FieldCompanyMandatory {
		t.Errorf("composed field policy changed after source mutation: got %q, want %q (deep copy broken)",
			composed.FieldPolicies[0].Policy["Some_Field"], validation.FieldCompanyMandatory)
	}
}

func TestPublish_DoesNotAliasComposed(t *testing.T) {
	composed := Compose(nil, nil, nil, nil, nil, []string{"master"})
	published, err := Publish(composed, "", "user-1")
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	composed.DefaultRoleNames[0] = "mutated"
	if published.DefaultRoleNames[0] == "mutated" {
		t.Error("Publish aliased composed's DefaultRoleNames slice instead of taking ownership at that point")
	}
}
