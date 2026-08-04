// SPDX-License-Identifier: AGPL-3.0-only

package schema

import (
	"bytes"
	"fmt"
	"io/fs"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Validator checks schema JSON documents against the project meta-schema
// (architecture 5.2). The same Validator is meant to be used everywhere a
// schema document is accepted: CI, the office upload flow, and tests.
type Validator struct {
	meta *jsonschema.Schema
}

// NewValidator compiles the meta-schema document at metaSchemaPath within
// fsys.
func NewValidator(fsys fs.FS, metaSchemaPath string) (*Validator, error) {
	data, err := fs.ReadFile(fsys, metaSchemaPath)
	if err != nil {
		return nil, fmt.Errorf("read meta-schema %s: %w", metaSchemaPath, err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode meta-schema %s: %w", metaSchemaPath, err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(metaSchemaPath, doc); err != nil {
		return nil, fmt.Errorf("add meta-schema resource %s: %w", metaSchemaPath, err)
	}
	compiled, err := compiler.Compile(metaSchemaPath)
	if err != nil {
		return nil, fmt.Errorf("compile meta-schema %s: %w", metaSchemaPath, err)
	}
	return &Validator{meta: compiled}, nil
}

// Validate checks a schema JSON document against the meta-schema. It
// returns a *jsonschema.ValidationError (wrapped) describing every
// violation on failure, per the "hard reject on failure with precise
// errors" requirement in architecture 5.3.
func (v *Validator) Validate(data []byte) error {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("decode schema document: %w", err)
	}
	if err := v.meta.Validate(doc); err != nil {
		return fmt.Errorf("schema document invalid: %w", err)
	}
	return nil
}
