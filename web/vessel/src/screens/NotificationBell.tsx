// SPDX-License-Identifier: AGPL-3.0-only

import { useEffect, useState } from "react";
import { api } from "../api/client";
import { LISTED_SCHEMAS } from "./reports/ReportsScreen";
import { schemaDisplayName } from "./report-form/ReportForm";
import type { Section } from "./AppShell";
import { cadenceStatus, formatDurationShort, lastSubmittedReport } from "./home/homeData";

type FlagCategory = "remarked" | "invalidated" | "overdue";

interface FlaggedReport {
  schemaName: string;
  reportId: string;
  eventType: string;
  state: FlagCategory;
  updatedAt: string;
}

const CATEGORY_ICON: Record<FlagCategory, string> = {
  remarked: "forum",
  invalidated: "report",
  overdue: "schedule",
};

const CATEGORY_COLOR: Record<FlagCategory, string> = {
  remarked: "var(--color-status-caution)",
  invalidated: "var(--color-error)",
  overdue: "var(--color-error)",
};

const CATEGORY_LABEL: Record<FlagCategory, string> = {
  remarked: "Remarked by office",
  invalidated: "Invalidated",
  overdue: "Report overdue",
};

// The vessel-side counterpart to office's own NotificationPanel (which
// this deliberately mirrors — bell, unread-style badge, anchored
// popover, deep-link to a *section* not a specific report, same
// reasoning as that component's own doc comment on why: opening one
// exact report from outside the Reports screen's own navigation state
// would need a real router, out of scope here). Vessel had no
// equivalent at all (manual-test review item 1) — every report the
// vessel needs to act on (remarked or invalidated by office review)
// only ever surfaced in Home's own "Needs attention" section, nowhere
// else in the persistent chrome.
//
// Unlike office's notifications, there is no "read" state here: a
// flagged report isn't a discrete event to dismiss, it's this report's
// *current* lifecycle state (architecture 8.1) — it stops appearing the
// moment the report is actually corrected, not when someone clicks
// "mark read" on it. A separate read-tracking table would just be a
// second, driftable copy of the same fact SaveReport already owns.
export function NotificationBell({ onNavigate }: { onNavigate: (section: Section) => void }) {
  const [open, setOpen] = useState(false);
  const [flagged, setFlagged] = useState<FlaggedReport[] | null>(null);

  const reload = () => {
    Promise.all(LISTED_SCHEMAS.map((name) => api.listReports(name)))
      .then((lists) => {
        const rows: FlaggedReport[] = [];
        for (const list of lists) {
          for (const r of list) {
            if (r.state === "remarked" || r.state === "invalidated") {
              rows.push({ schemaName: r.schemaName, reportId: r.reportId, eventType: r.eventType, state: r.state, updatedAt: r.updatedAt });
            }
          }
        }
        // 18.07.26 manual-test item 14: overdue cadence (Home's own banner,
        // dismissable there — see home/overdueDismissal.ts) also surfaces
        // here regardless of whether it's dismissed on Home, so it's still
        // reachable from "the notification section" as asked. Reuses
        // Home's own cadenceStatus/lastSubmittedReport — LISTED_SCHEMAS[0]
        // is log-abstract, the only schema with a cadence concept.
        const logAbstract = lists[0] ?? [];
        const last = lastSubmittedReport(logAbstract);
        const cadence = cadenceStatus(last?.eventTime, new Date());
        if (cadence.kind === "overdue" && last) {
          rows.push({
            schemaName: last.schemaName,
            reportId: last.reportId,
            eventType: `Overdue by ${formatDurationShort(Date.now() - cadence.dueAt.getTime())}`,
            state: "overdue",
            updatedAt: cadence.dueAt.toISOString(),
          });
        }
        rows.sort((a, b) => b.updatedAt.localeCompare(a.updatedAt));
        setFlagged(rows);
      })
      .catch(() => {
        // Best-effort — the bell just shows nothing rather than an error banner.
      });
  };

  useEffect(reload, []);
  // Same light-polling rationale as office's NotificationPanel: keeps the
  // badge fresh across a sync pull without a websocket/SSE channel.
  useEffect(() => {
    const id = window.setInterval(reload, 60_000);
    return () => window.clearInterval(id);
  }, []);

  const count = flagged?.length ?? 0;

  return (
    <div style={{ position: "relative" }}>
      <button
        onClick={() => setOpen((v) => !v)}
        aria-label="Notifications"
        style={{
          position: "relative", width: 40, height: 40, border: "none", background: "transparent",
          borderRadius: "var(--shape-full)", cursor: "pointer", display: "flex", alignItems: "center", justifyContent: "center",
          color: "var(--color-on-surface-variant)",
        }}
      >
        <span className="material-symbols-rounded">notifications</span>
        {count > 0 ? (
          <span
            className="md-label-small"
            style={{
              position: "absolute", top: 2, right: 2, minWidth: 15, height: 15, padding: "0 3px",
              borderRadius: "var(--shape-full)", background: "var(--color-error)", color: "var(--color-on-error)",
              fontSize: 9, fontWeight: 700, display: "flex", alignItems: "center", justifyContent: "center",
            }}
          >
            {count > 9 ? "9+" : count}
          </span>
        ) : null}
      </button>

      {open ? (
        <>
          <div onClick={() => setOpen(false)} style={{ position: "fixed", inset: 0, zIndex: 20 }} />
          <div
            style={{
              position: "absolute", top: "100%", right: 0, marginTop: 4, zIndex: 21,
              width: 380, maxHeight: 480, display: "flex", flexDirection: "column", overflow: "hidden",
              background: "var(--color-surface-container-high)", borderRadius: "var(--shape-medium)", boxShadow: "var(--elevation-2)",
            }}
          >
            <div style={{ display: "flex", alignItems: "center", gap: 8, padding: "13px 16px", borderBottom: "1px solid var(--color-outline-variant)" }}>
              <span className="md-title-medium">Notifications</span>
              {count > 0 ? (
                <span className="md-label-small" style={{ background: "var(--color-secondary-container)", color: "var(--color-on-secondary-container)", borderRadius: "var(--shape-full)", padding: "1px 7px" }}>
                  {count} need action
                </span>
              ) : null}
            </div>

            <div style={{ overflowY: "auto", flex: 1 }}>
              {flagged === null || flagged.length === 0 ? (
                <div className="md-body-medium" style={{ padding: 24, textAlign: "center", color: "var(--color-on-surface-variant)" }}>
                  {flagged === null ? "Loading…" : "Nothing needs your attention."}
                </div>
              ) : (
                flagged.map((r) => (
                  <button
                    key={`${r.schemaName}:${r.reportId}`}
                    onClick={() => {
                      setOpen(false);
                      onNavigate("reports");
                    }}
                    style={{
                      display: "flex", gap: 11, width: "100%", padding: "11px 16px", border: "none",
                      borderTop: "1px solid var(--color-outline-variant)", background: "transparent",
                      cursor: "pointer", textAlign: "left", font: "inherit",
                    }}
                  >
                    <span className="material-symbols-rounded" style={{ fontSize: 20, color: CATEGORY_COLOR[r.state], flexShrink: 0, marginTop: 1 }}>
                      {CATEGORY_ICON[r.state]}
                    </span>
                    <div style={{ flex: 1, minWidth: 0 }}>
                      <div className="md-body-medium" style={{ color: "var(--color-on-surface)" }}>
                        {schemaDisplayName(r.schemaName)} · {r.eventType}
                      </div>
                      <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)" }}>
                        {CATEGORY_LABEL[r.state]}
                      </div>
                      <div className="md-label-small" style={{ color: "var(--color-on-surface-variant)", marginTop: 2 }}>
                        {new Date(r.updatedAt).toLocaleString()}
                      </div>
                    </div>
                  </button>
                ))
              )}
            </div>
          </div>
        </>
      ) : null}
    </div>
  );
}
