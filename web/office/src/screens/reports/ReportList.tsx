// SPDX-License-Identifier: AGPL-3.0-only

import { useEffect, useMemo, useState } from "react";
import { DataTable } from "../../design/components/data/DataTable.jsx";
import { AlertBanner } from "../../design/components/feedback/AlertBanner.jsx";
import { Button } from "../../design/components/core/Button.jsx";
import { TextField } from "../../design/components/forms/TextField.jsx";
import { api, type HealthView, type ReportListItemView, type ReportState } from "../../api/client";
import { StatePill } from "./ReportDetailScreen";

// Design handoff B3's compact health cell: "errors(✕)/warnings(⚠)/pass(✓)
// indicator," now always showing the severity icon (including a clean
// check for zero findings) rather than only showing an icon for the
// clean case — matches the 2026-08-02 Reports redesign mockup's
// icon+count treatment. Errors still take priority over warnings when
// both are present.
const HEALTH_STYLE: Record<"ok" | "warning" | "error", { icon: string; color: string }> = {
  ok: { icon: "check_circle", color: "var(--color-status-underway)" },
  warning: { icon: "warning", color: "var(--color-status-caution)" },
  error: { icon: "cancel", color: "var(--color-status-warning)" },
};

const HEALTH_TITLE: Record<"ok" | "warning" | "error", string> = {
  ok: "No findings",
  warning: "Warnings found",
  error: "Errors found",
};

export function HealthCell({ health }: { health: HealthView }) {
  const severity = health.errors > 0 ? "error" : health.warnings > 0 ? "warning" : "ok";
  const count = severity === "error" ? health.errors : severity === "warning" ? health.warnings : 0;
  const { icon, color } = HEALTH_STYLE[severity];
  return (
    <span className="md-body-medium" style={{ display: "inline-flex", alignItems: "center", gap: 4, color, fontWeight: 700 }}>
      {count > 0 ? count : null}
      <span className="material-symbols-rounded" style={{ fontSize: 18 }} title={HEALTH_TITLE[severity]}>{icon}</span>
    </span>
  );
}

interface SavedView {
  name: string;
  dateFrom?: string;
  dateTo?: string;
}

// Saved views persist to localStorage rather than a new office/store
// table (Phase 5 T6.2's own explicitly-sanctioned fallback) — a saved
// view is a pure per-browser UI convenience with no cross-side or audit
// implications. The 2026-08-02 Reports redesign narrowed what a saved
// view captures to just the date range: every other column filter is
// now purely client-side inside DataTable, which doesn't expose its own
// filter state for a parent to save/restore. Older localStorage entries
// (from before the redesign) had a `filter: ReportFilter` shape instead
// of `dateFrom`/`dateTo` — read defensively, since those fields simply
// come back `undefined` on an old entry rather than throwing.
const SAVED_VIEWS_KEY = "ovl.office.reportSavedViews";

function loadSavedViews(): SavedView[] {
  try {
    const raw = localStorage.getItem(SAVED_VIEWS_KEY);
    return raw ? (JSON.parse(raw) as SavedView[]) : [];
  } catch {
    return [];
  }
}

function persistSavedViews(views: SavedView[]): void {
  try {
    localStorage.setItem(SAVED_VIEWS_KEY, JSON.stringify(views));
  } catch {
    // Best-effort — saved views are a convenience, not load-bearing.
  }
}

