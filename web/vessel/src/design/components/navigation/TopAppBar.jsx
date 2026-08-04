// SPDX-License-Identifier: AGPL-3.0-only
// FORKED COMPONENT: web/vessel and web/office each keep their own copy of this component and they are intentionally NOT identical (vessel wireframe rework, 2026-07-13). A change here likely needs mirroring to the other app's copy — do not assume they match. See docs/codebase-audit-2026-07-22.md §6.

import React from "react";

/**
 * TopAppBar — M3 top bar with leading icon, title, and trailing actions.
 *
 * `trailing` renders after the icon `actions`, for content that isn't a
 * simple icon button — e.g. an account/user menu. Icon `actions` stay
 * icon-only; `trailing` is an escape hatch for the one thing that
 * doesn't fit that shape, not a general children slot.
 */
export function TopAppBar({ title, leadingIcon = "menu", onLeadingClick, actions = [], trailing = null, style }) {
  return (
    <div
      style={{
        display: "flex",
        alignItems: "center",
        gap: 8,
        height: 64,
        padding: "0 8px",
        background: "var(--color-surface)",
        color: "var(--color-on-surface)",
        borderBottom: "1px solid var(--color-outline-variant)",
        ...style,
      }}
    >
      {leadingIcon ? (
        <button onClick={onLeadingClick} style={{ width: 40, height: 40, border: "none", background: "transparent", borderRadius: "var(--shape-full)", cursor: "pointer", display: "flex", alignItems: "center", justifyContent: "center" }}>
          <span className="material-symbols-rounded">{leadingIcon}</span>
        </button>
      ) : null}
      <span className="md-title-large" style={{ flex: 1, fontFamily: "var(--font-brand)" }}>{title}</span>
      {actions.map((a, i) => (
        <button key={i} onClick={a.onClick} aria-label={a.icon} style={{ width: 40, height: 40, border: "none", background: "transparent", borderRadius: "var(--shape-full)", cursor: "pointer", display: "flex", alignItems: "center", justifyContent: "center", color: "var(--color-on-surface-variant)" }}>
          <span className="material-symbols-rounded">{a.icon}</span>
        </button>
      ))}
      {trailing}
    </div>
  );
}
