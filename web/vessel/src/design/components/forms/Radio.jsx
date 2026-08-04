// SPDX-License-Identifier: AGPL-3.0-only

import React from "react";

export function Radio({ selected, onChange, label, disabled = false }) {
  return (
    <label style={{ display: "inline-flex", alignItems: "center", gap: 8, cursor: disabled ? "not-allowed" : "pointer", opacity: disabled ? "var(--state-disabled-opacity)" : 1 }}>
      <span
        onClick={() => !disabled && onChange && onChange()}
        style={{
          width: 18,
          height: 18,
          borderRadius: "50%",
          border: `2px solid ${selected ? "var(--color-primary)" : "var(--color-outline)"}`,
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
        }}
      >
        {selected ? <span style={{ width: 10, height: 10, borderRadius: "50%", background: "var(--color-primary)" }} /> : null}
      </span>
      {label ? <span className="md-body-medium">{label}</span> : null}
    </label>
  );
}
