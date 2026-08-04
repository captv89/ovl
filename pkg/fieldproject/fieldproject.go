// SPDX-License-Identifier: AGPL-3.0-only

// Package fieldproject is the single source of truth for "what Go shape
// does this OVD field type actually hold" — extracted from pkg/csvout's
// own formatValue (which needed exactly this same type-assertion logic
// before it could stringify a value) so office/graphql's resolvers can
// project a report's raw JSONB fields map into typed values too, instead
// of forking the type-switch a second time with its own drift risk.
// pkg/csvout keeps its own string-formatting rules (OVD's comma/dot
// quoting, whole-vs-decimal number formatting) layered on top of this —
// this package only answers "is this a string, a number, or a bool,"
// not "how should it be printed."
package fieldproject

import (
	"fmt"

	"github.com/captv89/ovl/pkg/schema"
)

// Kind discriminates which field of Value actually holds data.
type Kind int

const (
	KindNull Kind = iota
	KindString
	KindNumber
	KindBool
)

// Value is one field's projected, typed value. Exactly one of
// String/Number/Bool is meaningful, per Kind — text/enum/date/time/
// dateTime project to String (date/time/dateTime are already stored as
// the exact OVD-formatted strings pkg/validation's own layouts require,
// so no further parsing is needed or wanted), wholeNumber/decimal to
// Number, boolean to Bool. KindNull means the field had no value at all
// (schema-optional field never filled in) — distinct from a real zero
// value, matching the OVD rule that empty stays empty and is never
// zero-filled.
type Value struct {
	Kind   Kind
	String string
	Number float64
	Bool   bool
}

// Project converts v (as decoded from a report's JSONB fields map,
// i.e. whatever encoding/json/database/sql produced) into a typed Value
// per f's declared FieldType. Returns an error if v's actual Go type
// doesn't match what f.Type implies — the same defensive check
// pkg/csvout's formatValue already made before this extraction.
func Project(f schema.Field, v any) (Value, error) {
	if v == nil {
		return Value{Kind: KindNull}, nil
	}
	switch f.Type {
	case schema.FieldTypeText, schema.FieldTypeEnum, schema.FieldTypeDate, schema.FieldTypeTime, schema.FieldTypeDateTime:
		s, ok := v.(string)
		if !ok {
			return Value{}, fmt.Errorf("field %s: want string, got %T", f.Name, v)
		}
		return Value{Kind: KindString, String: s}, nil
	case schema.FieldTypeWholeNumber, schema.FieldTypeDecimal:
		n, ok := v.(float64)
		if !ok {
			return Value{}, fmt.Errorf("field %s: want float64, got %T", f.Name, v)
		}
		return Value{Kind: KindNumber, Number: n}, nil
	case schema.FieldTypeBoolean:
		b, ok := v.(bool)
		if !ok {
			return Value{}, fmt.Errorf("field %s: want bool, got %T", f.Name, v)
		}
		return Value{Kind: KindBool, Bool: b}, nil
	default:
		return Value{}, fmt.Errorf("field %s: unknown field type %q", f.Name, f.Type)
	}
}
