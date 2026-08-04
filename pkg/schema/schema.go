// SPDX-License-Identifier: AGPL-3.0-only

package schema

import (
	"encoding/json"
	"fmt"
)

// FieldType is one of the field types defined by schemas/meta-schema.json.
type FieldType string

const (
	FieldTypeText        FieldType = "text"
	FieldTypeWholeNumber FieldType = "wholeNumber"
	FieldTypeDecimal     FieldType = "decimal"
	FieldTypeDate        FieldType = "date"
	FieldTypeTime        FieldType = "time"
	FieldTypeDateTime    FieldType = "dateTime"
	FieldTypeBoolean     FieldType = "boolean"
	FieldTypeEnum        FieldType = "enum"
)

// Field is a single OVD schema field, matching the "field" definition in
// schemas/meta-schema.json.
type Field struct {
	Name             string    `json:"name"`
	Label            string    `json:"label"`
	Type             FieldType `json:"type"`
	Unit             *string   `json:"unit,omitempty"`
	MaxLength        *int      `json:"maxLength,omitempty"`
	EnumRef          *string   `json:"enumRef,omitempty"`
	SchemaMandatory  bool      `json:"schemaMandatory"`
	MandatoryNote    *string   `json:"mandatoryNote,omitempty"`
	Relevance        string    `json:"relevance"`
	Section          string    `json:"section"`
	AppliesToEvents  []string  `json:"appliesToEvents"`
	Description      string    `json:"description,omitempty"`
	UnsupportedByDnv bool      `json:"unsupportedByDnv,omitempty"`
}

// Schema is a curated OVD schema document (Log Abstract, Bunker Report,
// EDN Report, Commercial Period, or Cargo Nomination), matching
// schemas/meta-schema.json.
//
// Published schema versions are immutable (architecture 5.2): a new
// version is always a new document, never an edit of Version in place.
type Schema struct {
	SchemaName string   `json:"schemaName"`
	OvdVersion string   `json:"ovdVersion"`
	Version    string   `json:"version"`
	Sections   []string `json:"sections,omitempty"`
	Fields     []Field  `json:"fields"`
}

// Parse unmarshals a schema JSON document. It does not validate the
// document against the meta-schema; use Load, or call a Validator
// directly, for that.
func Parse(data []byte) (*Schema, error) {
	var s Schema
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse schema: %w", err)
	}
	return &s, nil
}

// FieldByName returns the field with the given name, if present.
func (s *Schema) FieldByName(name string) (Field, bool) {
	for _, f := range s.Fields {
		if f.Name == name {
			return f, true
		}
	}
	return Field{}, false
}
