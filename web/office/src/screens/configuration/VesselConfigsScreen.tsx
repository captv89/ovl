// SPDX-License-Identifier: AGPL-3.0-only

import { useEffect, useState } from "react";
import { DataTable } from "../../design/components/data/DataTable.jsx";
import { AlertBanner } from "../../design/components/feedback/AlertBanner.jsx";
import { api, type VesselConfigView } from "../../api/client";

const STATUS_LABEL: Record<VesselConfigView["status"], string> = {
  synced: "Synced",
  outOfDate: "Out of date",
  pendingSync: "Pending sync",
  unassigned: "Unassigned",
};

// Mirrors the mockup's own v.statusStyle exactly: Synced uses the
// underway status color, Out of date uses the (distinct, more alarming)
// warning status color, everything else falls back to a neutral
// surface tone — not DataTable's generic badge tone set, which maps
// "warning" to the caution color, not the warning one.
const STATUS_COLOR: Record<VesselConfigView["status"], { bg: string; fg: string }> = {
  synced: { bg: "var(--color-status-underway-container)", fg: "var(--color-status-underway)" },
  outOfDate: { bg: "var(--color-status-warning-container)", fg: "var(--color-status-warning)" },
  pendingSync: { bg: "var(--color-surface-container-highest)", fg: "var(--color-on-surface-variant)" },
  unassigned: { bg: "var(--color-surface-container-highest)", fg: "var(--color-on-surface-variant)" },
};

// Design handoff B7's 2026-07-25 redesign: the fleet-wide complement to
// B2's per-vessel Config tab (VesselDetailScreen's ConfigTab) — every
// vessel's assigned bundle compared against what it last reported
// running, in one table, so an admin can spot outliers without opening
// each vessel individually.
export function VesselConfigsScreen() {
  const [rows, setRows] = useState<VesselConfigView[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api
      .listVesselConfigs()
      .then(setRows)
      .catch((err) => setError(err instanceof Error ? err.message : "Could not load vessel configs."));
  }, []);

  return (
    <div style={{ padding: 24, display: "flex", flexDirection: "column", gap: 16 }}>
      {error ? <AlertBanner level="warning" title="Something went wrong" message={error} /> : null}
      {!rows ? (
        <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
          Loading…
        </div>
      ) : (
        <DataTable
          columns={[
            { key: "vessel", label: "Vessel", type: "iconText", sortable: true },
            {
              key: "imo",
              label: "IMO",
              type: "text",
              render: (row) => <span className="md-body-medium" style={{ color: "var(--color-on-surface-variant)", fontFamily: "var(--font-mono)" }}>{row.imo}</span>,
            },
            { key: "bundle", label: "Applied config package", type: "text" },
            {
              key: "activeSince",
              label: "Active since",
              type: "text",
              sortable: true,
              render: (row) => <span className="md-body-medium" style={{ color: "var(--color-on-surface-variant)", fontFamily: "var(--font-mono)" }}>{row.activeSince}</span>,
            },
            {
              key: "status",
              label: "Status",
              type: "text",
              filterable: true,
              render: (row) => {
                const color = STATUS_COLOR[row.statusKey as VesselConfigView["status"]];
                return (
                  <span
                    className="md-label-medium"
                    style={{
                      display: "inline-flex", alignItems: "center", gap: 6, padding: "4px 10px",
                      borderRadius: "var(--shape-full)", fontWeight: 700, fontSize: 12,
                      background: color.bg, color: color.fg,
                    }}
                  >
                    <span style={{ width: 6, height: 6, borderRadius: "50%", background: "currentColor", display: "inline-block" }} />
                    {row.status}
                  </span>
                );
              },
            },
          ]}
          rows={rows.map((r) => ({
            id: r.vesselId,
            vessel: { icon: "directions_boat", text: r.vesselName },
            imo: r.imo,
            bundle: r.assignedBundleLabel || "—",
            activeSince: r.activeSince ? new Date(r.activeSince).toLocaleDateString() : "—",
            status: STATUS_LABEL[r.status],
            statusKey: r.status,
          }))}
          searchPlaceholder="Search vessels"
          emptyMessage="No vessels yet."
        />
      )}
    </div>
  );
}
