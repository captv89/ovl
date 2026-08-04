// SPDX-License-Identifier: AGPL-3.0-only

import React from "react";
import { createPortal } from "react-dom";

/**
 * MultiSelectMenu — SegmentedButtonMenu's multi-select sibling: a compact
 * trigger that opens an anchored popover of checkable options, for picking
 * several values where a table column has no room for inline controls.
 *
 * The empty selection is meaningful, not a null state: it renders as
 * `allLabel` ("All events") and is what an unconfigured row shows. That is
 * why there is no placeholder/required treatment — selecting nothing is a
 * valid, common answer.
 *
 * Two shapes, because this control has to sit convincingly in two places:
 * - default (no `label`): a 36px filled pill matching SegmentedButtonMenu,
 *   for use inside a dense grid cell.
 * - `label` set: the 56px outlined shell Select/TextField use, so it can
 *   stand next to real Selects in a toolbar without looking unboxed.
 *
 * The popover renders through a portal at fixed coordinates rather than as
 * an absolutely-positioned child. It has to: the field-policy grid lives in
 * an `overflow: auto` scroller, which clips an absolute popover at the
 * container edge — for any row below the fold that left the options
 * unreachable, so a selection could be made but never taken back.
 *
 * Same locked semantics as SegmentedButtonMenu: when `locked` is set the
 * trigger is a static pill with a lock icon and no chevron, rather than a
 * menu the user can open but not act on.
 */
