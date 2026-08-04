// SPDX-License-Identifier: AGPL-3.0-only

import { useEffect, useState } from "react";
import { Card } from "../../design/components/surfaces/Card.jsx";
import { AlertBanner } from "../../design/components/feedback/AlertBanner.jsx";
import { api, type SystemView } from "../../api/client";

function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B";
  const units = ["B", "KB", "MB", "GB"];
  const i = Math.min(units.length - 1, Math.floor(Math.log(bytes) / Math.log(1024)));
  return `${(bytes / 1024 ** i).toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

// Design handoff B10's System tab. Rebuilt (manual-test review item 8:
// the three-flat-KpiTile version "looks very unprofessional") into a
// real status board — an icon-led card per subsystem instead of copies
// of Dashboard's own KPI-tile widget, plus an explicit, honestly-worded
// row for the one subsystem this office instance genuinely has no
// signal for. Still real values only: no job-queue/background-worker
// health, since River isn't wired yet (PROJECT.md's Phase 4-6 status) —
// StatusRow's own "not wired yet" tone makes that gap visible rather
// than silently omitting the row, which used to read as "forgotten"
// rather than "not built."
export function SystemTab() {
  const [data, setData] = useState<SystemView | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api.getSystem().then(setData).catch((err) => setError(err instanceof Error ? err.message : "Could not load system status."));
  }, []);

  if (error) {
    return <AlertBanner level="warning" title="Couldn't load system status" message={error} />;
  }
  if (!data) {
    return (
      <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
        Loading…
      </div>
    );
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 20, maxWidth: 720 }}>
      <div>
        <div className="md-title-medium">System status</div>
        <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)" }}>
          Live, read from this office instance directly — nothing here is a stored snapshot.
        </div>
      </div>

      <Card variant="outlined" style={{ padding: 0, overflow: "hidden" }}>
        <StatusRow
          icon="dns"
          label="ovl-office"
          value={data.version}
          sublabel="build version"
          tone="neutral"
        />
        <StatusRow
          icon="database"
          label="Database"
          value={data.databaseReachable ? "Reachable" : "Unreachable"}
          sublabel="PostgreSQL connectivity"
          tone={data.databaseReachable ? "ok" : "error"}
        />
        <StatusRow
          icon="folder_open"
          label="Attachment store"
          value={formatBytes(data.attachmentStoreBytes)}
          sublabel={`${data.attachmentStoreCount} file${data.attachmentStoreCount === 1 ? "" : "s"} on disk`}
          tone="neutral"
        />
        <StatusRow
          icon="schedule"
          label="Background jobs"
          value="Not wired yet"
          sublabel="River job queue is planned (Phase 4-6) but not connected — no health signal to show here yet"
          tone="unknown"
          last
        />
      </Card>
    </div>
  );
}

const TONE_COLOR: Record<string, string> = {
  ok: "var(--color-status-underway)",
  error: "var(--color-error)",
  neutral: "var(--color-on-surface-variant)",
  unknown: "var(--color-on-surface-variant)",
};

function StatusRow({
  icon,
  label,
  value,
  sublabel,
  tone,
  last = false,
}: {
  icon: string;
  label: string;
  value: string;
  sublabel: string;
  tone: "ok" | "error" | "neutral" | "unknown";
  last?: boolean;
}) {
  return (
    <div
      style={{
        display: "flex", alignItems: "center", gap: 16, padding: "16px 20px",
        borderBottom: last ? "none" : "1px solid var(--color-outline-variant)",
        opacity: tone === "unknown" ? 0.75 : 1,
      }}
    >
      <span
        className="material-symbols-rounded"
        style={{
          fontSize: 22, color: TONE_COLOR[tone],
          width: 40, height: 40, borderRadius: "var(--shape-full)",
          background: "var(--color-surface-container-highest)",
          display: "flex", alignItems: "center", justifyContent: "center", flexShrink: 0,
        }}
      >
        {icon}
      </span>
      <div style={{ flex: 1, minWidth: 0 }}>
        <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>{label}</div>
        <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)" }}>{sublabel}</div>
      </div>
      <div className="md-title-medium mono" style={{ color: TONE_COLOR[tone], textAlign: "right", flexShrink: 0 }}>
        {value}
      </div>
    </div>
  );
}
