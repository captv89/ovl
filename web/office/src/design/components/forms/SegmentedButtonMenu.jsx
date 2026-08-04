// SPDX-License-Identifier: AGPL-3.0-only

import React from "react";

/**
 * SegmentedButtonMenu — a compact single-line trigger (current option's
 * label + chevron) that opens an anchored popover listing the same
 * options SegmentedButton renders inline. Use in place of SegmentedButton
 * wherever horizontal space is tight (e.g. a table column). Same locked-
 * option semantics as SegmentedButton: a locked option is never itself
 * selectable, and when the *current* value is locked the trigger renders
 * as a static pill (lock icon, no chevron, not clickable) rather than
 * opening a menu with nothing the user could pick.
 */
export function SegmentedButtonMenu({ options, value, onChange, style }) {
  const [open, setOpen] = React.useState(false);
  const containerRef = React.useRef(null);
  const current = options.find((o) => o.value === value);
  const currentLocked = !!current?.locked;

  React.useEffect(() => {
    if (!open) return undefined;
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
  }, [open]);

  return (
    <div ref={containerRef} style={{ position: "relative", display: "inline-block", ...style }}>
      <button
        type="button"
        onClick={() => !currentLocked && setOpen((o) => !o)}
        disabled={currentLocked}
        aria-haspopup="menu"
        aria-expanded={open}
        title={currentLocked ? `${current?.label ?? value} (locked)` : current?.label ?? value}
        style={{
          display: "inline-flex",
          alignItems: "center",
          gap: 6,
          height: 36,
          padding: "0 12px",
          border: "none",
          borderRadius: "var(--shape-small)",
          cursor: currentLocked ? "default" : "pointer",
          background: currentLocked ? "var(--color-surface-container-highest)" : "var(--color-secondary-container)",
          color: currentLocked ? "var(--color-on-surface-variant)" : "var(--color-on-secondary-container)",
          fontFamily: "var(--font-body)",
          fontSize: "13.5px",
          fontWeight: 500,
          whiteSpace: "nowrap",
          width: "100%",
          ...style,
        }}
      >
        {currentLocked ? <span className="material-symbols-rounded" style={{ fontSize: 15 }}>lock</span> : null}
        <span>{current?.label ?? value}</span>
        {!currentLocked ? <span className="material-symbols-rounded" style={{ fontSize: 17, marginLeft: "auto" }}>expand_more</span> : null}
      </button>
      {open ? (
        <div
          role="menu"
          style={{
            position: "absolute",
            top: "calc(100% + 4px)",
            left: 0,
            minWidth: 180,
            zIndex: 30,
            background: "var(--color-surface-container-high)",
            borderRadius: "var(--shape-medium)",
            boxShadow: "var(--elevation-2)",
            overflow: "hidden",
            padding: "var(--space-2) 0",
          }}
        >
          {options.map((opt) => {
            const selected = opt.value === value;
            const locked = !!opt.locked;
            return (
              <button
                key={opt.value}
                type="button"
                role="menuitemradio"
                aria-checked={selected}
                disabled={locked}
                title={locked ? `${opt.label} (locked)` : opt.label}
                onClick={() => {
                  setOpen(false);
                  if (!locked && onChange) onChange(opt.value);
                }}
                style={{
                  width: "100%",
                  display: "flex",
                  alignItems: "center",
                  gap: "var(--space-3)",
                  padding: "var(--space-2) var(--space-4)",
                  border: "none",
                  background: selected ? "var(--color-secondary-container)" : "none",
                  cursor: locked ? "not-allowed" : "pointer",
                  textAlign: "left",
                  font: "inherit",
                  color: selected
                    ? "var(--color-on-secondary-container)"
                    : locked
                    ? "var(--color-on-surface-variant)"
                    : "var(--color-on-surface)",
                  opacity: locked && !selected ? 0.7 : 1,
                }}
              >
                {selected ? <span className="material-symbols-rounded" style={{ fontSize: 16 }}>check</span> : null}
                {locked ? <span className="material-symbols-rounded" style={{ fontSize: 14 }}>lock</span> : null}
                <span className="md-body-medium">{opt.label}</span>
              </button>
            );
          })}
        </div>
      ) : null}
    </div>
  );
}