export function MultiSelectMenu({ options, value, onChange, allLabel = "All", label = null, locked = false, style }) {
  const [open, setOpen] = React.useState(false);
  const [filter, setFilter] = React.useState("");
  const [activeIndex, setActiveIndex] = React.useState(-1);
  const [rect, setRect] = React.useState(null);
  const rootRef = React.useRef(null);
  const triggerRef = React.useRef(null);
  const popoverRef = React.useRef(null);
  const filterRef = React.useRef(null);
  const listboxId = React.useId();
  const selected = value ?? [];
  const narrowed = selected.length > 0;
  const boxed = label != null;

  // Long vocabularies (the OVD event-type enum is 33 entries) get an inline
  // filter; short ones would only be cluttered by it.
  const filterable = options.length > 12;
  const shown = filterable && filter.trim() ? options.filter((o) => o.toLowerCase().includes(filter.trim().toLowerCase())) : options;

  const summary = selected.length === 0 ? allLabel : selected.length === 1 ? selected[0] : `${selected.length} selected`;
  const fullTitle = narrowed ? selected.join(", ") : allLabel;

  const reposition = React.useCallback(() => {
    const el = triggerRef.current;
    if (!el) return;
    const r = el.getBoundingClientRect();
    const width = Math.max(r.width, 280);
    const spaceBelow = window.innerHeight - r.bottom;
    const desired = Math.min(360, 44 + shown.length * 40 + (filterable ? 48 : 0));
    // Flip above the trigger when the menu would otherwise run off the
    // bottom of the viewport and there is more room up top.
    const flip = spaceBelow < desired && r.top > spaceBelow;
    setRect({
      left: Math.max(8, Math.min(r.left, window.innerWidth - width - 8)),
      top: flip ? null : r.bottom + 4,
      bottom: flip ? window.innerHeight - r.top + 4 : null,
      width,
      maxHeight: Math.max(160, (flip ? r.top : spaceBelow) - 12),
    });
  }, [filterable, shown.length]);

  React.useLayoutEffect(() => {
    if (!open) return undefined;
    reposition();
    // Capture-phase so this also tracks the grid's own scroll container, not
    // just the window.
    window.addEventListener("scroll", reposition, true);
    window.addEventListener("resize", reposition);
    return () => {
      window.removeEventListener("scroll", reposition, true);
      window.removeEventListener("resize", reposition);
    };
  }, [open, reposition]);

  React.useEffect(() => {
    if (!open) return undefined;
    function onDocMouseDown(e) {
      const inRoot = rootRef.current && rootRef.current.contains(e.target);
      const inPopover = popoverRef.current && popoverRef.current.contains(e.target);
      if (!inRoot && !inPopover) setOpen(false);
    }
    document.addEventListener("mousedown", onDocMouseDown);
    return () => document.removeEventListener("mousedown", onDocMouseDown);
  }, [open]);

  React.useEffect(() => {
    if (!open) return;
    if (filterable) filterRef.current?.focus();
    else popoverRef.current?.focus();
  }, [open, filterable]);

  function close({ refocus = true } = {}) {
    setOpen(false);
    setFilter("");
    setActiveIndex(-1);
    if (refocus) triggerRef.current?.focus();
  }

  function toggle(opt) {
    if (!onChange) return;
    onChange(selected.includes(opt) ? selected.filter((v) => v !== opt) : [...selected, opt]);
  }

  function clear() {
    if (onChange) onChange([]);
  }

  function onMenuKeyDown(e) {
    switch (e.key) {
      case "ArrowDown":
        e.preventDefault();
        setActiveIndex((i) => Math.min(i + 1, shown.length - 1));
        break;
      case "ArrowUp":
        e.preventDefault();
        setActiveIndex((i) => Math.max(i - 1, 0));
        break;
      case "Home":
        e.preventDefault();
        setActiveIndex(0);
        break;
      case "End":
        e.preventDefault();
        setActiveIndex(shown.length - 1);
        break;
      case "Enter":
      case " ":
        // Space is a literal character in the filter box; only claim it when
        // the user is actually navigating the list.
        if (e.key === " " && filterable && activeIndex < 0) break;
        e.preventDefault();
        if (activeIndex >= 0 && shown[activeIndex] !== undefined) toggle(shown[activeIndex]);
        break;
      case "Escape":
        e.preventDefault();
        close();
        break;
      case "Tab":
        close({ refocus: false });
        break;
      default:
        break;
    }
  }

  function onTriggerKeyDown(e) {
    if (locked) return;
    if (e.key === "ArrowDown" || e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      setOpen(true);
    }
  }

  const shellBackground = boxed
    ? "none"
    : locked
      ? "var(--color-surface-container-highest)"
      : narrowed
        ? "var(--color-tertiary-container)"
        : "var(--color-surface-container-highest)";
  const shellColor = locked
    ? "var(--color-on-surface-variant)"
    : narrowed && !boxed
      ? "var(--color-on-tertiary-container)"
      : "var(--color-on-surface)";
  const borderColor = open ? "var(--color-primary)" : "var(--color-outline)";

  return (
    <div ref={rootRef} style={{ position: "relative", display: "inline-block", width: style?.width }}>
      <div
        style={{
          display: "flex",
          alignItems: "center",
          height: boxed ? 56 : 36,
          borderRadius: "var(--shape-small)",
          border: boxed ? `1px solid ${borderColor}` : "none",
          background: shellBackground,
          color: shellColor,
          overflow: "hidden",
          opacity: locked ? "var(--state-disabled-opacity)" : 1,
          ...style,
        }}
      >
        <button
          ref={triggerRef}
          type="button"
          role="combobox"
          onClick={() => !locked && (open ? close({ refocus: false }) : setOpen(true))}
          onKeyDown={onTriggerKeyDown}
          disabled={locked}
          aria-haspopup="listbox"
          aria-expanded={open}
          aria-controls={open ? listboxId : undefined}
          aria-label={boxed ? `${label}: ${fullTitle}` : undefined}
          title={locked ? `${fullTitle} (locked)` : fullTitle}
          style={{
            flex: 1,
            minWidth: 0,
            display: "flex",
            alignItems: "center",
            gap: 6,
            height: "100%",
            padding: boxed ? "0 4px 0 12px" : "0 4px 0 12px",
            border: "none",
            background: "none",
            color: "inherit",
            cursor: locked ? "default" : "pointer",
            fontFamily: "var(--font-body)",
            textAlign: "left",
          }}
        >
          {locked ? <span className="material-symbols-rounded" aria-hidden="true" style={{ fontSize: 15 }}>lock</span> : null}
          <span style={{ display: "flex", flexDirection: "column", minWidth: 0, overflow: "hidden" }}>
            {boxed ? (
              <span className="md-body-small" style={{ color: "var(--color-on-surface-variant)", whiteSpace: "nowrap" }}>{label}</span>
            ) : null}
            <span
              className={boxed ? "md-body-large" : undefined}
              style={{
                whiteSpace: "nowrap",
                overflow: "hidden",
                textOverflow: "ellipsis",
                fontSize: boxed ? undefined : "13.5px",
                fontWeight: boxed ? undefined : 500,
                color: narrowed ? "inherit" : "var(--color-on-surface-variant)",
              }}
            >
              {summary}
            </span>
          </span>
          {!locked ? (
            <span
              className="material-symbols-rounded"
              aria-hidden="true"
              style={{
                fontSize: 17,
                marginLeft: "auto",
                color: "var(--color-on-surface-variant)",
                transform: open ? "rotate(180deg)" : "none",
                transition: "transform var(--motion-duration-short)",
              }}
            >
              expand_more
            </span>
          ) : null}
        </button>
        {/* Deselecting everything is the single most common correction here
            (narrow a field by mistake, put it back to all events), so it gets
            a one-click affordance on the trigger instead of only living
            inside the menu. Sibling button rather than nested, since a
            button inside a button is invalid. */}
        {narrowed && !locked ? (
          <button
            type="button"
            onClick={clear}
            aria-label={`Clear selection, apply to ${allLabel.toLowerCase()}`}
            title={`Clear selection, apply to ${allLabel.toLowerCase()}`}
            style={{
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              width: 28,
              height: "100%",
              padding: 0,
              border: "none",
              borderLeft: "1px solid var(--color-outline-variant)",
              background: "none",
              color: "var(--color-on-surface-variant)",
              cursor: "pointer",
              flexShrink: 0,
            }}
          >
            <span className="material-symbols-rounded" aria-hidden="true" style={{ fontSize: 16 }}>close</span>
          </button>
        ) : null}
      </div>
      {open && rect
        ? createPortal(
            <div
              ref={popoverRef}
              tabIndex={-1}
              onKeyDown={onMenuKeyDown}
              style={{
                position: "fixed",
                left: rect.left,
                top: rect.top ?? undefined,
                bottom: rect.bottom ?? undefined,
                width: rect.width,
                maxHeight: rect.maxHeight,
                display: "flex",
                flexDirection: "column",
                zIndex: 1000,
                background: "var(--color-surface-container-high)",
                borderRadius: "var(--shape-medium)",
                boxShadow: "var(--elevation-2)",
                outline: "none",
                overflow: "hidden",
              }}
            >
              {filterable ? (
                <div style={{ padding: "var(--space-2) var(--space-3)", flexShrink: 0 }}>
                  <input
                    ref={filterRef}
                    value={filter}
                    onChange={(e) => {
                      setFilter(e.target.value);
                      setActiveIndex(-1);
                    }}
                    placeholder="Filter…"
                    aria-label="Filter options"
                    style={{
                      width: "100%",
                      boxSizing: "border-box",
                      height: 32,
                      padding: "0 10px",
                      border: "1px solid var(--color-outline-variant)",
                      borderRadius: "var(--shape-small)",
                      background: "var(--color-surface-container-lowest)",
                      color: "var(--color-on-surface)",
                      font: "inherit",
                      fontSize: "13px",
                    }}
                  />
                </div>
              ) : null}
              <div
                id={listboxId}
                role="listbox"
                aria-multiselectable="true"
                aria-label={label ?? allLabel}
                style={{ overflowY: "auto", padding: "var(--space-1) 0", flex: 1 }}
              >
                <button
                  type="button"
                  role="option"
                  aria-selected={selected.length === 0}
                  onMouseEnter={() => setActiveIndex(-1)}
                  onClick={clear}
                  style={{
                    width: "100%",
                    display: "flex",
                    alignItems: "center",
                    gap: "var(--space-3)",
                    padding: "10px var(--space-4)",
                    border: "none",
                    borderBottom: "1px solid var(--color-outline-variant)",
                    background: selected.length === 0 ? "var(--color-secondary-container)" : "none",
                    cursor: "pointer",
                    textAlign: "left",
                    font: "inherit",
                    color: selected.length === 0 ? "var(--color-on-secondary-container)" : "var(--color-on-surface)",
                  }}
                >
                  <span className="material-symbols-rounded" aria-hidden="true" style={{ fontSize: 18 }}>
                    {selected.length === 0 ? "radio_button_checked" : "radio_button_unchecked"}
                  </span>
                  <span className="md-body-medium">{allLabel}</span>
                </button>
                {shown.map((opt, i) => {
                  const isOn = selected.includes(opt);
                  const active = i === activeIndex;
                  return (
                    <button
                      key={opt}
                      id={`${listboxId}-opt-${i}`}
                      type="button"
                      role="option"
                      aria-selected={isOn}
                      onMouseEnter={() => setActiveIndex(i)}
                      onClick={() => toggle(opt)}
                      style={{
                        width: "100%",
                        display: "flex",
                        alignItems: "center",
                        gap: "var(--space-3)",
                        padding: "10px var(--space-4)",
                        border: "none",
                        borderLeft: `3px solid ${active ? "var(--color-primary)" : "transparent"}`,
                        background: isOn ? "var(--color-secondary-container)" : active ? "var(--color-surface-container-highest)" : "none",
                        cursor: "pointer",
                        textAlign: "left",
                        font: "inherit",
                        color: isOn ? "var(--color-on-secondary-container)" : "var(--color-on-surface)",
                      }}
                    >
                      {/* Both states draw a real box. The previous version hid
                          the unchecked one at opacity 0, which left nothing on
                          screen to say a row was a toggle at all. */}
                      <span
                        className="material-symbols-rounded"
                        aria-hidden="true"
                        style={{ fontSize: 18, color: isOn ? "inherit" : "var(--color-on-surface-variant)" }}
                      >
                        {isOn ? "check_box" : "check_box_outline_blank"}
                      </span>
                      <span className="md-body-medium" style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{opt}</span>
                    </button>
                  );
                })}
                {shown.length === 0 ? (
                  <div className="md-body-small" style={{ padding: "var(--space-3) var(--space-4)", color: "var(--color-on-surface-variant)" }}>
                    No matches.
                  </div>
                ) : null}
              </div>
              {narrowed ? (
                <div
                  className="md-body-small"
                  style={{
                    flexShrink: 0,
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                    gap: "var(--space-3)",
                    padding: "var(--space-2) var(--space-4)",
                    borderTop: "1px solid var(--color-outline-variant)",
                    color: "var(--color-on-surface-variant)",
                  }}
                >
                  <span>{selected.length} of {options.length} selected</span>
                  <button
                    type="button"
                    onClick={clear}
                    style={{
                      border: "none",
                      background: "none",
                      padding: 0,
                      font: "inherit",
                      color: "var(--color-primary)",
                      cursor: "pointer",
                    }}
                  >
                    Clear
                  </button>
                </div>
              ) : null}
            </div>,
            document.body,
          )
        : null}
    </div>
  );
}
