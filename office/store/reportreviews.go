// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"fmt"
	"time"
)

// MarkReviewed records that reviewedBy has looked at reportID (design
// handoff B3's "mark reviewed" bulk action, Phase 5 open question 3's
// resolved default) — an office-only triage annotation, never a
// lifecycle state the vessel sees. Upserted: re-marking an already-
// reviewed report just updates who/when, no review history is kept.
func (s *Store) MarkReviewed(ctx context.Context, vesselID, reportID, reviewedBy string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO report_reviews (vessel_id, report_id, reviewed_by, reviewed_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (vessel_id, report_id) DO UPDATE SET
			reviewed_by = EXCLUDED.reviewed_by, reviewed_at = EXCLUDED.reviewed_at
	`, vesselID, reportID, reviewedBy, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("mark report %s reviewed for vessel %s: %w", reportID, vesselID, err)
	}
	return nil
}
