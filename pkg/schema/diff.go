// SPDX-License-Identifier: AGPL-3.0-only

package schema

// FieldChange describes how a single field differs between two schema
// versions, matched by name.
type FieldChange struct {
	Name   string
	Before Field
	After  Field
}

// Diff is the result of comparing two schema versions, per the mandatory
// diff view required before publishing a new version (architecture 5.3
// point 5): fields added, removed, type changed, mandatoriness changed,
// and enum changes.
type Diff struct {
	Added                []Field
	Removed              []Field
	TypeChanged          []FieldChange
	MandatorinessChanged []FieldChange
	EnumChanged          []FieldChange
}

// Empty reports whether the two schema versions have no differences at
// all in the categories Diff tracks.
func (d *Diff) Empty() bool {
	return len(d.Added) == 0 && len(d.Removed) == 0 &&
		len(d.TypeChanged) == 0 && len(d.MandatorinessChanged) == 0 &&
		len(d.EnumChanged) == 0
}

// DiffSchemas compares two versions of the same schema, matching fields
// by name.
func DiffSchemas(before, after *Schema) *Diff {
	beforeByName := make(map[string]Field, len(before.Fields))
	for _, f := range before.Fields {
		beforeByName[f.Name] = f
	}
	afterByName := make(map[string]Field, len(after.Fields))
	for _, f := range after.Fields {
		afterByName[f.Name] = f
	}

	d := &Diff{}
	for _, f := range after.Fields {
		bf, ok := beforeByName[f.Name]
		if !ok {
			d.Added = append(d.Added, f)
			continue
		}
		if bf.Type != f.Type {
			d.TypeChanged = append(d.TypeChanged, FieldChange{Name: f.Name, Before: bf, After: f})
		}
		if bf.SchemaMandatory != f.SchemaMandatory {
			d.MandatorinessChanged = append(d.MandatorinessChanged, FieldChange{Name: f.Name, Before: bf, After: f})
		}
		if !equalStringPtr(bf.EnumRef, f.EnumRef) {
			d.EnumChanged = append(d.EnumChanged, FieldChange{Name: f.Name, Before: bf, After: f})
		}
	}
	for _, f := range before.Fields {
		if _, ok := afterByName[f.Name]; !ok {
			d.Removed = append(d.Removed, f)
		}
	}
	return d
}

func equalStringPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
