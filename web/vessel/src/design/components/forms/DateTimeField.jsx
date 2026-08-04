// SPDX-License-Identifier: AGPL-3.0-only

import React from "react";
import { FieldShell, fieldFrameStyle } from "./FieldShell.jsx";

/**
 * DateTimeField — an always-typable masked date/time input, with a themed
 * calendar/time popover (replacing the browser's native
 * input[type=date|time|datetime-local], which is OS chrome that can't be
 * restyled or dark-moded) as a secondary, optional way to fill it in.
 * UTC-only throughout (all internal date math uses the UTC Date accessors,
 * never local time) — matching the project's "no local time anywhere in
 * data entry" rule — and always labels the clock face "UTC" so that's
 * never ambiguous to the user.
 *
 * Typing is digit-driven masking (type "08072026", see "08-07-2026" appear
 * as you go) rather than a native-input diff, so Backspace/selecting-all-
 * then-retyping behave predictably without fighting cursor placement.
 * onChange only fires once a segment is *fully* typed (all digits present,
 * in-range) — an in-progress partial date never reaches the caller in a
 * half-valid state; range/calendar-day validity (e.g. Feb 30) is left to
 * the health-check plausibility rules, not this input.
 *
 * Value format:
 *   mode="date"     -> "YYYY-MM-DD"
 *   mode="time"     -> "HH:MM"
 *   mode="datetime" -> "YYYY-MM-DD HH:MM"
 * The combined mode uses a space, not "T", to match pkg/validation's
 * dateTimeLayout (which in turn matches the OVD 3.13 interface
 * description's own "yyyy-mm-dd hh:mm" format) — not the native
 * datetime-local input's "T" separator this component otherwise mimics.
 */
const MONTH_NAMES = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
const WEEKDAY_LABELS = ["Mo", "Tu", "We", "Th", "Fr", "Sa", "Su"];
const MAX_DIGITS = { date: 8, time: 4, datetime: 12 };
const MASK_PLACEHOLDER = { date: "DD-MM-YYYY", time: "HH:MM", datetime: "DD-MM-YYYY HH:MM" };

function pad2(n) {
  return String(n).padStart(2, "0");
}

function parseDatePart(value) {
  const m = value ? /^(\d{4})-(\d{2})-(\d{2})/.exec(value) : null;
  return m ? { year: +m[1], month: +m[2] - 1, day: +m[3] } : null;
}

function parseTimePart(value) {
  const m = value ? /(\d{2}):(\d{2})(?!.*\d{2}:\d{2})/.exec(value) : null;
  return m ? { hour: +m[1], minute: +m[2] } : null;
}

function daysInMonth(year, month) {
  return new Date(Date.UTC(year, month + 1, 0)).getUTCDate();
}

function shiftMonth(year, month, delta) {
  const d = new Date(Date.UTC(year, month + delta, 1));
  return { year: d.getUTCFullYear(), month: d.getUTCMonth() };
}

function nowUTC() {
  const d = new Date();
  return { year: d.getUTCFullYear(), month: d.getUTCMonth(), day: d.getUTCDate(), hour: d.getUTCHours(), minute: d.getUTCMinutes() };
}

// Always 42 cells (6 full weeks) so the grid's height never changes between
// months — avoids the popover jumping size as the user navigates.
function buildCells(year, month) {
  const firstWeekday = (new Date(Date.UTC(year, month, 1)).getUTCDay() + 6) % 7; // Monday = 0
  const total = daysInMonth(year, month);
  const prev = shiftMonth(year, month, -1);
  const prevTotal = daysInMonth(prev.year, prev.month);
  const next = shiftMonth(year, month, 1);
  const cells = [];
  for (let i = firstWeekday - 1; i >= 0; i--) cells.push({ day: prevTotal - i, inMonth: false, month: prev.month, year: prev.year });
  for (let d = 1; d <= total; d++) cells.push({ day: d, inMonth: true, month, year });
  let nextDay = 1;
  while (cells.length < 42) cells.push({ day: nextDay++, inMonth: false, month: next.month, year: next.year });
  return cells;
}

// Digits (DDMMYYYY / HHMM / DDMMYYYYHHMM) <-> the canonical stored value.
function digitsFromValue(value, mode) {
  const d = parseDatePart(value);
  const t = parseTimePart(value);
  const datePart = d ? `${pad2(d.day)}${pad2(d.month + 1)}${d.year}` : "";
  const timePart = t ? `${pad2(t.hour)}${pad2(t.minute)}` : "";
  if (mode === "date") return datePart;
  if (mode === "time") return timePart;
  // datetime: this component only ever produces both parts together
  // (commitDate/commitTime below always fill in the other half), so a
  // lone date-only or time-only value shouldn't occur in practice.
  return datePart + timePart;
}

