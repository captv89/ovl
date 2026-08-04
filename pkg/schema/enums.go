// SPDX-License-Identifier: AGPL-3.0-only

package schema

import (
	"encoding/json"
	"fmt"
	"io/fs"
)

// enumFilePaths maps a curated field's enumRef to the curated enum file that
// defines its valid codes. Every file here shares the common
// {"values":[{"code": "..."}, ...]} shape (ignoring whatever other metadata
// a given enum file also carries, e.g. fuel-types' remark/imoFuelTypeName) —
// offshore-modes.json is deliberately not listed: it uses a different
// modes/activities document shape and has no caller needing generic
// resolution yet.
var enumFilePaths = map[string]string{
	"event-types":        "ovd-3.13/enums/event-types.json",
	"fuel-types":         "ovd-3.13/enums/fuel-types.json",
	"incoterms":          "ovd-3.13/enums/incoterms.json",
	"charter-types":      "ovd-3.13/enums/charter-types.json",
	"port-call-purposes": "ovd-3.13/enums/port-call-purposes.json",
	"operational-modes":  "ovd-3.13/enums/operational-modes.json",
	"voyage-types":       "ovd-3.13/enums/voyage-types.json",
}

type enumCodesDoc struct {
	Values []struct {
		Code string `json:"code"`
	} `json:"values"`
}

// ResolveEnum returns the valid codes for a curated field's enumRef, in file
// order. Returns an error if enumRef isn't a known, generically-resolvable
// enum (e.g. "offshore-modes") — callers fall back to unrestricted text
// entry in that case, matching today's behavior for enums with no resolver.
func ResolveEnum(fsys fs.FS, enumRef string) ([]string, error) {
	path, ok := enumFilePaths[enumRef]
	if !ok {
		return nil, fmt.Errorf("no generic resolver for enumRef %q", enumRef)
	}
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		return nil, fmt.Errorf("read enum %s: %w", enumRef, err)
	}
	var doc enumCodesDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse enum %s: %w", enumRef, err)
	}
	codes := make([]string, len(doc.Values))
	for i, v := range doc.Values {
		codes[i] = v.Code
	}
	return codes, nil
}
