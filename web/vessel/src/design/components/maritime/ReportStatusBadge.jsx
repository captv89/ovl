// SPDX-License-Identifier: AGPL-3.0-only

import React from "react";

/**
 * ReportStatusBadge — the OVL report/document lifecycle vocabulary (draft →
 * ready → submitted → synced → pushed, or remarked / invalidated / push
 * failed), distinct from StatusChip's vessel-motion states (underway/
 * moored/anchored/alert). Filled pills for settled states, outlined for
 * provisional ones (ready, invalidated) — Material 3's own filled/outlined
 * distinction for "acted on" vs "in review."
 */
export function ReportStatusBadge({ status, resubmitted = false }) {
  const map = {
    draft: { label: "Draft", variant: "filled", bg: "var(--color-surface-container-highest)", fg: "var(--color-on-surface-variant)" },
    ready: { label: "Ready", variant: "outlined", fg: "var(--color-secondary)", border: "var(--color-secondary)" },
    submitted: { label: "Submitted", variant: "filled", bg: "var(--color-primary-container)", fg: "var(--color-on-primary-container)" },
    synced: { label: "Synced", variant: "filled", bg: "var(--color-primary)", fg: "var(--color-on-primary)" },
    pushed: { label: "Pushed", variant: "filled", bg: "var(--color-status-underway-container)", fg: "var(--color-status-underway)" },
    remarked: { label: "Remarked", variant: "filled", bg: "var(--color-status-caution-container)", fg: "var(--color-status-caution)" },
    invalidated: { label: "Invalidated", variant: "outlined", fg: "var(--color-error)", border: "var(--color-error)" },
    failed: { label: "Push failed", variant: "filled", bg: "var(--color-error)", fg: "var(--color-on-error)" },
  }[status] || { label: status, variant: "filled", bg: "var(--color-surface-container-highest)", fg: "var(--color-on-surface-variant)" };

  return (
    <span style={{ display: "inline-flex", alignItems: "center", gap: 6 }}>
      <span
        className="md-label-medium"
        style={{
          display: "inline-flex",
          alignItems: "center",
          height: 22,
          padding: "0 10px",
          borderRadius: "var(--shape-full)",
          background: map.variant === "outlined" ? "transparent" : map.bg,
          color: map.fg,
          border: map.variant === "outlined" ? `1px solid ${map.border}` : "1px solid transparent",
          fontWeight: 700,
          whiteSpace: "nowrap",
        }}
      >
        {map.label}
      </span>
      {resubmitted ? (
        <span
          title="Resubmitted"
          style={{ width: 8, height: 8, borderRadius: "50%", background: "var(--color-primary)", flexShrink: 0 }}
        />
      ) : null}
    </span>
  );
}
