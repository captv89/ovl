// SPDX-License-Identifier: AGPL-3.0-only
// FORKED COMPONENT: web/vessel and web/office each keep their own copy of this component and they are intentionally NOT identical (vessel wireframe rework, 2026-07-13). A change here likely needs mirroring to the other app's copy — do not assume they match. See docs/codebase-audit-2026-07-22.md §6.

import React from "react";
import { Tooltip } from "../feedback/Tooltip.jsx";

/**
 * TextField — Material 3 filled or outlined text input with floating label.
 * Supports `type="password"` with a visibility toggle (Material 3's
 * standard password-field pattern) in addition to the default text input.
 * `suffix` renders a plain-text unit (e.g. "kn", "MT") after the input,
 * distinct from `trailingIcon` (a Material Symbols ligature). When
 * `trailingIcon` is paired with `trailingIconTooltip`, the icon shows that
 * text on hover/focus instead of needing a click — used for things like a
 * "calculated" indicator revealing its formula, or a compact info icon
 * standing in for a long help paragraph that would otherwise render as an
 * ugly wall of text under the field.
 *
 * `policyOutline` ("mandatory" | "ghgRelevant" | "both" | null) draws the
 * field-policy marker (see FieldRow's legend) as the field's OWN border —
 * solid for mandatory/both, dashed for GHG-relevant — instead of a second
 * frame stacked outside it. An earlier version drew that marker as an
 * absolutely-positioned overlay a couple of pixels outside this border; the
 * two edges never quite lined up on screen, reading as a broken/misaligned
 * box rather than one field. Error and focus still win over policy color —
 * those need to stay unambiguous while actively editing or fixing a field.
 */
