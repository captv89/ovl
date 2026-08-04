// SPDX-License-Identifier: AGPL-3.0-only

import React from "react";

export function AlertBanner({ level = "caution", title, message, onDismiss }) {
  const map = {
    caution: { bg: "var(--color-status-caution-container)", fg: "var(--color-status-caution)", icon: "info" },
    warning: { bg: "var(--color-status-warning-container)", fg: "var(--color-status-warning)", icon: "warning" },
    ok: { bg: "var(--color-status-underway-container)", fg: "var(--color-status-underway)", icon: "check_circle" },
  }[level];
  return (
    <div style={{ display: "flex", alignItems: "flex-start", gap: 12, padding: "12px 16px", borderRadius: "var(--shape-medium)", background: map.bg, borderLeft: `4px solid ${map.fg}` }}>
      <span className="material-symbols-rounded" style={{ color: map.fg, fontSize: 22 }}>{map.icon}</span>
      <div style={{ flex: 1 }}>
        <div className="md-title-small" style={{ color: "var(--color-on-surface)" }}>{title}</div>
        <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)" }}>{message}</div>
      </div>
      {onDismiss ? (
        <span className="material-symbols-rounded" style={{ cursor: "pointer", color: "var(--color-on-surface-variant)", fontSize: 18 }} onClick={onDismiss}>close</span>
      ) : null}
    </div>
  );
}
