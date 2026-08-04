// SPDX-License-Identifier: AGPL-3.0-only

import React from "react";

const SIZE = { standard: 40, small: 32, large: 56 };

/**
 * IconButton — circular/square icon-only control (M3 standard/filled/tonal/outlined).
 */
export function IconButton({
  icon,
  variant = "standard",
  size = "standard",
  selected = false,
  disabled = false,
  onClick,
  "aria-label": ariaLabel,
  style,
}) {
  const dim = SIZE[size] || 40;
  const base = {
    standard: { background: "transparent", color: "var(--color-on-surface-variant)" },
    filled: { background: selected ? "var(--color-primary)" : "var(--color-surface-container-highest)", color: selected ? "var(--color-on-primary)" : "var(--color-primary)" },
    tonal: { background: "var(--color-secondary-container)", color: "var(--color-on-secondary-container)" },
    outlined: { background: selected ? "var(--color-inverse-surface)" : "transparent", color: selected ? "var(--color-inverse-on-surface)" : "var(--color-on-surface-variant)", border: "1px solid var(--color-outline-variant)" },
  }[variant];

  return (
    <button
      aria-label={ariaLabel || icon}
      onClick={disabled ? undefined : onClick}
      disabled={disabled}
      style={{
        width: dim,
        height: dim,
        borderRadius: "var(--shape-full)",
        border: "1px solid transparent",
        display: "inline-flex",
        alignItems: "center",
        justifyContent: "center",
        cursor: disabled ? "not-allowed" : "pointer",
        opacity: disabled ? "var(--state-disabled-opacity)" : 1,
        transition: "background-color var(--motion-duration-short) var(--motion-easing-standard)",
        ...base,
        ...style,
      }}
    >
      <span className="material-symbols-rounded" style={{ fontSize: dim <= 32 ? 18 : 22 }}>{icon}</span>
    </button>
  );
}
