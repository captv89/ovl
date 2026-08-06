// SPDX-License-Identifier: AGPL-3.0-only

import { useEffect, useState } from "react";
import { Card } from "../../design/components/surfaces/Card.jsx";
import { AlertBanner } from "../../design/components/feedback/AlertBanner.jsx";
import { Select } from "../../design/components/forms/Select.jsx";
import { DataTable } from "../../design/components/data/DataTable.jsx";
import { api, ApiError, type DashboardView, type UserView } from "../../api/client";
import { formatOverdueDuration } from "../../format";
import type { Section } from "../AppShell";

const OPS_PERIOD_OPTIONS = [7, 30, 90, 180];

// Design handoff B1's dashboard widgets. Overdue vessels, Reporting
// compliance, Reports needing review, and Data quality (a 7-day error/
// warning trend, plain inline SVG — matching the wireframe's own markup
// rather than adding a chart library dependency) are unchanged. The
// fifth widget was originally "OVD/Veracity sync status" — dropped
// entirely (DNV declined Veracity API access, see architecture handoff
// section 13) rather than left as a dead tile; "Operations overview"
// (architecture 16: consumption + distance per vessel over a selectable
// period) takes its slot instead, since that was always the other real
// spec'd widget this screen never built. Reporting compliance is a live
// snapshot ("currently within cadence" / enrolled vessels), not a stored
// 7-day historical rate — no such time series exists yet, and this
// project's convention is to compute real metrics from data that
// actually exists rather than approximate a history that doesn't.
export function Dashboard({
  groupFilter,
  onNavigate,
}: {
  user: UserView;
  groupFilter: string | null;
  onNavigate: (section: Section) => void;
}) {
  const [data, setData] = useState<DashboardView | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [opsDays, setOpsDays] = useState(30);

  useEffect(() => {
    let cancelled = false;
    setData(null);
    setError(null);
    api
      .getDashboard(groupFilter, opsDays)
      .then((d) => {
        // Defensive, not just decorative: the backend now always sends
        // real arrays, but a client shouldn't crash the whole screen if
        // that ever regresses — this exact gap caused a live crash
        // before the backend fix landed.
        if (!cancelled)
          setData({
            ...d,
            overdueVessels: d.overdueVessels ?? [],
            dataQualityTrend: d.dataQualityTrend ?? [],
            operationsOverview: d.operationsOverview ?? [],
          });
      })
      .catch((err) => {
        if (!cancelled) setError(err instanceof ApiError ? err.message : "Could not load the dashboard.");
      });
    return () => {
      cancelled = true;
    };
  }, [groupFilter, opsDays]);

  if (error) {
    return (
      <div style={{ padding: 24 }}>
        <AlertBanner level="warning" title="Couldn't load the dashboard" message={error} />
      </div>
    );
  }
  if (!data) {
    return (
      <div className="md-body-medium" style={{ padding: 24, color: "var(--color-on-surface-variant)" }}>
        Loading…
      </div>
    );
  }

  return (
    <div style={{ padding: 24, display: "flex", flexDirection: "column", gap: 20 }}>
      <div className="md-headline-small">Fleet Overview</div>

      <div style={{ display: "flex", gap: 16, flexWrap: "wrap" }}>
        <KpiTile
          label="Overdue vessels"
          value={String(data.overdueVesselCount)}
          sublabel={`of ${data.enrolledVesselCount} enrolled`}
          alert={data.overdueVesselCount > 0}
        />
        <KpiTile label="Reporting compliance" value={`${data.compliancePercent.toFixed(0)}%`} sublabel="currently within cadence" />
        <KpiTile label="Reports needing review" value={String(data.reportsNeedingReview)} sublabel="remarked / unreviewed" />
      </div>

      <div>
        <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 12 }}>
          <span className="material-symbols-rounded" style={{ color: "var(--color-status-warning)" }}>warning</span>
          <span className="md-title-medium">Overdue vessels</span>
          <span className="grow" style={{ flex: 1 }} />
          <button
            onClick={() => onNavigate("vessels")}
            className="md-body-small"
            style={{ border: "none", background: "transparent", color: "var(--color-primary)", cursor: "pointer" }}
          >
            View all in Vessels ›
          </button>
        </div>
        {data.overdueVessels.length === 0 ? (
          <Card variant="outlined" style={{ padding: 16 }}>
            <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
              No overdue vessels.
            </div>
          </Card>
        ) : (
          <Card variant="outlined" style={{ padding: 0, overflow: "hidden" }}>
            {data.overdueVessels.map((v, i) => (
              <div
                key={v.vesselId}
                style={{
                  display: "flex", alignItems: "center", gap: 16, padding: "10px 16px",
                  borderTop: i === 0 ? "none" : "1px solid var(--color-outline-variant)",
                  background: "var(--color-error-container)",
                }}
              >
                <span className="material-symbols-rounded" style={{ fontSize: 18, color: "var(--color-on-surface-variant)" }}>directions_boat</span>
                <div style={{ flex: 1.4 }}>
                  <div className="md-body-medium" style={{ color: "var(--color-on-error-container)" }}>{v.vesselName}</div>
                  <div className="md-label-small" style={{ color: "var(--color-on-surface-variant)" }}>{v.vesselImo}</div>
                </div>
                <div style={{ flex: 1 }} className="md-body-small">{v.groups.join(", ") || "—"}</div>
                <div style={{ flex: 1.3 }} className="md-body-small">Last report {new Date(v.lastReportAt).toLocaleString()}</div>
                <div className="md-body-medium mono" style={{ width: 100, textAlign: "right", color: "var(--color-error)", fontWeight: 700 }}>
                  {formatOverdueDuration(v.overdueHours)}
                </div>
              </div>
            ))}
          </Card>
        )}
      </div>

      <div>
        <div className="md-title-medium" style={{ marginBottom: 2 }}>Data quality</div>
        <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)", marginBottom: 12 }}>
          Errors + warnings per day · last 7 days
        </div>
        <Card variant="outlined" style={{ padding: 16 }}>
          <DataQualityChart trend={data.dataQualityTrend} />
        </Card>
      </div>

      <div>
        <div style={{ display: "flex", alignItems: "center", gap: 12, marginBottom: 12 }}>
          <span className="md-title-medium">Operations overview</span>
          <span className="grow" style={{ flex: 1 }} />
          <Select
            label="Period"
            value={String(opsDays)}
            options={OPS_PERIOD_OPTIONS.map(String)}
            onChange={(v) => setOpsDays(Number(v))}
            style={{ width: 140 }}
          />
        </div>
        {data.operationsOverview.length === 0 ? (
          <Card variant="outlined" style={{ padding: 16 }}>
            <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
              No Log Abstract reports in the last {opsDays} days.
            </div>
          </Card>
        ) : (
          <DataTable
            columns={[
              { key: "vessel", label: "Vessel", type: "iconText", sortable: true },
              { key: "distance", label: "Distance", type: "text", sortable: true },
              { key: "consumption", label: "Consumption", type: "text", sortable: true },
              { key: "reportCount", label: "Reports", type: "number", sortable: true },
            ]}
            rows={data.operationsOverview.map((row) => ({
              id: row.vesselId,
              vessel: { icon: "directions_boat", text: row.vesselName, subtext: row.vesselImo },
              distance: `${row.totalDistanceNm.toFixed(1)} NM`,
              consumption: `${row.totalConsumptionMt.toFixed(1)} mt`,
              reportCount: row.reportCount,
            }))}
            hideSearch
            emptyMessage="No data for this period."
          />
        )}
      </div>
    </div>
  );
}

