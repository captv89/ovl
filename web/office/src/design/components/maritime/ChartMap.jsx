// SPDX-License-Identifier: AGPL-3.0-only

import React from "react";

/**
 * ChartMap — schematic nautical chart panel: graticule, coastline silhouette,
 * vessel markers and route track. A functional stand-in for a live ECDIS/chart
 * tile layer, not a rendering of real chart data.
 *
 * Vessel marker color follows the design system's own fixed bridge-alert
 * status convention (green underway/OK, amber caution, red alert — see
 * readme.md's "Status semantics"): pass `status` ("ok" | "caution" | "alert")
 * per vessel. The older boolean `alert` still works as a red/navy-only
 * shorthand for callers that don't need the three-way distinction — `status`
 * wins when both are given.
 */
export function ChartMap({ vessels = [], track = [], width = 480, height = 300, style, onVesselClick }) {
  function colorFor(v) {
    const status = v.status || (v.alert ? "alert" : undefined);
    if (status === "alert") return "var(--color-error)";
    if (status === "caution") return "var(--color-status-caution)";
    if (status === "ok") return "var(--color-status-underway)";
    return "var(--color-primary)";
  }

  return (
    <div style={{ position: "relative", width, height, borderRadius: "var(--shape-medium)", overflow: "hidden", background: "var(--color-primary-container)", border: "1px solid var(--color-outline-variant)", ...style }}>
      <svg width={width} height={height} style={{ position: "absolute", inset: 0 }}>
        {/* graticule */}
        {Array.from({ length: 6 }).map((_, i) => (
          <line key={"h" + i} x1={0} y1={(height / 6) * i} x2={width} y2={(height / 6) * i} stroke="var(--color-on-primary-container)" strokeOpacity="0.12" />
        ))}
        {Array.from({ length: 8 }).map((_, i) => (
          <line key={"v" + i} x1={(width / 8) * i} y1={0} x2={(width / 8) * i} y2={height} stroke="var(--color-on-primary-container)" strokeOpacity="0.12" />
        ))}
        {/* coastline silhouette (schematic landmass, lower-left) */}
        <path
          d={`M0,${height} L0,${height * 0.62} Q ${width * 0.18},${height * 0.5} ${width * 0.3},${height * 0.68} T ${width * 0.42},${height * 0.8} L ${width * 0.4},${height} Z`}
          fill="var(--color-tertiary-container)"
          stroke="var(--color-tertiary)"
          strokeOpacity="0.4"
        />
        {/* route track */}
        {track.length > 1 ? (
          <polyline
            points={track.map((p) => `${p.x * width},${p.y * height}`).join(" ")}
            fill="none"
            stroke="var(--color-secondary)"
            strokeWidth="2"
            strokeDasharray="6 4"
          />
        ) : null}
        {/* vessel markers */}
        {vessels.map((v, i) => (
          <g
            key={i}
            transform={`translate(${v.x * width}, ${v.y * height}) rotate(${v.heading || 0})`}
            onClick={onVesselClick ? () => onVesselClick(i) : undefined}
            style={onVesselClick ? { cursor: "pointer" } : undefined}
          >
            {onVesselClick ? <title>{v.label}</title> : null}
            <polygon points="0,-8 5,7 0,4 -5,7" fill={colorFor(v)} stroke="var(--color-surface)" strokeWidth="1" />
          </g>
        ))}
      </svg>
      {vessels.map((v, i) => (
        <div
          key={i}
          onClick={onVesselClick ? () => onVesselClick(i) : undefined}
          style={{ position: "absolute", left: v.x * width + 10, top: v.y * height - 8, pointerEvents: onVesselClick ? "auto" : "none", cursor: onVesselClick ? "pointer" : undefined }}
        >
          <span className="md-label-small" style={{ background: "var(--color-surface)", padding: "1px 4px", borderRadius: 3, color: "var(--color-on-surface)" }}>{v.label}</span>
        </div>
      ))}
    </div>
  );
}
