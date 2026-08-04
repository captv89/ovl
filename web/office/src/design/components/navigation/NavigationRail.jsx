// SPDX-License-Identifier: AGPL-3.0-only

import React from "react";

/**
 * NavigationRail — vertical icon+label rail for dashboard/console left nav.
 */
export function NavigationRail({ items, selected, onSelect, style }) {
  return (
    <div
      style={{
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        gap: 4,
        width: 88,
        padding: "16px 8px",
        background: "var(--color-surface-container-low)",
        borderRight: "1px solid var(--color-outline-variant)",
        ...style,
      }}
    >
      {items.map((it) => {
        const active = it.key === selected;
        return (
          <button
            key={it.key}
            onClick={() => onSelect && onSelect(it.key)}
            style={{
              display: "flex",
              flexDirection: "column",
              alignItems: "center",
              gap: 4,
              width: "100%",
              padding: "8px 0",
              border: "none",
              background: "transparent",
              cursor: "pointer",
              color: active ? "var(--color-on-secondary-container)" : "var(--color-on-surface-variant)",
            }}
          >
            <span
              className="material-symbols-rounded"
              style={{
                width: 56,
                height: 32,
                borderRadius: "var(--shape-full)",
                background: active ? "var(--color-secondary-container)" : "transparent",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
              }}
            >
              {it.icon}
            </span>
            <span className="md-label-medium">{it.label}</span>
          </button>
        );
      })}
    </div>
  );
}
