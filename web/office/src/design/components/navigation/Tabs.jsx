// SPDX-License-Identifier: AGPL-3.0-only

import React from "react";

/**
 * Tabs — M3 primary tabs with animated underline indicator.
 */
export function Tabs({ items, selected, onSelect, style }) {
  return (
    <div style={{ display: "flex", borderBottom: "1px solid var(--color-outline-variant)", ...style }}>
      {items.map((it) => {
        const active = it === selected;
        return (
          <button
            key={it}
            onClick={() => onSelect && onSelect(it)}
            style={{
              padding: "12px 16px",
              border: "none",
              background: "transparent",
              cursor: "pointer",
              position: "relative",
              color: active ? "var(--color-primary)" : "var(--color-on-surface-variant)",
              fontFamily: "var(--font-body)",
              fontSize: "var(--type-title-small-size)",
              fontWeight: 600,
            }}
          >
            {it}
            {active ? (
              <span style={{ position: "absolute", left: 0, right: 0, bottom: -1, height: 3, borderRadius: "3px 3px 0 0", background: "var(--color-primary)" }} />
            ) : null}
          </button>
        );
      })}
    </div>
  );
}
