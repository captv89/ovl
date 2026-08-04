// SPDX-License-Identifier: AGPL-3.0-only

import React from "react";
import { FieldShell, fieldFrameStyle } from "../forms/FieldShell.jsx";

const HEMISPHERES = { lat: ["N", "S"], lon: ["E", "W"] };
// Mirrors pkg/validation/plausibility.go's evaluatePositionRequired exactly
// (latitude 0-90°, longitude 0-180°, minutes 0-<60') — the same physical
// bounds, not a new business rule, so masking degree/minutes entry to these
// ranges here can never disagree with the engine's own plausibility
// findings for the same fields.
const MAX_DEGREE = { lat: 90, lon: 180 };
const MAX_MINUTES = 59.999;
// Longitude's own max (180) needs a third digit latitude never does (90) —
// a shared 2-character box/placeholder clipped a typed "180" (2026-07-13
// manual-test feedback). Both the placeholder and the input's own width
// scale with the axis's real digit count instead of assuming 2 for both.
const DEGREE_DIGITS = { lat: 2, lon: 3 };
const DEGREE_INPUT_WIDTH = { lat: 40, lon: 52 };

// Strips anything that isn't a digit (degree is always a whole number,
// architecture has no fractional-degree fields) and clamps to [0, max] —
// but only once the typed value is unambiguously out of range, so a
// legitimate in-progress value like "9" (on the way to "90") is never
// clobbered mid-keystroke.
function maskDegreeInput(raw, max) {
  const digits = raw.replace(/[^0-9]/g, "");
  if (digits === "") return "";
  const n = Number(digits);
  return n > max ? String(max) : String(Number.parseInt(digits, 10));
}

// Minutes allow one decimal point; clamps to [0, 59.999] the same way —
// only when the parsed value is actually out of range, preserving an
// in-progress "12." or trailing-zero entry like "12.30" untouched.
function maskMinutesInput(raw) {
  let cleaned = raw.replace(/[^0-9.]/g, "");
  const firstDot = cleaned.indexOf(".");
  if (firstDot !== -1) cleaned = cleaned.slice(0, firstDot + 1) + cleaned.slice(firstDot + 1).replace(/\./g, "");
  if (cleaned === "" || cleaned === ".") return cleaned;
  const n = Number(cleaned);
  if (Number.isNaN(n)) return cleaned;
  if (n > MAX_MINUTES) return String(MAX_MINUTES);
  if (n < 0) return "0";
  return cleaned;
}

/**
 * PositionField — a single bordered field combining a lat/long triple
 * (whole-number degrees, decimal minutes, hemisphere) into one compound
 * control with inline ° and ' symbols, instead of three separate text
 * boxes. Sits on FieldShell like every other primitive. Degree/minutes
 * typing is masked and range-clamped as the officer types (see
 * maskDegreeInput/maskMinutesInput) rather than relying on native
 * `<input type="number">` min/max, which only affects the spinner and a
 * validity flag — it never actually stops an out-of-range value (e.g.
 * "1232") from being typed and committed.
 */
export function PositionField({
  axis,
  label,
  degree,
  minutes,
  hemisphere,
  onChangeDegree,
  onChangeMinutes,
  onChangeHemisphere,
  error = false,
  warning = false,
  supportingText = null,
  infoTip = null,
  disabled = false,
  required = false,
  policyOutline = null,
  style,
}) {
  const [focused, setFocused] = React.useState(false);
  const options = HEMISPHERES[axis];
  const maxDegree = MAX_DEGREE[axis];
  const degreePlaceholder = "0".repeat(DEGREE_DIGITS[axis]);
  const degreeInputWidth = DEGREE_INPUT_WIDTH[axis];
  const showWarning = warning && !error;
  const frame = fieldFrameStyle({ error, focused, warning: showWarning, policyOutline });

  return (
    <FieldShell
      label={label}
      required={required}
      infoTip={infoTip}
      frame={frame}
      disabled={disabled}
      supportingText={supportingText}
      error={error}
      warning={showWarning}
      style={{ width: 280 + (degreeInputWidth - DEGREE_INPUT_WIDTH.lat), ...style }}
    >
      <input
        type="text"
        inputMode="numeric"
        placeholder={degreePlaceholder}
        disabled={disabled}
        value={degree ?? ""}
        onFocus={() => setFocused(true)}
        onBlur={() => setFocused(false)}
        onChange={(e) => onChangeDegree && onChangeDegree(maskDegreeInput(e.target.value, maxDegree))}
        aria-label={`${typeof label === "string" ? label : "Position"} degrees (0-${maxDegree})`}
        style={{
          width: degreeInputWidth,
          border: "none",
          outline: "none",
          background: "transparent",
          fontFamily: "var(--font-mono)",
          fontVariantNumeric: "tabular-nums",
          fontSize: 14,
          color: "var(--color-on-surface)",
          textAlign: "right",
        }}
      />
      <span className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>°</span>
      <input
        type="text"
        inputMode="decimal"
        placeholder="00.000"
        disabled={disabled}
        value={minutes ?? ""}
        onFocus={() => setFocused(true)}
        onBlur={() => setFocused(false)}
        onChange={(e) => onChangeMinutes && onChangeMinutes(maskMinutesInput(e.target.value))}
        aria-label={`${typeof label === "string" ? label : "Position"} minutes (0-${MAX_MINUTES})`}
        style={{
          width: 78,
          marginLeft: 4,
          border: "none",
          outline: "none",
          background: "transparent",
          fontFamily: "var(--font-mono)",
          fontVariantNumeric: "tabular-nums",
          fontSize: 14,
          color: "var(--color-on-surface)",
          textAlign: "right",
        }}
      />
      <span className="md-body-medium" style={{ color: "var(--color-on-surface-variant)", marginRight: 8 }}>&rsquo;</span>
      <div
        style={{
          display: "flex",
          marginLeft: "auto",
          borderRadius: "var(--shape-extra-small)",
          border: "1px solid var(--color-outline-variant)",
          overflow: "hidden",
          flexShrink: 0,
        }}
      >
        {options.map((opt) => (
          <button
            key={opt}
            type="button"
            disabled={disabled}
            onClick={() => onChangeHemisphere && onChangeHemisphere(opt)}
            aria-pressed={hemisphere === opt}
            style={{
              width: 28,
              height: 28,
              border: "none",
              cursor: disabled ? "not-allowed" : "pointer",
              background: hemisphere === opt ? "var(--color-secondary-container)" : "transparent",
              color: hemisphere === opt ? "var(--color-on-secondary-container)" : "var(--color-on-surface-variant)",
              fontFamily: "var(--font-body)",
              fontSize: 13,
              fontWeight: 600,
              transition: "background-color var(--motion-duration-short) var(--motion-easing-standard)",
            }}
          >
            {opt}
          </button>
        ))}
      </div>
    </FieldShell>
  );
}
