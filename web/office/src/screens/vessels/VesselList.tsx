// SPDX-License-Identifier: AGPL-3.0-only

import { useEffect, useMemo, useState } from "react";
import { Button } from "../../design/components/core/Button.jsx";
import { Chip } from "../../design/components/surfaces/Chip.jsx";
import { DataTable, type BadgeTone } from "../../design/components/data/DataTable.jsx";
import { AlertBanner } from "../../design/components/feedback/AlertBanner.jsx";
import { api, type EnrollmentState, type VesselView } from "../../api/client";
import { formatOverdueDuration, syncHealth } from "../../format";

function enrollmentLabel(state: EnrollmentState): string {
  return { notIssued: "Not issued", issued: "Issued", enrolled: "Enrolled", revoked: "Revoked" }[state];
}

function enrollmentTone(state: EnrollmentState): BadgeTone {
  return { notIssued: "neutral", issued: "info", enrolled: "success", revoked: "error" }[state] as BadgeTone;
}

// Design handoff B2's fleet list: name, IMO, type, groups (tag chips),
// enrollment state, filterable by group and enrollment state. Last sync/
// last report/overdue indicator are all wired now (see
// office/httpapi.vesselView's own doc comment on where last sync comes
// from and format.ts's syncHealth on the "Online"/"Stale" heuristic).
export function VesselList({
  canCreate,
  globalGroup,
  onOpenVessel,
  onCreateVessel,
  onOpenMap,
}: {
  canCreate: boolean;
  /** App-level global group filter (top bar) — an additional AND-filter alongside this screen's own group chips, not a replacement for them. */
  globalGroup: string | null;
  onOpenVessel: (id: string) => void;
  onCreateVessel: () => void;
  /** Design handoff B2·1 note 3: "Map view" opens the schematic fleet map (B2·M) — a view toggle within Vessels, not a separate nav destination. */
  onOpenMap: () => void;
}) {
  const [vessels, setVessels] = useState<VesselView[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [selectedGroups, setSelectedGroups] = useState<Set<string>>(new Set());

  useEffect(() => {
    let cancelled = false;
    api
      .listVessels()
      .then((list) => {
        if (!cancelled) setVessels(list);
      })
      .catch((err) => {
        if (!cancelled) setError(err instanceof Error ? err.message : "Could not load vessels.");
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const allGroups = useMemo(() => {
    const set = new Set<string>();
    for (const v of vessels ?? []) {
      for (const g of v.groups) set.add(g);
    }
    return [...set].sort();
  }, [vessels]);

  const filtered = useMemo(() => {
    if (!vessels) return [];
    return vessels
      .filter((v) => selectedGroups.size === 0 || v.groups.some((g) => selectedGroups.has(g)))
      .filter((v) => !globalGroup || v.groups.includes(globalGroup));
  }, [vessels, selectedGroups, globalGroup]);

  function toggleGroup(g: string) {
    setSelectedGroups((prev) => {
      const next = new Set(prev);
      if (next.has(g)) {
        next.delete(g);
      } else {
        next.add(g);
      }
      return next;
    });
  }

  if (error) {
    return (
      <div style={{ padding: 24 }}>
        <AlertBanner level="warning" title="Couldn't load vessels" message={error} />
      </div>
    );
  }
  if (!vessels) {
    return (
      <div className="md-body-medium" style={{ padding: 24, color: "var(--color-on-surface-variant)" }}>
        Loading vessels…
      </div>
    );
  }

  return (
    <div style={{ padding: 24, display: "flex", flexDirection: "column", gap: 16 }}>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <div className="md-headline-small">Vessels</div>
        <div style={{ display: "flex", gap: 8 }}>
          <Button variant="outlined" icon="map" onClick={onOpenMap}>
            Map view
          </Button>
          {canCreate ? (
            <Button variant="filled" icon="add" onClick={onCreateVessel}>
              Add vessel
            </Button>
          ) : null}
        </div>
      </div>

      {allGroups.length > 0 ? (
        <div style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
          {allGroups.map((g) => (
            <Chip key={g} label={g} type="filter" selected={selectedGroups.has(g)} onClick={() => toggleGroup(g)} />
          ))}
        </div>
      ) : null}

      <DataTable
        columns={[
          { key: "vessel", label: "Vessel", type: "iconText", sortable: true },
          { key: "type", label: "Type", type: "text", sortable: true },
          { key: "groups", label: "Groups", type: "text" },
          { key: "enrollment", label: "Enrollment", type: "badge", filterable: true },
          {
            key: "lastSync",
            label: "Last sync",
            type: "text",
            render: (row) => {
              const health = syncHealth(row.lastSyncAt as string | undefined);
              const color = health.tone === "success" ? "var(--color-status-underway)" : health.tone === "warning" ? "var(--color-error)" : "var(--color-on-surface-variant)";
              return (
                <span>
                  <span style={{ color, fontWeight: 600 }}>{health.label}</span>
                  {row.lastSyncAt ? <span style={{ color: "var(--color-on-surface-variant)" }}> · {new Date(row.lastSyncAt as string).toLocaleString()}</span> : null}
                </span>
              );
            },
          },
          {
            key: "lastReport",
            label: "Last report",
            type: "text",
            render: (row) => (row.lastReportAt ? <span>{new Date(row.lastReportAt as string).toLocaleString()}</span> : <span>—</span>),
          },
          {
            key: "overdue",
            label: "Overdue by",
            type: "text",
            sortable: true,
            render: (row) =>
              row.overdueHours != null ? (
                <span style={{ color: "var(--color-error)", fontWeight: 700 }}>{formatOverdueDuration(row.overdueHours as number)}</span>
              ) : (
                <span>—</span>
              ),
          },
        ]}
        rows={filtered
          .slice()
          // Worst-first (design handoff B1's own Overdue vessels table
          // sort) — overdue vessels sort ahead of everything else, most
          // overdue first; vessels without an overdue value keep their
          // existing (name) order after that.
          .sort((a, b) => (b.overdueHours ?? -1) - (a.overdueHours ?? -1))
          .map((v) => ({
            id: v.id,
            vessel: { icon: "directions_boat", text: v.name, subtext: v.imo },
            type: v.type,
            groups: v.groups.length > 0 ? v.groups.join(", ") : "—",
            enrollment: { label: enrollmentLabel(v.enrollmentState), tone: enrollmentTone(v.enrollmentState) },
            lastSyncAt: v.lastSyncAt,
            lastReportAt: v.lastReportAt,
            overdueHours: v.overdueHours,
          }))}
        onRowAction={(row) => onOpenVessel(row.id as string)}
        searchPlaceholder="Search vessels"
        emptyMessage={vessels.length === 0 ? "No vessels enrolled yet." : "No vessels match the current filters."}
      />
    </div>
  );
}
