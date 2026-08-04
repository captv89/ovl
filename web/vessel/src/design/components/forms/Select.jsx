// SPDX-License-Identifier: AGPL-3.0-only
// FORKED COMPONENT: web/vessel and web/office each keep their own copy of this component and they are intentionally NOT identical (vessel wireframe rework, 2026-07-13). A change here likely needs mirroring to the other app's copy — do not assume they match. See docs/codebase-audit-2026-07-22.md §6.

import React from "react";
import { FieldShell, fieldFrameStyle } from "./FieldShell.jsx";

/**
 * Select — label-above dropdown on FieldShell. Previously a bare
 * `<div onClick>` with no error/warning/disabled states and no keyboard
 * support at all — the only field primitive missing that parity (see
 * PROJECT.md's "Vessel UI rework" section). Rebuilt as a real listbox
 * button: `role="listbox"`/`role="option"`, full keyboard operation
 * (ArrowUp/Down move the highlight and open the menu if closed,
 * Home/End jump to the ends, Enter/Space selects, Escape closes),
 * closes on outside click or blur.
 *
 * The chevron in FieldShell's `suffix` slot carries its own
 * onClick/onMouseDown: a suffix there is a sibling of the value `<button>`,
 * not a descendant, so a click landing exactly on the chevron glyph never
 * reached the button's own onClick — the control only opened/closed when
 * clicked anywhere else in its box (2026-07-13 manual-test feedback:
 * "clicking on the arrow mark should show and hide the dropdown"). Synced
 * from the canonical Tideline `Select.jsx`, which already carried this
 * exact fix — vessel's copy had drifted behind it. `onMouseDown`'s
 * `preventDefault` stops the click from first blurring/losing focus
 * (which would fire the outside-click closer before the chevron's own
 * onClick runs).
 */
export function Select({
  label,
  value,
  options,
  onChange,
  placeholder = "Select…",
  supportingText = null,
  error = false,
  warning = false,
  disabled = false,
  required = false,
  infoTip = null,
  policyOutline = null,
  style,
}) {
  const [open, setOpen] = React.useState(false);
  const [activeIndex, setActiveIndex] = React.useState(-1);
  const rootRef = React.useRef(null);
  const listboxId = React.useId();

  const frame = fieldFrameStyle({ error, focused: open, warning, policyOutline });

  React.useEffect(() => {
    if (!open) return;
    function onOutside(e) {
      if (rootRef.current && !rootRef.current.contains(e.target)) setOpen(false);
    }
    document.addEventListener("mousedown", onOutside);
    return () => document.removeEventListener("mousedown", onOutside);
  }, [open]);

  function openAt(index) {
    setActiveIndex(index);
    setOpen(true);
  }

  function selectIndex(index) {
    const opt = options[index];
    if (opt !== undefined && onChange) onChange(opt);
    setOpen(false);
  }

  function handleKeyDown(e) {
    if (disabled) return;
    const currentIndex = Math.max(options.indexOf(value), 0);
    switch (e.key) {
      case "ArrowDown":
        e.preventDefault();
        if (!open) openAt(currentIndex);
        else setActiveIndex((i) => Math.min((i < 0 ? currentIndex : i) + 1, options.length - 1));
        break;
      case "ArrowUp":
        e.preventDefault();
        if (!open) openAt(currentIndex);
        else setActiveIndex((i) => Math.max((i < 0 ? currentIndex : i) - 1, 0));
        break;
      case "Home":
        if (open) {
          e.preventDefault();
          setActiveIndex(0);
        }
        break;
      case "End":
        if (open) {
          e.preventDefault();
          setActiveIndex(options.length - 1);
        }
        break;
      case "Enter":
      case " ":
        e.preventDefault();
        if (!open) openAt(currentIndex);
        else if (activeIndex >= 0) selectIndex(activeIndex);
        break;
      case "Escape":
        if (open) {
          e.preventDefault();
          setOpen(false);
        }
        break;
      case "Tab":
        setOpen(false);
        break;
      default:
        break;
    }
  }

  return (
    <FieldShell
      label={label}
      required={required}
      infoTip={infoTip}
      frame={frame}
      disabled={disabled}
      supportingText={supportingText}
      error={error}
      warning={warning}
      suffix={
        <span
          className="material-symbols-rounded"
          aria-hidden="true"
          onMouseDown={(e) => e.preventDefault()}
          onClick={() => (disabled ? null : open ? setOpen(false) : openAt(Math.max(options.indexOf(value), 0)))}
          style={{ fontSize: 18, color: "var(--color-on-surface-variant)", transform: open ? "rotate(180deg)" : "none", transition: "transform var(--motion-duration-short)", cursor: disabled ? "not-allowed" : "pointer" }}
        >
          expand_more
        </span>
      }
      style={{ position: "relative", ...style }}
    >
      <div ref={rootRef} style={{ position: "relative", width: "100%" }}>
        <button
          type="button"
          role="combobox"
          aria-haspopup="listbox"
          aria-expanded={open}
          aria-controls={listboxId}
          aria-activedescendant={open && activeIndex >= 0 ? `${listboxId}-opt-${activeIndex}` : undefined}
          disabled={disabled}
          onClick={() => (open ? setOpen(false) : openAt(Math.max(options.indexOf(value), 0)))}
          onKeyDown={handleKeyDown}
          style={{
            width: "100%",
            display: "block",
            textAlign: "left",
            border: "none",
            outline: "none",
            background: "transparent",
            padding: 0,
            font: "inherit",
            fontFamily: "var(--font-body)",
            fontSize: 14,
            color: value ? "var(--color-on-surface)" : "var(--color-on-surface-variant)",
            cursor: disabled ? "not-allowed" : "pointer",
          }}
        >
          {value || placeholder}
        </button>
        {open ? (
          <ul
            id={listboxId}
            role="listbox"
            aria-label={typeof label === "string" ? label : undefined}
            style={{
              listStyle: "none",
              margin: 0,
              position: "absolute",
              top: "calc(100% + 6px)",
              left: 0,
              right: 0,
              maxHeight: 260,
              overflowY: "auto",
              background: "var(--color-surface-container)",
              borderRadius: "var(--shape-small)",
              boxShadow: "var(--elevation-2)",
              padding: 4,
              zIndex: 20,
            }}
          >
            {options.map((opt, i) => {
              const selected = opt === value;
              const active = i === activeIndex;
              return (
                <li
                  key={opt}
                  id={`${listboxId}-opt-${i}`}
                  role="option"
                  aria-selected={selected}
                  onMouseEnter={() => setActiveIndex(i)}
                  onMouseDown={(e) => e.preventDefault()}
                  onClick={() => selectIndex(i)}
                  className="md-body-medium"
                  style={{
                    padding: "8px 12px",
                    borderRadius: "var(--shape-extra-small)",
                    cursor: "pointer",
                    background: active ? "var(--color-secondary-container)" : selected ? "var(--color-surface-container-highest)" : "transparent",
                    color: active ? "var(--color-on-secondary-container)" : "var(--color-on-surface)",
                  }}
                >
                  {opt}
                </li>
              );
            })}
          </ul>
        ) : null}
      </div>
    </FieldShell>
  );
}
