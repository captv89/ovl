// SPDX-License-Identifier: AGPL-3.0-only

package domain

import "time"

// InvalidationNotice is computed office-side by cascade revalidation
// (architecture 8.3) when an earlier report's change breaks a later
// one, and pulled by the affected vessel so it can apply the same
// Invalidate call locally (see Report.Invalidate).
type InvalidationNotice struct {
	ReportID    string
	VersionNo   int
	BrokenRules []string
	ComputedAt  time.Time
}