function maskDate(digits) {
  const d = digits.slice(0, 2);
  const m = digits.slice(2, 4);
  const y = digits.slice(4, 8);
  let out = d;
  if (digits.length > 2) out += `-${m}`;
  if (digits.length > 4) out += `-${y}`;
  return out;
}

function maskTime(digits) {
  const h = digits.slice(0, 2);
  const mnt = digits.slice(2, 4);
  let out = h;
  if (digits.length > 2) out += `:${mnt}`;
  return out;
}

function maskDisplay(digits, mode) {
  if (mode === "date") return maskDate(digits);
  if (mode === "time") return maskTime(digits);
  const datePart = digits.slice(0, 8);
  const timePart = digits.slice(8, 12);
  let out = maskDate(datePart);
  if (digits.length > 8) out += ` ${maskTime(timePart)}`;
  return out;
}

// Returns the canonical stored value once digits fully (and validly) spell
// out this mode's segment(s), else null — null means "not committed yet",
// not "invalid forever": the caller just keeps typing.
function tryCommit(digits, mode) {
  function commitDate(ds) {
    if (ds.length !== 8) return null;
    const day = +ds.slice(0, 2);
    const month = +ds.slice(2, 4);
    const year = +ds.slice(4, 8);
    if (day < 1 || day > 31 || month < 1 || month > 12 || year < 1000) return null;
    return `${year}-${pad2(month)}-${pad2(day)}`;
  }
  function commitTime(ds) {
    if (ds.length !== 4) return null;
    const hour = +ds.slice(0, 2);
    const minute = +ds.slice(2, 4);
    if (hour > 23 || minute > 59) return null;
    return `${pad2(hour)}:${pad2(minute)}`;
  }
  if (mode === "date") return commitDate(digits);
  if (mode === "time") return commitTime(digits);
  if (digits.length !== 12) return null;
  const datePart = commitDate(digits.slice(0, 8));
  const timePart = commitTime(digits.slice(8, 12));
  return datePart && timePart ? `${datePart} ${timePart}` : null;
}

const navButtonStyle = {
  width: 32,
  height: 32,
  border: "none",
  background: "transparent",
  borderRadius: "var(--shape-full)",
  cursor: "pointer",
  color: "var(--color-on-surface-variant)",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
};

function TimeStepper({ value, max, onChange, ariaLabel }) {
  return (
    <div style={{ display: "flex", flexDirection: "column", alignItems: "center" }}>
      <button type="button" aria-label={`Increase ${ariaLabel}`} onClick={() => onChange((value + 1) % (max + 1))} style={{ ...navButtonStyle, width: 28, height: 20 }}>
        <span className="material-symbols-rounded" style={{ fontSize: 16 }}>expand_less</span>
      </button>
      <span className="md-readout-medium" style={{ minWidth: 32, textAlign: "center" }}>{pad2(value)}</span>
      <button type="button" aria-label={`Decrease ${ariaLabel}`} onClick={() => onChange((value - 1 + max + 1) % (max + 1))} style={{ ...navButtonStyle, width: 28, height: 20 }}>
        <span className="material-symbols-rounded" style={{ fontSize: 16 }}>expand_more</span>
      </button>
    </div>
  );
}

