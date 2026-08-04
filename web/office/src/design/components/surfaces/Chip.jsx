// SPDX-License-Identifier: AGPL-3.0-only

import React from "react";

/**
 * Chip — Assist / Filter / Input / Suggestion chip.
 */
export function Chip({ label, icon = null, type = "assist", selected = false, onDelete = null, onClick, style }) {
  const isFilter = type === "filter";
  const selectedStyle = selected
    ? { background: "var(--color-secondary-container)", color: "var(--color-on-secondary-container)", border: "1px solid transparent" }
    : { background: "transparent", color: "var(--color-on-surface-variant)", border: "1px solid var(--color-outline-variant)" };

  return (
    <button
      onClick={onClick}
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: 6,
        height: 32,
        padding: "0 12px",
        borderRadius: "var(--shape-small)",
        fontFamily: "var(--font-body)",
        fontSize: "var(--type-label-large-size)",
        fontWeight: 500,
        cursor: "pointer",
        ...selectedStyle,
        ...style,
      }}
    >
      {isFilter && selected ? <span className="material-symbols-rounded" style={{ fontSize: 16 }}>check</span> : null}
      {icon && !(isFilter && selected) ? <span className="material-symbols-rounded" style={{ fontSize: 16 }}>{icon}</span> : null}
      <span>{label}</span>
      {onDelete ? (
        <span
          className="material-symbols-rounded"
          style={{ fontSize: 16, marginLeft: 2 }}
          onClick={(e) => { e.stopPropagation(); onDelete(); }}
        >
          close
        </span>
      ) : null}
    </button>
  );
}
