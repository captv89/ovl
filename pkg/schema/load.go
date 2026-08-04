// SPDX-License-Identifier: AGPL-3.0-only

package schema

import (
	"fmt"
	"io/fs"
)

// Load reads and parses the schema document at path within fsys. If v is
// non-nil, the document is validated against the meta-schema first and
// Load fails with the validation error rather than returning a
// possibly-invalid Schema.
func Load(fsys fs.FS, path string, v *Validator) (*Schema, error) {
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		return nil, fmt.Errorf("read schema %s: %w", path, err)
	}
	if v != nil {
		if err := v.Validate(data); err != nil {
			return nil, fmt.Errorf("validate schema %s: %w", path, err)
		}
	}
	return Parse(data)
}
