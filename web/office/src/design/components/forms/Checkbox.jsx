// SPDX-License-Identifier: AGPL-3.0-only

import React from "react";

export function Checkbox({ checked, onChange, label, disabled = false }) {
  return (
    <label style={{ display: "inline-flex", alignItems: "center", gap: 8, cursor: disabled ? "not-allowed" : "pointer", opacity: disabled ? "var(--state-disabled-opacity)" : 1 }}>
      <span
        onClick={() => !disabled && onChange && onChange(!checked)}
        style={{
          width: 18,
          height: 18,
          borderRadius: 3,
          border: checked ? "none" : "2px solid var(--color-outline)",
          background: checked ? "var(--color-primary)" : "transparent",
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
        }}
      >
        {checked ? <span className="material-symbols-rounded" style={{ fontSize: 14, color: "var(--color-on-primary)" }}>check</span> : null}
      </span>
      {label ? <span className="md-body-medium">{label}</span> : null}
    </label>
  );
}