// Design handoff B3: fleet-wide reports explorer, "same row anatomy as
// vessel A4 plus a vessel column." Redesigned 2026-08-02 from a T6.2
// filter-bar-drives-server-query model to "sort and filter from each
// column header": only the date range still narrows the server fetch
// (office/store.ReportFilter's dateFrom/dateTo) — vessel/event type/
// schema/state/remarks now filter purely client-side via DataTable's
// own filterable columns, matching what the screen already did by
// default whenever those fields were left blank.
export function ReportList({
  isReviewer,
  globalGroup,
  onOpenReport,
}: {
  isReviewer: boolean;
  /** App-level global group filter (top bar) — the only group scoping this screen has; DataTable's own column filters cover everything else. */
  globalGroup: string | null;
  onOpenReport: (vesselId: string, reportId: string) => void;
}) {
  const [reports, setReports] = useState<ReportListItemView[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [dateFrom, setDateFrom] = useState("");
  const [dateTo, setDateTo] = useState("");
  const [eventTimeSortDir, setEventTimeSortDir] = useState<"asc" | "desc" | null>(null);
  const [eventTimeFilterOpen, setEventTimeFilterOpen] = useState(false);
  const [savedViews, setSavedViews] = useState<SavedView[]>(() => loadSavedViews());
  const [savedViewsOpen, setSavedViewsOpen] = useState(false);
  const [newViewName, setNewViewName] = useState("");
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [visibleIds, setVisibleIds] = useState<string[]>([]);
  const [bulkBusy, setBulkBusy] = useState(false);
  const [bulkError, setBulkError] = useState<string | null>(null);

  function currentFilter() {
    return {
      group: globalGroup ?? undefined,
      dateFrom: dateFrom ? `${dateFrom}T00:00:00Z` : undefined,
      dateTo: dateTo ? `${dateTo}T23:59:59Z` : undefined,
    };
  }

  function reload() {
    setError(null);
    api
      .listReports(currentFilter())
      .then((list) => {
        setReports(list);
        setSelected(new Set());
      })
      .catch((err) => setError(err instanceof Error ? err.message : "Could not load reports."));
  }

  useEffect(() => {
    let cancelled = false;
    setError(null);
    api
      .listReports(currentFilter())
      .then((list) => {
        if (!cancelled) {
          setReports(list);
          setSelected(new Set());
        }
      })
      .catch((err) => {
        if (!cancelled) setError(err instanceof Error ? err.message : "Could not load reports.");
      });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [globalGroup, dateFrom, dateTo]);

  const anyFiltersActive = !!(dateFrom || dateTo);

  function clearFilters() {
    setDateFrom("");
    setDateTo("");
    setEventTimeSortDir(null);
  }

  function saveCurrentView() {
    const name = newViewName.trim();
    if (!name) return;
    const next = [...savedViews.filter((v) => v.name !== name), { name, dateFrom, dateTo }];
    setSavedViews(next);
    persistSavedViews(next);
    setNewViewName("");
  }

  function applyView(view: SavedView) {
    setDateFrom(view.dateFrom ?? "");
    setDateTo(view.dateTo ?? "");
    setSavedViewsOpen(false);
  }

  function deleteView(name: string) {
    const next = savedViews.filter((v) => v.name !== name);
    setSavedViews(next);
    persistSavedViews(next);
  }

  async function handleMarkReviewed() {
    if (!reports || selected.size === 0) return;
    setBulkBusy(true);
    setBulkError(null);
    try {
      const items = reports
        .filter((r) => selected.has(`${r.vesselId}/${r.reportId}`))
        .map((r) => ({ vesselId: r.vesselId, reportId: r.reportId }));
      await api.markReviewed(items);
      reload();
    } catch (err) {
      setBulkError(err instanceof Error ? err.message : "Could not mark reports reviewed.");
    } finally {
      setBulkBusy(false);
    }
  }

  function toggleSelected(id: string) {
    setSelected((prev) => {
      const next = new Set(prev);
      next.has(id) ? next.delete(id) : next.add(id);
      return next;
    });
  }

  const allVisibleSelected = visibleIds.length > 0 && visibleIds.every((id) => selected.has(id));
  function toggleSelectAll() {
    setSelected((prev) => {
      const next = new Set(prev);
      if (allVisibleSelected) visibleIds.forEach((id) => next.delete(id));
      else visibleIds.forEach((id) => next.add(id));
      return next;
    });
  }

  // Memoized so its reference stays stable across renders that don't
  // change reports/eventTimeSortDir — DataTable's own `filtered` memo
  // depends on this `rows` reference, and its onVisibleRowsChange effect
  // depends on that `filtered` memo in turn. An unmemoized array here
  // fed a fresh reference every render, which re-fired onVisibleRowsChange
  // -> setVisibleIds -> re-render, unbounded (mirrors the same trap
  // FieldPolicyScreen's tableRows/columns memoization already documents).
  // Also folds in the former standalone `sortedReports`: only the date
  // range narrows the server fetch, so this sort applies before
  // DataTable's own internal search/column-filter/sort pipeline sees the
  // rows, since a headerRender column (the Event time header below)
  // loses access to DataTable's internal sort state — clicking any other
  // column's built-in sort arrow still overrides this order exactly like
  // it always could. Hooks can't follow the early returns below, so this
  // has to sit above them and guard for `reports` being null itself.
  const tableRows = useMemo(() => {
    if (!reports) return [];
    const sorted = eventTimeSortDir
      ? [...reports].sort((a, b) => {
          const cmp = new Date(a.eventTime).getTime() - new Date(b.eventTime).getTime();
          return eventTimeSortDir === "asc" ? cmp : -cmp;
        })
      : reports;
    return sorted.map((r) => ({
      id: `${r.vesselId}/${r.reportId}`,
      vesselId: r.vesselId,
      reportId: r.reportId,
      vessel: { icon: "directions_boat", text: r.vesselName, subtext: r.vesselImo },
      eventType: r.eventType,
      schemaName: r.schemaName,
      state: r.state,
      resubmitted: r.versionNo > 1,
      health: r.health,
      hasRemarks: r.hasRemarks,
      remarksLabel: r.hasRemarks ? "Has remarks" : "No remarks",
      reviewed: r.reviewed,
      eventTime: new Date(r.eventTime).toLocaleString(),
    }));
  }, [reports, eventTimeSortDir]);

  if (error) {
    return (
      <div style={{ padding: 24 }}>
        <AlertBanner level="warning" title="Couldn't load reports" message={error} />
      </div>
    );
  }
  if (!reports) {
    return (
      <div className="md-body-medium" style={{ padding: 24, color: "var(--color-on-surface-variant)" }}>
        Loading reports…
      </div>
    );
  }

  const columns = [
    ...(isReviewer
      ? [
          {
            key: "select",
            label: "",
            type: "iconText" as const,
            headerRender: () => (
              <input
                type="checkbox"
                checked={allVisibleSelected}
                onChange={toggleSelectAll}
                aria-label="Select all visible reports"
              />
            ),
            render: (row: Record<string, unknown>) => (
              <input
                type="checkbox"
                checked={selected.has(row.id as string)}
                onChange={() => toggleSelected(row.id as string)}
                onClick={(e) => e.stopPropagation()}
              />
            ),
          },
        ]
      : []),
    { key: "vessel", label: "Vessel", type: "iconText" as const, sortable: true, filterable: true },
    { key: "eventType", label: "Event type", type: "text" as const, sortable: true, filterable: true },
    {
      key: "schemaName",
      label: "Schema",
      type: "text" as const,
      filterable: true,
      render: (row: Record<string, unknown>) => (
        <span className="md-body-medium" style={{ fontFamily: "var(--font-mono)" }}>{row.schemaName as string}</span>
      ),
    },
    {
      key: "state",
      label: "State",
      type: "badge" as const,
      filterable: true,
      render: (row: Record<string, unknown>) => <StatePill state={row.state as ReportState} resubmitted={row.resubmitted as boolean} />,
    },
    {
      key: "health",
      label: "Health",
      type: "text" as const,
      render: (row: Record<string, unknown>) => <HealthCell health={row.health as HealthView} />,
    },
    {
      key: "remarksLabel",
      label: "Remarks",
      type: "text" as const,
      filterable: true,
      render: (row: Record<string, unknown>) =>
        row.hasRemarks ? (
          <span className="material-symbols-rounded" title="Has remarks" style={{ fontSize: 18, color: "var(--color-tertiary)" }}>sticky_note_2</span>
        ) : (
          <span className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>—</span>
        ),
    },
    {
      key: "reviewed",
      label: "Reviewed",
      type: "symbol" as const,
      render: (row: Record<string, unknown>) =>
        row.reviewed ? <span className="material-symbols-rounded" title="Reviewed" style={{ color: "var(--color-primary)" }}>check_circle</span> : null,
    },
    {
      key: "eventTime",
      label: "Event time",
      type: "text" as const,
      headerRender: () => (
        <>
          <div style={{ display: "flex", alignItems: "center", gap: 4 }}>
            <span
              onClick={() => setEventTimeSortDir((d) => (d === null ? "asc" : d === "asc" ? "desc" : null))}
              style={{ display: "flex", alignItems: "center", gap: 4, cursor: "pointer", userSelect: "none" }}
            >
              Event time
              <span
                className="material-symbols-rounded"
                style={{ fontSize: 16, color: eventTimeSortDir ? "var(--color-primary)" : "var(--color-on-surface-variant)" }}
              >
                {eventTimeSortDir === "asc" ? "arrow_upward" : eventTimeSortDir === "desc" ? "arrow_downward" : "unfold_more"}
              </span>
            </span>
            <span
              onClick={() => setEventTimeFilterOpen((v) => !v)}
              className="material-symbols-rounded"
              style={{
                fontSize: 16, cursor: "pointer", borderRadius: "var(--shape-full)", width: 20, height: 20,
                display: "inline-flex", alignItems: "center", justifyContent: "center",
                color: anyFiltersActive ? "var(--color-primary)" : "var(--color-on-surface-variant)",
                background: anyFiltersActive ? "var(--color-primary-container)" : "transparent",
              }}
            >
              event
            </span>
          </div>
          {eventTimeFilterOpen ? (
            <>
              <div onClick={() => setEventTimeFilterOpen(false)} style={{ position: "fixed", inset: 0, zIndex: 20 }} />
              <div
                style={{
                  position: "absolute", top: "100%", left: 0, marginTop: 4, zIndex: 21, width: 220,
                  background: "var(--color-surface-container-high)", borderRadius: "var(--shape-small)",
                  boxShadow: "var(--elevation-2)", padding: "var(--space-3)", display: "flex", flexDirection: "column", gap: 10,
                }}
              >
                <TextField label="From" type="date" value={dateFrom} onChange={setDateFrom} style={{ width: "100%" }} />
                <TextField label="To" type="date" value={dateTo} onChange={setDateTo} style={{ width: "100%" }} />
                <button
                  onClick={clearFilters}
                  className="md-label-medium"
                  style={{ background: "none", border: "none", color: "var(--color-primary)", cursor: "pointer", padding: "4px 0", fontWeight: 600, textAlign: "left" }}
                >
                  Clear
                </button>
              </div>
            </>
          ) : null}
        </>
      ),
    },
  ];

  return (
    <div style={{ padding: 24, display: "flex", flexDirection: "column", gap: 16 }}>
      <div className="md-headline-small">Reports</div>

      {bulkError ? <AlertBanner level="warning" title="Bulk action failed" message={bulkError} onDismiss={() => setBulkError(null)} /> : null}

      <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
        <span className="md-body-small" style={{ color: "var(--color-on-surface-variant)" }}>Sort and filter from each column header.</span>
        {anyFiltersActive ? (
          <button onClick={clearFilters} className="md-label-large" style={{ background: "none", border: "none", color: "var(--color-primary)", cursor: "pointer", padding: "6px 4px", fontWeight: 600 }}>
            Clear filters
          </button>
        ) : null}
        <div style={{ position: "relative", marginLeft: "auto" }}>
          <button
            onClick={() => setSavedViewsOpen((v) => !v)}
            title="Saved views"
            style={{
              width: 40, height: 40, borderRadius: "var(--shape-full)", border: "1px solid var(--color-outline-variant)",
              background: savedViewsOpen || savedViews.length ? "var(--color-secondary-container)" : "transparent",
              color: savedViewsOpen || savedViews.length ? "var(--color-on-secondary-container)" : "var(--color-on-surface-variant)",
              cursor: "pointer", display: "flex", alignItems: "center", justifyContent: "center",
            }}
          >
            <span className="material-symbols-rounded">bookmark</span>
          </button>
          {savedViewsOpen ? (
            <>
              <div onClick={() => setSavedViewsOpen(false)} style={{ position: "fixed", inset: 0, zIndex: 20 }} />
              <div
                style={{
                  position: "absolute", top: "100%", right: 0, marginTop: 6, zIndex: 21, width: 280,
                  background: "var(--color-surface-container-high)", borderRadius: "var(--shape-medium)",
                  boxShadow: "var(--elevation-2)", padding: 14, display: "flex", flexDirection: "column", gap: 10,
                }}
              >
                <div className="md-label-medium" style={{ color: "var(--color-on-surface-variant)", textTransform: "uppercase", letterSpacing: "0.04em" }}>Saved views</div>
                {savedViews.length === 0 ? (
                  <span className="md-body-small" style={{ color: "var(--color-on-surface-variant)" }}>No saved views yet.</span>
                ) : (
                  savedViews.map((v) => (
                    <div
                      key={v.name}
                      onClick={() => applyView(v)}
                      style={{ display: "flex", alignItems: "center", gap: 8, padding: "8px 10px", borderRadius: "var(--shape-small)", cursor: "pointer" }}
                    >
                      <span className="md-body-medium" style={{ flex: 1 }}>{v.name}</span>
                      <button
                        onClick={(e) => {
                          e.stopPropagation();
                          deleteView(v.name);
                        }}
                        style={{ width: 24, height: 24, border: "none", background: "transparent", borderRadius: "var(--shape-full)", cursor: "pointer", color: "var(--color-on-surface-variant)", display: "flex", alignItems: "center", justifyContent: "center" }}
                      >
                        <span className="material-symbols-rounded" style={{ fontSize: 15 }}>close</span>
                      </button>
                    </div>
                  ))
                )}
                <div style={{ height: 1, background: "var(--color-outline-variant)", margin: "2px 0" }} />
                <TextField label="Save date range as" value={newViewName} onChange={setNewViewName} style={{ width: "100%" }} />
                <Button variant="filled" onClick={saveCurrentView} disabled={!newViewName.trim() || (!dateFrom && !dateTo)}>Save view</Button>
              </div>
            </>
          ) : null}
        </div>
      </div>

      {isReviewer && selected.size > 0 ? (
        <div style={{ display: "flex", alignItems: "center", gap: 12, padding: "10px 16px", borderRadius: "var(--shape-small)", background: "var(--color-secondary-container)", color: "var(--color-on-secondary-container)" }}>
          <span className="md-title-small">{selected.size} selected</span>
          <Button variant="filled" onClick={() => void handleMarkReviewed()} disabled={bulkBusy}>
            {bulkBusy ? "Marking…" : "Mark reviewed"}
          </Button>
          <button onClick={() => setSelected(new Set())} className="md-label-large" style={{ marginLeft: "auto", background: "none", border: "none", color: "inherit", cursor: "pointer", padding: "6px 4px", fontWeight: 600 }}>
            Clear selection
          </button>
        </div>
      ) : null}

      <DataTable
        columns={columns}
        rows={tableRows}
        onRowAction={(row) => onOpenReport(row.vesselId as string, row.reportId as string)}
        onVisibleRowsChange={(rows) => setVisibleIds(rows.map((r) => r.id as string))}
        searchPlaceholder="Search reports"
        emptyMessage={reports.length === 0 ? "No reports have synced from any vessel yet." : "No reports match the current filters."}
      />
    </div>
  );
}
