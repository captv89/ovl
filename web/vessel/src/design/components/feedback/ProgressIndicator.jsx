// SPDX-License-Identifier: AGPL-3.0-only

import React from "react";

export function ProgressIndicator({ variant = "linear", value = null, size = 40 }) {
  if (variant === "circular") {
    const r = (size - 4) / 2;
    const c = 2 * Math.PI * r;
    const pct = value == null ? 0.25 : value / 100;
    return (
      <svg width={size} height={size} style={{ animation: value == null ? "spin 1.2s linear infinite" : "none" }}>
        <circle cx={size / 2} cy={size / 2} r={r} stroke="var(--color-surface-container-highest)" strokeWidth="4" fill="none" />
        <circle
          cx={size / 2}
          cy={size / 2}
          r={r}
          stroke="var(--color-primary)"
          strokeWidth="4"
          fill="none"
          strokeDasharray={c}
          strokeDashoffset={c * (1 - pct)}
          strokeLinecap="round"
          transform={`rotate(-90 ${size / 2} ${size / 2})`}
        />
        <style>{"@keyframes spin { to { transform: rotate(360deg); } }"}</style>
      </svg>
    );
  }
  return (
    <div style={{ width: "100%", height: 4, borderRadius: 2, background: "var(--color-surface-container-highest)", overflow: "hidden" }}>
      <div style={{ height: "100%", width: value == null ? "40%" : `${value}%`, background: "var(--color-primary)", borderRadius: 2, animation: value == null ? "indeterminate 1.4s ease-in-out infinite" : "none" }} />
      <style>{"@keyframes indeterminate { 0% { margin-left: -40%; } 100% { margin-left: 100%; } }"}</style>
    </div>
  );
}
