// SPDX-License-Identifier: AGPL-3.0-only
// FORKED COMPONENT: web/vessel and web/office each keep their own copy of this component and they are intentionally NOT identical (vessel wireframe rework, 2026-07-13). A change here likely needs mirroring to the other app's copy — do not assume they match. See docs/codebase-audit-2026-07-22.md §6.

import React from "react";
import { FieldShell, fieldFrameStyle } from "./FieldShell.jsx";

/**
 * TextField — label-above text input on FieldShell (OVL Vessel
 * Wireframes anatomy, replacing the earlier Material-3 floating-label
 * version).
 * Supports `type="password"` with a visibility toggle rendered into
 * FieldShell's `suffix` slot, in addition to the default text input.
 * `suffix` renders arbitrary content after the input (a plain unit span,
 * a computed-field restore button, a lock icon) — TextField itself only
 * owns the password-reveal case, everything else is the caller's (see
 * FieldRow.tsx) composition.
 *
 * `policyOutline` ("mandatory" | "ghgRelevant" | "both" | null) — see
 * `fieldFrameStyle`'s own doc comment for the border priority this
 * feeds into.
 */
export function TextField({
  label,
  value,
  onChange,
  type = "text",
  placeholder = null,
  supportingText = null,
  error = false,
  warning = false,
  disabled = false,
  required = false,
  infoTip = null,
  chip = null,
  leadingIcon = null,
  suffix = null,
  filledTint = false,
  policyOutline = null,
  style,
}) {
  const [focused, setFocused] = React.useState(false);
  const [revealed, setRevealed] = React.useState(false);
  const isPassword = type === "password";
  // Any non-password type passes straight through to the native input
  // (number/date/time/datetime-local/...) instead of being coerced to
  // "text" — callers rely on this for the correct keyboard/picker.
  const inputType = isPassword ? (revealed ? "text" : "password") : type;

  const frame = fieldFrameStyle({ error, focused, warning, policyOutline });

  // A password field owns its own suffix slot (the reveal toggle) rather
  // than accepting a caller-supplied one — the two needs don't co-occur
  // in practice, and this keeps password handling fully self-contained.
  const effectiveSuffix = isPassword ? (
    <button
      type="button"
      aria-label={revealed ? "Hide password" : "Show password"}
      onClick={() => setRevealed((r) => !r)}
      className="material-symbols-rounded"
      style={{
        fontSize: 18,
        color: "var(--color-on-surface-variant)",
        cursor: "pointer",
        background: "none",
        border: "none",
        padding: 4,
        borderRadius: "var(--shape-full)",
        display: "inline-flex",
      }}
    >
      {revealed ? "visibility_off" : "visibility"}
    </button>
  ) : (
    suffix
  );

  return (
    <FieldShell
      label={label}
      required={required}
      infoTip={infoTip}
      chip={chip}
      frame={frame}
      filledTint={filledTint}
      disabled={disabled}
      leadingIcon={leadingIcon}
      suffix={effectiveSuffix}
      supportingText={supportingText}
      error={error}
      warning={warning}
      style={style}
    >
      <input
        type={inputType}
        value={value}
        placeholder={placeholder ?? undefined}
        disabled={disabled}
        title={label}
        onChange={(e) => onChange && onChange(e.target.value)}
        onFocus={() => setFocused(true)}
        onBlur={() => setFocused(false)}
        style={{
          width: "100%",
          border: "none",
          outline: "none",
          background: "transparent",
          fontFamily: "var(--font-body)",
          fontSize: 14,
          color: "var(--color-on-surface)",
        }}
      />
    </FieldShell>
  );
}