export function TextField({ label, value, onChange, variant = "outlined", type = "text", supportingText = null, error = false, warning = false, leadingIcon = null, trailingIcon = null, trailingIconTooltip = null, trailingIconColor = null, trailingIconLabel = null, onTrailingIconClick = null, suffix = null, disabled = false, labelTooltip = null, policyOutline = null, style }) {
  const [focused, setFocused] = React.useState(false);
  const [revealed, setRevealed] = React.useState(false);
  const [labelHovered, setLabelHovered] = React.useState(false);
  const filled = variant === "filled";
  // date/time/datetime-local inputs always render their own placeholder
  // skeleton (dd/mm/yyyy, --:--) in the inline text position even when
  // "empty", unlike a plain text input — so the label has to live in its
  // floated slot permanently for these types, or the two collide visually.
  const alwaysFloatLabel = type === "date" || type === "time" || type === "datetime-local";
  const active = alwaysFloatLabel || focused || (value && value.length > 0);
  const policyActive = !error && !warning && !focused && !!policyOutline;
  // A live plausibility finding (see FieldRow's `pickFinding`) outranks the
  // field-policy marker but not an active error/focus state — error is
  // always the loudest signal, warning (amber) sits between focus and the
  // policy outline's quieter teal.
  const borderColor = error ? "var(--color-error)" : focused ? "var(--color-primary)" : warning ? "var(--color-status-caution)" : policyOutline ? "var(--color-secondary)" : "var(--color-outline)";
  const borderStyle = policyActive && policyOutline === "ghgRelevant" ? "dashed" : "solid";
  const borderWidth = policyActive || error || warning ? "1.5px" : "1px";
  // Explicit longhands (never mix the `border` shorthand with a `borderBottom`
  // override in the same style object — browsers can drop the whole border on
  // first paint when a shorthand and a conflicting longhand land together).
  const sideBorder = filled ? "none" : `${borderWidth} ${borderStyle} ${borderColor}`;
  const bottomBorder = `${borderWidth} ${borderStyle} ${borderColor}`;
  const isPassword = type === "password";
  // Any non-password type passes straight through to the native input
  // (number/date/time/datetime-local/...) instead of being coerced to
  // "text" — callers rely on this for the correct keyboard/picker.
  const inputType = isPassword ? (revealed ? "text" : "password") : type;
  // A caller-supplied trailingIcon takes precedence; the reveal toggle
  // only appears when there isn't already a trailing icon in use.
  const effectiveTrailingIcon = trailingIcon || (isPassword ? (revealed ? "visibility_off" : "visibility") : null);
  const trailingIconInteractive = !!onTrailingIconClick || (!trailingIcon && isPassword);
  const handleTrailingIconClick = onTrailingIconClick ? onTrailingIconClick : trailingIconInteractive ? () => setRevealed((r) => !r) : undefined;

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 4, width: 240, ...style }}>
      <div
        style={{
          position: "relative",
          display: "flex",
          alignItems: "center",
          height: 56,
          borderRadius: filled ? "var(--shape-extra-small) var(--shape-extra-small) 0 0" : "var(--shape-small)",
          background: filled ? "var(--color-surface-container-highest)" : "transparent",
          borderTop: sideBorder,
          borderRight: sideBorder,
          borderLeft: sideBorder,
          borderBottom: bottomBorder,
          padding: "0 12px",
          opacity: disabled ? "var(--state-disabled-opacity)" : 1,
        }}
      >
        {leadingIcon ? <span className="material-symbols-rounded" style={{ fontSize: 20, marginRight: 8, color: "var(--color-on-surface-variant)" }}>{leadingIcon}</span> : null}
        {/* No overflow:hidden here — the label itself already clips its own
            text horizontally (see below); a parent overflow:hidden combined
            with the label's negative top offset when floated clips its
            ascenders/cap-height vertically instead (visible as a sliced-off
            top edge on every floated label). */}
        <div style={{ position: "relative", flex: 1, minWidth: 0 }}>
          <label
            onMouseEnter={labelTooltip && active ? () => setLabelHovered(true) : undefined}
            onMouseLeave={labelTooltip && active ? () => setLabelHovered(false) : undefined}
            onFocus={labelTooltip && active ? () => setLabelHovered(true) : undefined}
            onBlur={labelTooltip && active ? () => setLabelHovered(false) : undefined}
            tabIndex={labelTooltip && active ? 0 : undefined}
            style={{
              position: "absolute",
              left: 0,
              top: active ? -8 : "50%",
              transform: active ? "translateY(0) scale(0.75)" : "translateY(-50%) scale(1)",
              transformOrigin: "left",
              background: active && !filled ? "var(--color-surface)" : "transparent",
              padding: active && !filled ? "0 4px" : 0,
              color: error ? "var(--color-error)" : focused ? "var(--color-primary)" : warning ? "var(--color-status-caution)" : "var(--color-on-surface-variant)",
              fontFamily: "var(--font-body)",
              fontSize: 16,
              transition: "all var(--motion-duration-short) var(--motion-easing-standard)",
              // The label is position:absolute, so it already paints above
              // the static-flow input beneath it — pointer-events must stay
              // "none" until the label is actually floated (active) out of
              // the input's own tappable area, or a tooltip-carrying label
              // would swallow clicks/typing meant for an empty, unfocused
              // field (real bug caught live: typing into IMO silently did
              // nothing because the still full-size label was intercepting
              // the click before it ever reached the input).
              pointerEvents: labelTooltip && active ? "auto" : "none",
              cursor: labelTooltip && active ? "help" : "default",
              whiteSpace: "nowrap",
              // A long field label (this schema has some) must never run
              // into the suffix/trailing-icon slot — clip with ellipsis at
              // the input's own width instead of visually overlapping it.
              overflow: "hidden",
              textOverflow: "ellipsis",
              maxWidth: "100%",
            }}
          >
            {label}
            {labelTooltip && labelHovered ? (
              <span
                role="tooltip"
                style={{
                  position: "absolute",
                  bottom: "125%",
                  left: 0,
                  background: "var(--color-inverse-surface)",
                  color: "var(--color-inverse-on-surface)",
                  fontSize: 12,
                  lineHeight: 1.4,
                  padding: "6px 10px",
                  borderRadius: "var(--shape-extra-small)",
                  boxShadow: "var(--elevation-2)",
                  whiteSpace: "normal",
                  width: 220,
                  textAlign: "left",
                  zIndex: 20,
                }}
              >
                {labelTooltip}
              </span>
            ) : null}
          </label>
          <input
            type={inputType}
            value={value}
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
              fontSize: 16,
              color: "var(--color-on-surface)",
              paddingTop: active ? 8 : 0,
            }}
          />
        </div>
        {suffix ? (
          <span className="md-body-medium" style={{ marginLeft: 8, color: "var(--color-on-surface-variant)", whiteSpace: "nowrap" }}>
            {suffix}
          </span>
        ) : null}
        {effectiveTrailingIcon ? (() => {
          // An interactive trailing icon (the calculate/restore button, the
          // password reveal toggle) is a real <button>, not a decorative
          // <span> with an onClick bolted on — it needs to be reachable and
          // operable via keyboard (Enter/Space), not just a mouse target.
          const iconEl = trailingIconInteractive ? (
            <button
              type="button"
              aria-label={trailingIconLabel || trailingIconTooltip || undefined}
              onClick={handleTrailingIconClick}
              className="material-symbols-rounded"
              style={{
                fontSize: 20,
                marginLeft: 8,
                color: trailingIconColor || "var(--color-on-surface-variant)",
                cursor: "pointer",
                background: "none",
                border: "none",
                padding: 4,
                margin: "0 0 0 4px",
                borderRadius: "var(--shape-full)",
                display: "inline-flex",
              }}
            >
              {effectiveTrailingIcon}
            </button>
          ) : (
            <span
              className="material-symbols-rounded"
              tabIndex={trailingIconTooltip ? 0 : undefined}
              style={{ fontSize: 20, marginLeft: 8, color: trailingIconColor || "var(--color-on-surface-variant)", cursor: trailingIconTooltip ? "help" : "default" }}
            >
              {effectiveTrailingIcon}
            </span>
          );
          return trailingIconTooltip ? (
            <Tooltip label={trailingIconTooltip} maxWidth={240}>
              {iconEl}
            </Tooltip>
          ) : (
            iconEl
          );
        })() : null}
      </div>
      {supportingText ? (
        <span className="md-body-small" style={{ color: error ? "var(--color-error)" : warning ? "var(--color-status-caution)" : "var(--color-on-surface-variant)", paddingLeft: 12 }}>{supportingText}</span>
      ) : null}
    </div>
  );
}
