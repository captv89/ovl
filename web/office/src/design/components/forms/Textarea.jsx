// SPDX-License-Identifier: AGPL-3.0-only
// FORKED COMPONENT: web/vessel and web/office each keep their own copy of this component and they are intentionally NOT identical (vessel wireframe rework, 2026-07-13). A change here likely needs mirroring to the other app's copy — do not assume they match. See docs/codebase-audit-2026-07-22.md §6.

import React from "react";
import { Tooltip } from "../feedback/Tooltip.jsx";

/**
 * Textarea — Material 3 outlined multiline input for free-form/narrative
 * content (remarks, notes, descriptions). Unlike TextField's single-line
 * floating label (which reclaims horizontal space when the field is
 * empty), a multi-row box has no such need — the label sits in a fixed
 * slot above the field at all times instead of animating into the border.
 */
export function Textarea({ label, value, onChange, variant = "outlined", supportingText = null, error = false, warning = false, labelTooltip = null, rows = 4, maxLength = null, disabled = false, policyOutline = null, style }) {
  const [focused, setFocused] = React.useState(false);
  const filled = variant === "filled";
  const policyActive = !error && !warning && !focused && !!policyOutline;
  const borderColor = error ? "var(--color-error)" : focused ? "var(--color-primary)" : warning ? "var(--color-status-caution)" : policyOutline ? "var(--color-secondary)" : "var(--color-outline)";
  const borderStyle = policyActive && policyOutline === "ghgRelevant" ? "dashed" : "solid";
  const borderWidth = policyActive || error || warning ? "1.5px" : "1px";
  // Explicit longhands, matching TextField — never mix the `border` shorthand
  // with a `borderBottom` override in the same style object.
  const sideBorder = filled ? "none" : `${borderWidth} ${borderStyle} ${borderColor}`;
  const bottomBorder = `${borderWidth} ${borderStyle} ${borderColor}`;
  const length = typeof value === "string" ? value.length : 0;

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 4, width: 240, ...style }}>
      {labelTooltip ? (
        <Tooltip label={labelTooltip} maxWidth={240}>
          <span
            className="md-body-small"
            style={{ color: error ? "var(--color-error)" : focused ? "var(--color-primary)" : warning ? "var(--color-status-caution)" : "var(--color-on-surface-variant)", paddingLeft: 12, cursor: "help" }}
          >
            {label}
          </span>
        </Tooltip>
      ) : (
        <span
          className="md-body-small"
          style={{ color: error ? "var(--color-error)" : focused ? "var(--color-primary)" : warning ? "var(--color-status-caution)" : "var(--color-on-surface-variant)", paddingLeft: 12 }}
        >
          {label}
        </span>
      )}
      <div
        style={{
          borderRadius: filled ? "var(--shape-extra-small) var(--shape-extra-small) 0 0" : "var(--shape-small)",
          background: filled ? "var(--color-surface-container-highest)" : "transparent",
          borderTop: sideBorder,
          borderRight: sideBorder,
          borderLeft: sideBorder,
          borderBottom: bottomBorder,
          padding: "10px 12px",
          opacity: disabled ? "var(--state-disabled-opacity)" : 1,
        }}
      >
        <textarea
          value={value}
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
            fontSize: 16,
            lineHeight: 1.5,
            color: "var(--color-on-surface)",
          }}
        />
      </div>
      <div style={{ display: "flex", justifyContent: "space-between", paddingLeft: 12, paddingRight: 12 }}>
        <span className="md-body-small" style={{ color: error ? "var(--color-error)" : warning ? "var(--color-status-caution)" : "var(--color-on-surface-variant)" }}>
          {supportingText ?? ""}
        </span>
        {maxLength ? (
          <span className="md-body-small" style={{ color: "var(--color-on-surface-variant)" }}>{length}/{maxLength}</span>
        ) : null}
      </div>
    </div>
  );
}
