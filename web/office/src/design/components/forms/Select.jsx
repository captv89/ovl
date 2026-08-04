// SPDX-License-Identifier: AGPL-3.0-only
// FORKED COMPONENT: web/vessel and web/office each keep their own copy of this component and they are intentionally NOT identical (vessel wireframe rework, 2026-07-13). A change here likely needs mirroring to the other app's copy — do not assume they match. See docs/codebase-audit-2026-07-22.md §6.

import React from "react";
import { Tooltip } from "../feedback/Tooltip.jsx";

/**
 * Select — dropdown built on TextField's outlined shell. Gained real
 * error/warning/disabled/supportingText parity and keyboard operability
 * (ArrowUp/Down, Home/End, Enter/Space, Escape, close-on-outside-click) —
 * previously the only field primitive missing all of that, both here and
 * on the vessel side (see the vessel repo's PROJECT.md "Vessel UI rework"
 * section for the full history; this is the office-side forward-port of
 * that same fix, keeping office's own M3 visual shell rather than
 * adopting vessel's wireframe-driven anatomy, which has no office
 * equivalent spec yet).
 *
 * `policyOutline` — see TextField's own doc comment: drawn as this field's
 * own border rather than a second overlay frame.
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
  labelTooltip = null,
  policyOutline = null,
  style,
}) {
  const [open, setOpen] = React.useState(false);
  const [activeIndex, setActiveIndex] = React.useState(-1);
  const rootRef = React.useRef(null);
  const listboxId = React.useId();

  const policyActive = !error && !warning && !open && !!policyOutline;
  const borderColor = error
    ? "var(--color-error)"
    : open
      ? "var(--color-primary)"
      : warning
        ? "var(--color-status-caution)"
        : policyOutline
          ? "var(--color-secondary)"
          : "var(--color-outline)";
  const borderStyle = policyActive && policyOutline === "ghgRelevant" ? "dashed" : "solid";
  const borderWidth = policyActive || error || warning ? "1.5px" : "1px";
  const labelEl = (
    <span className="md-body-small" style={{ color: error ? "var(--color-error)" : warning ? "var(--color-status-caution)" : "var(--color-on-surface-variant)", whiteSpace: "nowrap", cursor: labelTooltip ? "help" : "default" }}>
      {label}
    </span>
  );

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
    <div ref={rootRef} style={{ display: "flex", flexDirection: "column", gap: 4, width: 240, opacity: disabled ? "var(--state-disabled-opacity)" : 1, ...style }}>
      <div style={{ position: "relative" }}>
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
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            height: 56,
            padding: "0 12px",
            borderRadius: "var(--shape-small)",
            borderTop: `${borderWidth} ${borderStyle} ${borderColor}`,
            borderRight: `${borderWidth} ${borderStyle} ${borderColor}`,
            borderBottom: `${borderWidth} ${borderStyle} ${borderColor}`,
            borderLeft: `${borderWidth} ${borderStyle} ${borderColor}`,
            background: "none",
            cursor: disabled ? "not-allowed" : "pointer",
            fontFamily: "var(--font-body)",
            textAlign: "left",
          }}
        >
          <div style={{ display: "flex", flexDirection: "column", minWidth: 0, overflow: "hidden" }}>
            {labelTooltip ? <Tooltip label={labelTooltip} maxWidth={240}>{labelEl}</Tooltip> : labelEl}
            <span className="md-body-large" style={{ whiteSpace: "nowrap", color: value ? "var(--color-on-surface)" : "var(--color-on-surface-variant)" }}>{value || placeholder}</span>
          </div>
          <span className="material-symbols-rounded" aria-hidden="true" style={{ color: "var(--color-on-surface-variant)", transform: open ? "rotate(180deg)" : "none", transition: "transform var(--motion-duration-short)" }}>expand_more</span>
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
              top: "104%",
              left: 0,
              right: 0,
              maxHeight: 260,
              overflowY: "auto",
              background: "var(--color-surface-container)",
              borderRadius: "var(--shape-small)",
              boxShadow: "var(--elevation-2)",
              padding: 4,
              zIndex: 5,
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
                  style={{
                    padding: "10px 16px",
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
      {supportingText ? (
        <span className="md-body-small" style={{ color: error ? "var(--color-error)" : warning ? "var(--color-status-caution)" : "var(--color-on-surface-variant)", paddingLeft: 12 }}>{supportingText}</span>
      ) : null}
    </div>
  );
}