export function KpiTile({ label, value, sublabel, alert = false }: { label: string; value: string; sublabel: string; alert?: boolean }) {
  return (
    <Card
      variant="outlined"
      style={{
        flex: 1, minWidth: 180, padding: "14px 16px",
        borderColor: alert ? "var(--color-error)" : undefined,
        background: alert ? "var(--color-error-container)" : undefined,
      }}
    >
      <div className="md-label-medium" style={{ color: alert ? "var(--color-on-error-container)" : "var(--color-on-surface-variant)" }}>{label}</div>
      <div className="md-headline-medium mono" style={{ margin: "4px 0 2px", color: alert ? "var(--color-error)" : "var(--color-on-surface)" }}>{value}</div>
      <div className="md-label-small" style={{ color: alert ? "var(--color-on-error-container)" : "var(--color-on-surface-variant)" }}>{sublabel}</div>
    </Card>
  );
}

// Plain inline SVG bars — matching the wireframe's own markup, no chart
// library dependency for a single 7-bar sparkline.
function DataQualityChart({ trend }: { trend: DashboardView["dataQualityTrend"] }) {
  const max = Math.max(1, ...trend.map((p) => p.errors + p.warnings));
  const barWidth = 100 / trend.length;
  return (
    <div>
      <svg viewBox="0 0 100 40" preserveAspectRatio="none" style={{ width: "100%", height: 96, display: "block" }}>
        {trend.map((p, i) => {
          const total = p.errors + p.warnings;
          const height = (total / max) * 38;
          const errorHeight = (p.errors / max) * 38;
          const x = i * barWidth + barWidth * 0.15;
          const width = barWidth * 0.7;
          return (
            <g key={p.date}>
              <rect x={x} y={40 - height} width={width} height={height} fill="var(--color-status-caution)" />
              {p.errors > 0 ? <rect x={x} y={40 - errorHeight} width={width} height={errorHeight} fill="var(--color-error)" /> : null}
            </g>
          );
        })}
      </svg>
      <div style={{ display: "flex", justifyContent: "space-between", marginTop: 6 }}>
        {trend.map((p) => (
          <span key={p.date} className="md-label-small mono" style={{ color: "var(--color-on-surface-variant)" }}>
            {p.date.slice(5)}
          </span>
        ))}
      </div>
    </div>
  );
}
