// SPDX-License-Identifier: AGPL-3.0-only

import React from "react";

/**
 * DataReadout — instrument-style labeled numeric value (speed, depth, wind, etc).
 */
export function DataReadout({ label, value, unit = "", size = "medium", trend = null }) {
  const cls = size === "large" ? "md-readout-large" : "md-readout-medium";
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 2 }}>
      <span className="md-label-medium" style={{ color: "var(--color-on-surface-variant)", textTransform: "uppercase", letterSpacing: 1 }}>{label}</span>
      <div style={{ display: "flex", alignItems: "baseline", gap: 4 }}>
        <span className={cls} style={{ color: "var(--color-on-surface)" }}>{value}</span>
        {unit ? <span className="md-body-small" style={{ color: "var(--color-on-surface-variant)" }}>{unit}</span> : null}
        {trend ? (
          <span className="material-symbols-rounded" style={{ fontSize: 16, color: trend === "up" ? "var(--color-status-warning)" : "var(--color-status-underway)" }}>
            {trend === "up" ? "arrow_upward" : "arrow_downward"}
          </span>
        ) : null}
      </div>
    </div>
  );
}
