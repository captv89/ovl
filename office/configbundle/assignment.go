// SPDX-License-Identifier: AGPL-3.0-only

package configbundle

import (
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/captv89/ovl/office/compliance"
)

// BundleAssignment records which published ConfigBundle applies to one
// vessel, vessel group, or the whole fleet (design handoff B7's
// "assignment picker (vessels or groups)," extended to include
// fleet-wide by 2026-07-15 test feedback #3 — architecture 6.5 originally
// read "assigned per vessel or per vessel group" only; corrected to match,
// same vessel > group > fleet precedence ProfileAssignment/CadenceRule/
// RuleSeverityAssignment already use). Reuses office/compliance.Scope for
// the vessel/group/fleet identity.
//
// Resolving which bundle actually applies (see Resolve below) is now
// built — it wasn't when this comment was first written, back when this
// package's own doc comment still called it "Phase 4 sync territory."
// Tracking whether a vessel has actually pulled/applied its resolved
// bundle yet (B7's "per-vessel applied-state: pulled or pending next
// sync" display) is separate and still not built — that's a dashboard/UI
// concern layered on top of the sync cursor PullInbox now advances, not
// this package's job.
type BundleAssignment struct {
	Scope      compliance.Scope
	BundleID   string
	AssignedAt time.Time
}

// NewBundleAssignment validates scope and bundleID.
func NewBundleAssignment(scope compliance.Scope, bundleID string) (*BundleAssignment, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	bundleID = strings.TrimSpace(bundleID)
	if bundleID == "" {
		return nil, errors.New("configbundle: bundle id is required")
	}
	return &BundleAssignment{Scope: scope, BundleID: bundleID, AssignedAt: time.Now().UTC()}, nil
}

// Resolve picks which assignment in assignments applies to the vessel
// identified by vesselID/vesselGroups — architecture 6.5's "the vessel
// pulls the newest assigned bundle on every sync," which presupposes
// exactly one bundle applies at a time. Most specific wins: a direct
// vessel-scope assignment always wins over a group-scope one, which
// always wins over a fleet-wide one — the same vessel > group > fleet
// precedence ProfileAssignment/CadenceRule/RuleSeverityAssignment already
// use. Among group-scope assignments (a vessel can belong to more than
// one assigned group), the first match in assignments wins — architecture
// 6.5 does not define a tie-break for that case, so this is a defined,
// documented default rather than a claim that one group is authoritative
// over another. Returns nil if nothing matches at all (the vessel has no
// bundle assigned, not even fleet-wide).
func Resolve(assignments []*BundleAssignment, vesselID string, vesselGroups []string) *BundleAssignment {
	for _, a := range assignments {
		if a.Scope.Type == compliance.ScopeVessel && a.Scope.Key == vesselID {
			return a
		}
	}
	for _, a := range assignments {
		if a.Scope.Type == compliance.ScopeGroup && slices.Contains(vesselGroups, a.Scope.Key) {
			return a
		}
	}
	for _, a := range assignments {
		if a.Scope.Type == compliance.ScopeFleet {
			return a
		}
	}
	return nil
}
