// SPDX-License-Identifier: AGPL-3.0-only

import React from "react";

/**
 * WeatherVane — top-down vessel silhouette with live wind/sea-state/swell/
 * current vectors, each drawn from the direction it's coming FROM toward
 * the hull. All four OVD direction fields this reads (Wind_Dir_Degree,
 * Sea_state_Dir_Degree, Swell_Dir_Degree, Current_Dir_Degree) are defined
 * relative to the vessel itself (0° = dead ahead, 90° = starboard beam,
 * 180° = astern, 270° = port beam — confirmed by the OVD field
 * descriptions, e.g. "rel. to the vessel, i.e. 90° is wind from SB side"),
 * so the vessel silhouette can stay bow-up with no heading/course
 * correction needed. Arrow length encodes intensity, normalized per field
 * against a scale-appropriate max (Beaufort 12, Douglas 9, swell height and
 * current speed capped at typical extremes) — a stronger reading reaches
 * closer to the hull rather than stopping short at the rim.
 *
 * Also draws the 8 compass sectors (Wind_Dir/Sea_state_Dir/Swell_Dir/
 * Current_Dir's own "1-8" fields) as labeled wedges — sector 1 centered
 * dead ahead (0°), running clockwise in 45° steps, boundaries at
 * 22.5°+n·45°. This is the actual DNV reference diagram the xlsx's
 * "as shown in graph" note points to (the user supplied it directly,
 * 2026-07-05) — an earlier version of this component deliberately left
 * the sectors undrawn because that graph hadn't been available yet, not
 * because the mapping was guessed.
 */
const CENTER = 100;
const RING_R = 82;
const HULL_R = 34;
const SECTOR_LABEL_R = RING_R + 12;
const SECTOR_BOUNDARY_DEGREES = [22.5, 67.5, 112.5, 157.5];
const SECTOR_LABELS = [1, 2, 3, 4, 5, 6, 7, 8]; // sector n is centered at (n-1)*45°

const VECTORS = [
  { key: "wind", label: "Wind", color: "var(--color-primary)", unit: "Bft", max: 12, dashed: false },
  { key: "seaState", label: "Sea state", color: "var(--color-secondary)", unit: "Douglas", max: 9, dashed: false },
  { key: "swell", label: "Swell", color: "var(--color-tertiary)", unit: "m", max: 12, dashed: false },
  { key: "current", label: "Current", color: "var(--color-on-surface-variant)", unit: "kn", max: 6, dashed: true },
];

function toNumber(v) {
  if (v === undefined || v === null || v === "") return null;
  const n = typeof v === "number" ? v : parseFloat(v);
  return Number.isFinite(n) ? n : null;
}

function Vector({ angleDeg, intensity, max, color, dashed }) {
  if (angleDeg === null) return null;
  const t = Math.max(0, Math.min(1, (intensity ?? 0) / max));
  const innerR = RING_R - t * (RING_R - HULL_R);
  const apexY = CENTER - innerR + 7;
  const baseY = CENTER - innerR - 5;
  return (
    <g transform={`rotate(${angleDeg} ${CENTER} ${CENTER})`}>
      <line
        x1={CENTER} y1={CENTER - RING_R} x2={CENTER} y2={CENTER - innerR}
        stroke={color} strokeWidth={2.5} strokeLinecap="round"
        strokeDasharray={dashed ? "5 4" : undefined}
      />
      <polygon points={`${CENTER},${apexY} ${CENTER - 5},${baseY} ${CENTER + 5},${baseY}`} fill={color} />
    </g>
  );
}

