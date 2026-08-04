// SPDX-License-Identifier: AGPL-3.0-only

// Package restorebundle builds architecture 12.5's office-generated
// restore bundle content (pkg/restorebundle.Bundle) from office's own
// store. A separate package from pkg/restorebundle (the shared wire
// envelope both office and vessel import) and from office/httpapi (which
// used to own this logic as an unexported method) because two office
// call sites now need it: office/httpapi's browser-download handler and
// office/syncservice's vessel-credential-authenticated FetchRestoreBundle
// RPC (architecture 11.2's PullInbox restore_commands delivery path).
// Neither of those packages imports the other, so the shared logic lives
// here instead.
package restorebundle

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/captv89/ovl/office/configbundle"
	"github.com/captv89/ovl/office/store"
	"github.com/captv89/ovl/pkg/domain"
	"github.com/captv89/ovl/pkg/restorebundle"
)

// BuildBundle assembles vesselID's full report history (every version,
// not just the latest — ListReports itself only ever surfaces the latest
// per report_id, so the distinct report_id set it returns is used purely
// as a work list here), audit trail, chat, and currently-resolved config
// bundle (architecture 12.5's "all reports, config, chat and
// attachments"). Deliberately still excludes attachments: they are
// content-addressed blobs living in pkg/attachmentstore, a different
// retrieval/streaming shape than this single-JSON-document approach —
// a real, explicit scope boundary carried over from before this package
// existed, not a new gap introduced here.
func BuildBundle(ctx context.Context, st *store.Store, vesselID, vesselName, vesselIMO string) (*restorebundle.Bundle, error) {
	latest, err := st.ListReports(ctx, store.ReportFilter{VesselID: &vesselID})
	if err != nil {
		return nil, fmt.Errorf("list reports for vessel %s: %w", vesselID, err)
	}

	bundle := &restorebundle.Bundle{
		VesselID: vesselID, VesselName: vesselName, VesselIMO: vesselIMO,
		GeneratedAt: time.Now().UTC(),
		Reports:     make([]restorebundle.BundleReport, 0, len(latest)),
	}
	for _, row := range latest {
		versions, err := st.ListReportVersions(ctx, vesselID, row.ReportID)
		if err != nil {
			return nil, fmt.Errorf("list versions for report %s: %w", row.ReportID, err)
		}
		auditRows, err := st.ListReportAuditEvents(ctx, vesselID, row.ReportID)
		if err != nil {
			return nil, fmt.Errorf("list audit events for report %s: %w", row.ReportID, err)
		}
		events := make([]domain.Event, len(auditRows))
		for i, ar := range auditRows {
			events[i] = ar.Event
		}
		chat, err := st.ListChatMessages(ctx, vesselID, row.ReportID)
		if err != nil {
			return nil, fmt.Errorf("list chat for report %s: %w", row.ReportID, err)
		}
		bundle.Reports = append(bundle.Reports, restorebundle.BundleReport{
			ReportID: row.ReportID, Versions: versions, Events: events, Chat: chat,
		})
	}

	cb, err := resolvedConfigBundle(ctx, st, vesselID)
	if err != nil {
		return nil, err
	}
	bundle.ConfigBundle = cb
	return bundle, nil
}

// resolvedConfigBundle mirrors office/syncservice.pullAssignedBundle's
// own resolution (vessel-scope wins over group-scope, office/
// configbundle.Resolve) but unconditionally includes the result — a
// restore bundle is a point-in-time full snapshot, not a cursor-delta
// pull, so there is no "only if newer than the vessel's cursor" gate
// here. Returns nil, nil (not an error) if the vessel has no bundle
// assignment resolved at all — a real, reachable state (a vessel with no
// group tags and no direct assignment yet).
func resolvedConfigBundle(ctx context.Context, st *store.Store, vesselID string) (*restorebundle.ConfigBundle, error) {
	vessel, err := st.GetVessel(ctx, vesselID)
	if err != nil {
		return nil, fmt.Errorf("load vessel %s: %w", vesselID, err)
	}
	assignments, err := st.ListBundleAssignments(ctx)
	if err != nil {
		return nil, fmt.Errorf("list bundle assignments: %w", err)
	}
	match := configbundle.Resolve(assignments, vesselID, vessel.Groups)
	if match == nil {
		return nil, nil
	}

	cursor, err := st.GetConfigBundleCursor(ctx, match.BundleID)
	if err != nil {
		return nil, fmt.Errorf("get cursor for bundle %s: %w", match.BundleID, err)
	}
	bundle, err := st.GetConfigBundle(ctx, match.BundleID)
	if err != nil {
		return nil, fmt.Errorf("get bundle %s: %w", match.BundleID, err)
	}
	// Same tagged, resolved-for-this-vessel wire shape the live pull path
	// emits (office/syncservice.pullAssignedBundle) — a restored vessel
	// applies its config bundle through the identical configwire.Decode
	// path, so the DR snapshot must not carry the old raw-marshal shape.
	contentJSON, err := json.Marshal(bundle.ResolveFor(vesselID, vessel.Groups, cursor))
	if err != nil {
		return nil, fmt.Errorf("marshal bundle %s: %w", bundle.ID, err)
	}
	return &restorebundle.ConfigBundle{
		BundleID: bundle.ID, VersionNo: cursor, ContentJSON: contentJSON, PublishedAt: bundle.PublishedAt,
	}, nil
}
