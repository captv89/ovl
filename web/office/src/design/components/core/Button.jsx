// SPDX-License-Identifier: AGPL-3.0-only

import React from "react";

const VARIANT_STYLES = {
  filled: {
    background: "var(--color-primary)",
    color: "var(--color-on-primary)",
    border: "1px solid transparent",
  },
  tonal: {
    background: "var(--color-secondary-container)",
    color: "var(--color-on-secondary-container)",
    border: "1px solid transparent",
  },
  outlined: {
    background: "transparent",
    color: "var(--color-primary)",
    border: "1px solid var(--color-outline)",
  },
  text: {
    background: "transparent",
    color: "var(--color-primary)",
    border: "1px solid transparent",
  },
  elevated: {
    background: "var(--color-surface-container-low)",
    color: "var(--color-primary)",
    border: "1px solid transparent",
    boxShadow: "var(--elevation-1)",
  },
};

const SIZE_STYLES = {
  small: { height: 32, padding: "0 16px", fontSize: "var(--type-label-large-size)", gap: 6 },
  medium: { height: 40, padding: "0 24px", fontSize: "var(--type-label-large-size)", gap: 8 },
  large: { height: 56, padding: "0 32px", fontSize: "18px", gap: 10 },
};

/**
 * Button — Material 3 common button, re-themed for Tideline.
 * Variants: filled (primary action), tonal (secondary emphasis),
 * outlined, text (low emphasis), elevated (on busy/colored surfaces).
 */
export function Button({
  children,
  variant = "filled",
  size = "medium",
  icon = null,
  disabled = false,
  onClick,
  style,
  ...rest
}) {
  const v = VARIANT_STYLES[variant] || VARIANT_STYLES.filled;
  const s = SIZE_STYLES[size] || SIZE_STYLES.medium;
  const [hover, setHover] = React.useState(false);

  return (
    <button
      onClick={disabled ? undefined : onClick}
      disabled={disabled}
      onMouseEnter={() => setHover(true)}
      onMouseLeave={() => setHover(false)}
      style={{
        display: "inline-flex",
        alignItems: "center",
        justifyContent: "center",
        gap: s.gap,
        height: s.height,
        padding: s.padding,
        borderRadius: "var(--shape-full)",
        fontFamily: "var(--font-body)",
        fontSize: s.fontSize,
        fontWeight: "var(--type-label-large-weight)",
        letterSpacing: "var(--type-label-large-tracking)",
        cursor: disabled ? "not-allowed" : "pointer",
        transition: "background-color var(--motion-duration-short) var(--motion-easing-standard), box-shadow var(--motion-duration-short) var(--motion-easing-standard)",
        opacity: disabled ? "var(--state-disabled-opacity)" : 1,
        ...v,
        ...(hover && !disabled
          ? { filter: variant === "filled" || variant === "tonal" ? "brightness(0.94)" : undefined, background: variant === "text" || variant === "outlined" ? "color-mix(in oklab, var(--color-primary) 8%, transparent)" : v.background }
          : {}),
        ...style,
      }}
      {...rest}
    >
      {icon ? <span className="material-symbols-rounded" style={{ fontSize: 18 }}>{icon}</span> : null}
      {children}
    </button>
  );
}
