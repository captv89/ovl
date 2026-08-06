// SPDX-License-Identifier: AGPL-3.0-only

import React from "react";
import { InfoTip } from "./InfoTip.jsx";

/**
 * fieldFrameStyle — the single implementation of every field primitive's
 * border-color/style/width priority: error > focus > warning > policy
 * outline > default. Previously duplicated with drift across TextField,
 * Textarea, DateTimeField, PositionField, Select (which had none at all),
 * and FieldRow's own inline boolean-row copy. `policyOutline` is
 * "mandatory" | "ghgRelevant" | "both" | null; only pure "ghgRelevant"
 * draws a dashed border, "both" stays solid (matches the field-policy
 * legend). A bare focus with nothing else to flag stays hairline-width —
 * only a state actually worth calling out (policy marker active, error,
 * warning) gets the thicker 1.5px border.
 */
export function fieldFrameStyle({ error = false, focused = false, warning = false, policyOutline = null } = {}) {
  const policyActive = !error && !warning && !focused && !!policyOutline;
  const borderColor = error
    ? "var(--color-error)"
    : focused
      ? "var(--color-primary)"
      : warning
        ? "var(--color-status-caution)"
        : policyOutline
          ? "var(--color-secondary)"
          : "var(--color-outline)";
  const borderStyle = policyActive && policyOutline === "ghgRelevant" ? "dashed" : "solid";
  const borderWidth = policyActive || error || warning ? "1.5px" : "1px";
  return { borderColor, borderStyle, borderWidth };
}

/**
 * FieldShell — shared label row + input frame + supporting-text line for
 * every field primitive. Anatomy matches the OVL Vessel Wireframes design
 * spec (label-above, 38px input frame, always-visible ⓘ info icon) rather
 * than Material 3's floating-label pattern, which was dropped.
 *
 * `frame` is the object `fieldFrameStyle` returns — FieldShell doesn't
 * compute it itself so a non-standard trigger (e.g. Select's clickable
 * row) can still share this exact visual shell with its own open/closed
 * state feeding the same shared function. `infoTip` renders an
 * always-visible ⓘ after the label (reachable regardless of value/focus,
 * unlike the floating-label tooltip it replaces); `chip` is an arbitrary
 * node after that (used today only for the computed-field "calculated"
 * chip). `children` is the actual input control, rendered inside the
 * bordered frame alongside the optional `suffix` unit slot.
 *
 * Border sides are explicit longhands, not the `border` shorthand — see
 * TextField's original comment (now here) on why: a shorthand and a
 * conflicting longhand override in the same style object can make a
 * browser drop the whole border on first paint.
 */
export function FieldShell({
  label,
  required = false,
  infoTip = null,
  chip = null,
  frame,
  filledTint = false,
  disabled = false,
  leadingIcon = null,
  suffix = null,
  supportingText = null,
  error = false,
  warning = false,
  // Multiline callers (Textarea) pass minHeight + alignItems="flex-start"
  // to get the wireframe's own taller `.inp.area` variant instead of the
  // single-line 38px/centered frame every other primitive uses.
  minHeight = 38,
  alignItems = "center",
  children,
  style,
}) {
  const supportingColor = error ? "var(--color-error)" : warning ? "var(--color-status-caution)" : "var(--color-on-surface-variant)";
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 5, width: 240, opacity: disabled ? "var(--state-disabled-opacity)" : 1, ...style }}>
      <span className="md-label-small" style={{ display: "flex", alignItems: "center", gap: 4, color: "var(--color-on-surface-variant)", minWidth: 0 }}>
        <span style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{label}</span>
        {required ? <span aria-hidden="true">*</span> : null}
        {infoTip ? <InfoTip label={infoTip} /> : null}
        {chip}
      </span>
      <div
        style={{
          display: "flex",
          alignItems,
          gap: 8,
          minHeight,
          borderRadius: "var(--shape-small)",
          background: filledTint ? "var(--color-surface-container-highest)" : "var(--color-surface)",
          padding: alignItems === "flex-start" ? "10px 12px" : "0 12px",
          borderTop: `${frame.borderWidth} ${frame.borderStyle} ${frame.borderColor}`,
          borderRight: `${frame.borderWidth} ${frame.borderStyle} ${frame.borderColor}`,
          borderBottom: `${frame.borderWidth} ${frame.borderStyle} ${frame.borderColor}`,
          borderLeft: `${frame.borderWidth} ${frame.borderStyle} ${frame.borderColor}`,
        }}
      >
        {leadingIcon ? (
          <span className="material-symbols-rounded" aria-hidden="true" style={{ fontSize: 18, flexShrink: 0, color: "var(--color-on-surface-variant)" }}>
            {leadingIcon}
          </span>
        ) : null}
        {children}
        {/* Layout-only wrapper: a plain unit ("kn", "MT") and a real
            clickable restore button (the computed-field ↺) both land here,
            so this doesn't impose text styling a button would have to
            fight — each caller styles its own suffix content. */}
        {suffix ? <span style={{ marginLeft: "auto", display: "flex", alignItems: "center", flexShrink: 0 }}>{suffix}</span> : null}
      </div>
      {supportingText ? (
        <span className="md-body-small" style={{ color: supportingColor }}>
          {supportingText}
        </span>
      ) : null}
    </div>
  );
}
