// SPDX-License-Identifier: AGPL-3.0-only

import React from "react";

const VARIANT = {
  elevated: { background: "var(--color-surface-container-low)", boxShadow: "var(--elevation-1)", border: "1px solid transparent" },
  filled: { background: "var(--color-surface-container-highest)", boxShadow: "none", border: "1px solid transparent" },
  outlined: { background: "var(--color-surface)", boxShadow: "none", border: "1px solid var(--color-outline-variant)" },
};

/**
 * Card — Material 3 container (elevated / filled / outlined).
 */
export function Card({ children, variant = "elevated", padding = "var(--space-4)", style, onClick }) {
  return (
    <div
      onClick={onClick}
      style={{
        borderRadius: "var(--shape-medium)",
        padding,
        color: "var(--color-on-surface)",
        cursor: onClick ? "pointer" : "default",
        ...VARIANT[variant],
        ...style,
      }}
    >
      {children}
    </div>
  );
}
