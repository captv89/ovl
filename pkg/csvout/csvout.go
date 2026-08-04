// SPDX-License-Identifier: AGPL-3.0-only

package csvout

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/captv89/ovl/pkg/domain"
	"github.com/captv89/ovl/pkg/schema"
)

// Generate writes one OVD-formatted CSV file for s to w: a header row of
// s.Fields' names in schema order, then one data row per report in
// reports, in the order given (architecture 13.5 — "one file per schema
// per export"). Column order and formatting always match s, the active
// schema version, since both come from the same schema.Field slice the
// caller resolved.
//
// Every report must have SchemaName == s.SchemaName; Generate returns an
// error rather than silently emitting a mismatched row.
func Generate(w io.Writer, s *schema.Schema, reports []*domain.Report) error {
	bw := bufio.NewWriter(w)

	header := make([]string, len(s.Fields))
	for i, f := range s.Fields {
		header[i] = f.Name
	}
	if _, err := bw.WriteString(strings.Join(header, ",") + "\n"); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	for _, r := range reports {
		if r.SchemaName != s.SchemaName {
			return fmt.Errorf("report %s: schema %q does not match %q", r.ReportID, r.SchemaName, s.SchemaName)
		}
		row := make([]string, len(s.Fields))
		for i, f := range s.Fields {
			v, err := formatValue(f, r.Fields[f.Name])
			if err != nil {
				return fmt.Errorf("report %s: %w", r.ReportID, err)
			}
			row[i] = v
		}
		if _, err := bw.WriteString(strings.Join(row, ",") + "\n"); err != nil {
			return fmt.Errorf("write row for report %s: %w", r.ReportID, err)
		}
	}

	return bw.Flush()
}
