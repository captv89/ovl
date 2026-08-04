// SPDX-License-Identifier: AGPL-3.0-only
// FORKED COMPONENT: web/vessel and web/office each keep their own copy of this component and they are intentionally NOT identical (vessel wireframe rework, 2026-07-13). A change here likely needs mirroring to the other app's copy — do not assume they match. See docs/codebase-audit-2026-07-22.md §6.

import React from "react";
import { FieldShell, fieldFrameStyle } from "./FieldShell.jsx";

/**
 * Textarea — label-above multiline input on FieldShell, for free-form/
 * narrative content (remarks, notes, descriptions). Uses FieldShell's
 * taller `minHeight`/`alignItems="flex-start"` variant, matching the
 * wireframe's own `.inp.area` (a taller, top-aligned version of the same
 * frame every other primitive uses) rather than a bespoke layout.
 */
export function Textarea({
  label,
  value,
  onChange,
  placeholder = null,
  supportingText = null,
  error = false,
  warning = false,
  disabled = false,
  required = false,
  infoTip = null,
  rows = 4,
  maxLength = null,
  policyOutline = null,
  style,
}) {
  const [focused, setFocused] = React.useState(false);
  const frame = fieldFrameStyle({ error, focused, warning, policyOutline });
  const length = typeof value === "string" ? value.length : 0;

  const counter = maxLength ? `${length}/${maxLength}` : null;
  // Finding text and the length counter can both be present at once —
  // shown side by side rather than one silently winning over the other.
  const footer =
    supportingText && counter ? (
      <span style={{ display: "flex", justifyContent: "space-between" }}>
        <span>{supportingText}</span>
        <span style={{ color: "var(--color-on-surface-variant)" }}>{counter}</span>
      </span>
    ) : (
      (supportingText ?? counter)
    );

  return (
    <FieldShell
      label={label}
      required={required}
      infoTip={infoTip}
      frame={frame}
      disabled={disabled}
      supportingText={footer}
      error={error}
      warning={warning}
      minHeight={rows * 22 + 20}
      alignItems="flex-start"
      style={style}
    >
      <textarea
        value={value}
        placeholder={placeholder ?? undefined}
        disabled={disabled}
        maxLength={maxLength ?? undefined}
        rows={rows}
        onChange={(e) => onChange && onChange(e.target.value)}
        onFocus={() => setFocused(true)}
        onBlur={() => setFocused(false)}
        style={{
          width: "100%",
          display: "block",
          border: "none",
          outline: "none",
          resize: "vertical",
          background: "transparent",
          fontFamily: "var(--font-body)",
          fontSize: 14,
          lineHeight: 1.5,
          color: "var(--color-on-surface)",
        }}
      />
    </FieldShell>
  );
}