export function DateTimeField({ mode = "date", label, value, onChange, error = false, warning = false, supportingText = null, infoTip = null, disabled = false, required = false, policyOutline = null, style }) {
  const containerRef = React.useRef(null);
  const inputRef = React.useRef(null);
  const [open, setOpen] = React.useState(false);
  const [focused, setFocused] = React.useState(false);
  const [digits, setDigits] = React.useState(() => digitsFromValue(value, mode));
  const maxDigits = MAX_DIGITS[mode];

  // Re-sync from the canonical value whenever it changes from outside
  // (calendar pick, prefill, Clear/Now) — but never while the officer is
  // actively typing, or an in-progress keystroke would get clobbered by a
  // still-stale parent value on every render.
  React.useEffect(() => {
    if (!focused) setDigits(digitsFromValue(value, mode));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [value, mode]);

  const parsedDate = parseDatePart(value);
  const parsedTime = parseTimePart(value);
  const today = nowUTC();
  const [viewYear, setViewYear] = React.useState((parsedDate ?? today).year);
  const [viewMonth, setViewMonth] = React.useState((parsedDate ?? today).month);

  React.useEffect(() => {
    if (!open) return undefined;
    if (parsedDate) {
      setViewYear(parsedDate.year);
      setViewMonth(parsedDate.month);
    }
    function onDocMouseDown(e) {
      if (containerRef.current && !containerRef.current.contains(e.target)) setOpen(false);
    }
    function onKeyDown(e) {
      if (e.key === "Escape") setOpen(false);
    }
    document.addEventListener("mousedown", onDocMouseDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("mousedown", onDocMouseDown);
      document.removeEventListener("keydown", onKeyDown);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  // A partially-typed segment (some digits entered, not yet enough to
  // commit, and no longer focused so it's not just mid-keystroke) reads
  // as an error, same as an explicit validation error.
  const incomplete = !focused && digits.length > 0 && digits.length < maxDigits;
  const showWarning = warning && !error && !incomplete;
  const frame = fieldFrameStyle({ error: error || incomplete, focused: open || focused, warning: showWarning, policyOutline });

  function commit(nextDigits) {
    const value2 = tryCommit(nextDigits, mode);
    if (value2 !== null) onChange && onChange(value2);
  }

  function handleKeyDown(e) {
    if (disabled) return;
    if (/^[0-9]$/.test(e.key)) {
      e.preventDefault();
      const el = inputRef.current;
      const allSelected = !!el && el.selectionStart === 0 && el.selectionEnd === el.value.length && el.value.length > 0;
      const base = allSelected ? "" : digits;
      const next = (base + e.key).slice(0, maxDigits);
      setDigits(next);
      commit(next);
    } else if (e.key === "Backspace" || e.key === "Delete") {
      e.preventDefault();
      const next = digits.slice(0, -1);
      setDigits(next);
      if (next === "") onChange && onChange("");
    }
  }

  function commitDate(year, month, day) {
    const datePart = `${year}-${pad2(month + 1)}-${pad2(day)}`;
    if (mode === "date") {
      onChange && onChange(datePart);
      setOpen(false);
      return;
    }
    const timePart = parsedTime ? `${pad2(parsedTime.hour)}:${pad2(parsedTime.minute)}` : "00:00";
    onChange && onChange(`${datePart} ${timePart}`);
  }

  function commitTime(hour, minute) {
    if (mode === "time") {
      onChange && onChange(`${pad2(hour)}:${pad2(minute)}`);
      return;
    }
    const datePart = parsedDate
      ? `${parsedDate.year}-${pad2(parsedDate.month + 1)}-${pad2(parsedDate.day)}`
      : `${today.year}-${pad2(today.month + 1)}-${pad2(today.day)}`;
    onChange && onChange(`${datePart} ${pad2(hour)}:${pad2(minute)}`);
  }

  // Built as one combined value in a single onChange call — calling
  // commitDate then commitTime back to back would each read the other's
  // still-stale (pre-update) half from the current `value` prop and clobber
  // it, since this is a controlled component with no re-render between them.
  function goNow() {
    const n = nowUTC();
    if (mode === "date") onChange && onChange(`${n.year}-${pad2(n.month + 1)}-${pad2(n.day)}`);
    else if (mode === "time") onChange && onChange(`${pad2(n.hour)}:${pad2(n.minute)}`);
    else onChange && onChange(`${n.year}-${pad2(n.month + 1)}-${pad2(n.day)} ${pad2(n.hour)}:${pad2(n.minute)}`);
    setOpen(false);
  }

  const cells = mode !== "time" ? buildCells(viewYear, viewMonth) : [];

  return (
    <div ref={containerRef} style={{ position: "relative", width: 240, ...style }}>
      <FieldShell
        label={label}
        required={required}
        infoTip={infoTip}
        frame={frame}
        disabled={disabled}
        suffix={mode !== "date" ? <span className="md-label-small" style={{ color: "var(--color-on-surface-variant)", whiteSpace: "nowrap" }}>UTC</span> : null}
        supportingText={supportingText}
        error={error || incomplete}
        warning={showWarning}
        style={{ width: "100%" }}
      >
        <button
          type="button"
          aria-label="Open calendar picker"
          disabled={disabled}
          onClick={() => setOpen((o) => !o)}
          className="material-symbols-rounded"
          style={{ ...navButtonStyle, width: 24, height: 24, flexShrink: 0, marginLeft: -4, cursor: disabled ? "not-allowed" : "pointer" }}
        >
          {mode === "time" ? "schedule" : "calendar_month"}
        </button>
        <input
          ref={inputRef}
          type="text"
          inputMode="numeric"
          autoComplete="off"
          disabled={disabled}
          value={maskDisplay(digits, mode)}
          placeholder={MASK_PLACEHOLDER[mode]}
          onKeyDown={handleKeyDown}
          onChange={() => undefined /* fully keydown-driven; see handleKeyDown */}
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

      {open ? (
        <div
          role="dialog"
          aria-label={`${label} picker`}
          style={{
            position: "absolute",
            top: "calc(100% + 4px)",
            left: 0,
            zIndex: 30,
            background: "var(--color-surface-container)",
            borderRadius: "var(--shape-medium)",
            boxShadow: "var(--elevation-2)",
            padding: 16,
            width: mode === "time" ? 200 : 280,
          }}
        >
          {mode !== "time" ? (
            <>
              <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 8 }}>
                <button type="button" aria-label="Previous month" onClick={() => { const p = shiftMonth(viewYear, viewMonth, -1); setViewYear(p.year); setViewMonth(p.month); }} style={navButtonStyle}>
                  <span className="material-symbols-rounded">chevron_left</span>
                </button>
                <span className="md-title-small">{MONTH_NAMES[viewMonth]} {viewYear}</span>
                <button type="button" aria-label="Next month" onClick={() => { const n = shiftMonth(viewYear, viewMonth, 1); setViewYear(n.year); setViewMonth(n.month); }} style={navButtonStyle}>
                  <span className="material-symbols-rounded">chevron_right</span>
                </button>
              </div>
              <div style={{ display: "grid", gridTemplateColumns: "repeat(7, 1fr)", marginBottom: 4 }}>
                {WEEKDAY_LABELS.map((w) => (
                  <span key={w} className="md-label-small" style={{ textAlign: "center", color: "var(--color-on-surface-variant)" }}>{w}</span>
                ))}
              </div>
              <div style={{ display: "grid", gridTemplateColumns: "repeat(7, 1fr)", gap: 2 }}>
                {cells.map((cell, i) => {
                  const isSelected = !!(parsedDate && cell.year === parsedDate.year && cell.month === parsedDate.month && cell.day === parsedDate.day);
                  const isToday = cell.year === today.year && cell.month === today.month && cell.day === today.day;
                  return (
                    <button
                      key={i}
                      type="button"
                      onClick={() => commitDate(cell.year, cell.month, cell.day)}
                      style={{
                        height: 32,
                        borderRadius: "var(--shape-full)",
                        border: isToday && !isSelected ? "1px solid var(--color-primary)" : "1px solid transparent",
                        background: isSelected ? "var(--color-primary)" : "transparent",
                        color: isSelected ? "var(--color-on-primary)" : cell.inMonth ? "var(--color-on-surface)" : "var(--color-on-surface-variant)",
                        opacity: cell.inMonth ? 1 : 0.5,
                        cursor: "pointer",
                        fontFamily: "var(--font-body)",
                        fontSize: 13,
                      }}
                    >
                      {cell.day}
                    </button>
                  );
                })}
              </div>
            </>
          ) : null}

          {mode !== "date" ? (
            <div
              style={{
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                gap: 8,
                marginTop: mode === "time" ? 0 : 16,
                paddingTop: mode === "time" ? 0 : 12,
                borderTop: mode === "time" ? "none" : "1px solid var(--color-outline-variant)",
              }}
            >
              <TimeStepper value={parsedTime?.hour ?? 0} max={23} ariaLabel="hour" onChange={(h) => commitTime(h, parsedTime?.minute ?? 0)} />
              <span className="md-title-medium">:</span>
              <TimeStepper value={parsedTime?.minute ?? 0} max={59} ariaLabel="minute" onChange={(m) => commitTime(parsedTime?.hour ?? 0, m)} />
              <span className="md-label-medium" style={{ color: "var(--color-on-surface-variant)", marginLeft: 4 }}>UTC</span>
            </div>
          ) : null}

          <div style={{ display: "flex", justifyContent: "space-between", marginTop: 12, paddingTop: 8, borderTop: "1px solid var(--color-outline-variant)" }}>
            <button type="button" onClick={() => { onChange && onChange(""); setDigits(""); setOpen(false); }} className="md-label-large" style={{ background: "none", border: "none", color: "var(--color-on-surface-variant)", cursor: "pointer", padding: 4 }}>
              Clear
            </button>
            <button type="button" onClick={goNow} className="md-label-large" style={{ background: "none", border: "none", color: "var(--color-primary)", cursor: "pointer", padding: 4 }}>
              Now (UTC)
            </button>
          </div>
        </div>
      ) : null}
    </div>
  );
}