export function WeatherVane({
  windDirDeg, windForceBft,
  seaDirDeg, seaForceDouglas,
  swellDirDeg, swellForceM,
  currentDirDeg, currentSpeedKn,
  size = 220,
}) {
  const readings = {
    wind: { dir: toNumber(windDirDeg), intensity: toNumber(windForceBft) },
    seaState: { dir: toNumber(seaDirDeg), intensity: toNumber(seaForceDouglas) },
    swell: { dir: toNumber(swellDirDeg), intensity: toNumber(swellForceM) },
    current: { dir: toNumber(currentDirDeg), intensity: toNumber(currentSpeedKn) },
  };

  return (
    <div style={{ display: "flex", flexDirection: "column", alignItems: "center", gap: 8, width: size }}>
      <svg width={size} height={size} viewBox="0 0 200 200">
        <circle cx={CENTER} cy={CENTER} r={RING_R} fill="var(--color-surface-container-low)" stroke="var(--color-outline-variant)" strokeWidth="1" />

        {/* Sector dividers: 4 full diameters at the 8 boundary angles
            (each line covers two opposite boundaries at once), forming
            the 8 wedges the DNV reference diagram shows. */}
        {SECTOR_BOUNDARY_DEGREES.map((deg) => (
          <line
            key={deg}
            x1={CENTER} y1={CENTER - RING_R} x2={CENTER} y2={CENTER + RING_R}
            stroke="var(--color-outline-variant)"
            strokeWidth={1}
            transform={`rotate(${deg} ${CENTER} ${CENTER})`}
          />
        ))}
        {/* Sector numbers, 1 (dead ahead) through 8 clockwise, each
            centered in its own 45° wedge just outside the ring — so the
            crew can read off which numbered sector a bearing falls in
            without needing to know the underlying degrees. */}
        {SECTOR_LABELS.map((n) => {
          const deg = (n - 1) * 45;
          const rad = (deg * Math.PI) / 180;
          const x = CENTER + SECTOR_LABEL_R * Math.sin(rad);
          const y = CENTER - SECTOR_LABEL_R * Math.cos(rad);
          return (
            <text
              key={n}
              x={x}
              y={y}
              textAnchor="middle"
              dominantBaseline="middle"
              fontSize="10"
              fontWeight="600"
              fontFamily="var(--font-mono)"
              fill="var(--color-on-surface-variant)"
            >
              {n}
            </text>
          );
        })}

        {[0, 90, 180, 270].map((deg) => (
          <line
            key={deg}
            x1={CENTER} y1={CENTER - RING_R} x2={CENTER} y2={CENTER - RING_R + 8}
            stroke="var(--color-on-surface-variant)"
            strokeWidth={2}
            transform={`rotate(${deg} ${CENTER} ${CENTER})`}
          />
        ))}
        <text x={CENTER} y={CENTER - RING_R + 22} textAnchor="middle" fontSize="8" fontFamily="var(--font-mono)" fill="var(--color-on-surface)">BOW</text>
        <text x={CENTER + RING_R - 20} y={CENTER + 3} textAnchor="middle" fontSize="8" fontFamily="var(--font-mono)" fill="var(--color-on-surface-variant)">STBD</text>
        <text x={CENTER} y={CENTER + RING_R - 12} textAnchor="middle" fontSize="8" fontFamily="var(--font-mono)" fill="var(--color-on-surface-variant)">STERN</text>
        <text x={CENTER - RING_R + 20} y={CENTER + 3} textAnchor="middle" fontSize="8" fontFamily="var(--font-mono)" fill="var(--color-on-surface-variant)">PORT</text>

        {VECTORS.map(({ key, color, max, dashed }) => (
          <Vector key={key} angleDeg={readings[key].dir} intensity={readings[key].intensity} max={max} color={color} dashed={dashed} />
        ))}

        <path
          d="M100,58 C112,80 118,108 114,132 C114,140 108,146 100,146 C92,146 86,140 86,132 C82,108 88,80 100,58 Z"
          fill="var(--color-surface-container-high)"
          stroke="var(--color-outline-variant)"
          strokeWidth="1.5"
        />
        <rect x="91" y="117" width="18" height="15" rx="2" fill="var(--color-surface-container-highest)" stroke="var(--color-outline-variant)" strokeWidth="1" />
      </svg>
      <div style={{ display: "flex", flexDirection: "column", gap: 4, width: "100%" }}>
        {VECTORS.map(({ key, label, color, unit }) => {
          const r = readings[key];
          return (
            <div key={key} style={{ display: "flex", alignItems: "center", gap: 6 }}>
              <span style={{ width: 10, height: 10, borderRadius: "var(--shape-full)", background: color, flexShrink: 0 }} />
              <span className="md-body-small" style={{ color: "var(--color-on-surface-variant)" }}>
                {label}{" "}
                {r.dir !== null
                  ? `— ${String(Math.round(r.dir)).padStart(3, "0")}°${r.intensity !== null ? `, ${r.intensity} ${unit}` : ""}`
                  : "— no data"}
              </span>
            </div>
          );
        })}
      </div>
    </div>
  );
}
