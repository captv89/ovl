// SPDX-License-Identifier: AGPL-3.0-only

import React from "react";

export function Switch({ checked, onChange, disabled = false }) {
  return (
    <span
      onClick={() => !disabled && onChange && onChange(!checked)}
      style={{
        width: 52,
        height: 32,
        borderRadius: "var(--shape-full)",
        background: checked ? "var(--color-primary)" : "var(--color-surface-container-highest)",
        border: checked ? "none" : "2px solid var(--color-outline)",
        display: "inline-flex",
        alignItems: "center",
        padding: 2,
        cursor: disabled ? "not-allowed" : "pointer",
        opacity: disabled ? "var(--state-disabled-opacity)" : 1,
        transition: "background var(--motion-duration-short) var(--motion-easing-standard)",
      }}
    >
      <span
        style={{
          width: checked ? 24 : 14,
          height: checked ? 24 : 14,
          borderRadius: "50%",
          background: checked ? "var(--color-on-primary)" : "var(--color-outline)",
          transform: checked ? "translateX(20px)" : "translateX(0)",
          transition: "transform var(--motion-duration-short) var(--motion-easing-standard), width var(--motion-duration-short), height var(--motion-duration-short)",
        }}
      />
    </span>
  );
}
