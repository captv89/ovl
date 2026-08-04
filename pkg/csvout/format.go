// SPDX-License-Identifier: AGPL-3.0-only

package csvout

import (
	"strconv"
	"strings"

	"github.com/captv89/ovl/pkg/fieldproject"
	"github.com/captv89/ovl/pkg/schema"
)

// formatValue renders a single field value per the OVD 3.13 data
// formatting requirements (interface description, "Notes on GHG
// reporting" sheet, "Data formatting requirements"):
//   - A missing/nil value stays truly empty, never zero-filled.
//   - The decimal separator is ".", which Go's default float formatting
//     already produces, so no locale handling is needed or wanted.
//   - Only text/enum values are quoted, and only when they contain a
//     "," or "." — numeric fields legitimately contain "." as their own
//     decimal separator and are never quoted, since a "." isn't the
//     field separator and can't break column parsing.
//   - date/time/dateTime fields are already stored as the exact
//     "yyyy-mm-dd" / "hh:mm" / "yyyy-mm-dd hh:mm" strings the interface
//     description specifies (matching pkg/validation's own dateLayout/
//     timeLayout/dateTimeLayout, which fields are validated against
//     before a report can be submitted), so they pass through unchanged.
//
// Boolean encoding as "1"/"0" is this package's own choice, not
// something the interface description states: its own two example
// boolean fields (ME_1_Aux_Blower, Boiler_1_Operation_Mode) are both
// flagged unsupportedByDnv, so there's no documented convention to
// follow. This picks OVD's general numeric-flag style over "true"/
// "false" or "Yes"/"No" — revisit if real onboarding surfaces guidance.
//
// The "is v actually the Go type f.Type implies" check now lives in
// pkg/fieldproject.Project, shared with office/graphql's resolvers
// (added alongside the data API, architecture 13) — this function only
// adds CSV's own string-formatting rules on top of that typed value, so
// the two consumers can never disagree about what a field's raw JSONB
// value actually means, only about how to print it.
func formatValue(f schema.Field, v any) (string, error) {
	val, err := fieldproject.Project(f, v)
	if err != nil {
		return "", err
	}
	switch val.Kind {
	case fieldproject.KindNull:
		return "", nil
	case fieldproject.KindString:
		if strings.ContainsAny(val.String, ",.") {
			return `"` + val.String + `"`, nil
		}
		return val.String, nil
	case fieldproject.KindNumber:
		if f.Type == schema.FieldTypeWholeNumber {
			return strconv.FormatFloat(val.Number, 'f', 0, 64), nil
		}
		return strconv.FormatFloat(val.Number, 'f', -1, 64), nil
	case fieldproject.KindBool:
		if val.Bool {
			return "1", nil
		}
		return "0", nil
	default:
		return "", nil
	}
}
