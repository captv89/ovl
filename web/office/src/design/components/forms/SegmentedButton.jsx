// SPDX-License-Identifier: AGPL-3.0-only

import React from "react";

/**
 * SegmentedButton — a single-select row of connected segments (Material 3
 * segmented button). One segment may be locked (shown dimmed with a lock
 * icon, non-interactive) for a value the user cannot choose themselves.
 */
export function SegmentedButton({ options, value, onChange, style }) {
  return (
    <div
      role="radiogroup"
      style={{
        display: "inline-flex",
        border: "1px solid var(--color-outline-variant)",
        borderRadius: "var(--shape-full)",
        overflow: "hidden",
        fontFamily: "var(--font-body)",
        ...style,
      }}
    >
      {options.map((opt, i) => {
        const selected = opt.value === value;
        const locked = !!opt.locked;
        return (
          <button
            key={opt.value}
            role="radio"
            aria-checked={selected}
            disabled={locked}
            onClick={() => !locked && onChange && onChange(opt.value)}
            title={locked ? `${opt.label} (locked)` : opt.label}
            style={{
              display: "flex",
              alignItems: "center",
              gap: 6,
              height: 36,
              padding: "0 14px",
              border: "none",
              borderLeft: i === 0 ? "none" : "1px solid var(--color-outline-variant)",
              // Selected always wins the fill, even when locked — a
              // locked segment that IS the current value must still look
              // visibly different from a locked segment that isn't, or
              // there is no way to tell which state is actually in
              // effect on a schemaMandatory field (every segment would
              // render identically muted).
              background: selected
                ? "var(--color-secondary-container)"
                : locked
                ? "var(--color-surface-container-highest)"
                : "transparent",
              color: selected
                ? "var(--color-on-secondary-container)"
                : locked
                ? "var(--color-on-surface-variant)"
                : "var(--color-on-surface)",
              cursor: locked ? "not-allowed" : "pointer",
              fontSize: "var(--type-label-large-size)",
              fontWeight: 500,
              opacity: locked && !selected ? 0.7 : 1,
              whiteSpace: "nowrap",
            }}
          >
            {selected ? <span className="material-symbols-rounded" style={{ fontSize: 16 }}>check</span> : null}
            {locked ? <span className="material-symbols-rounded" style={{ fontSize: 14 }}>lock</span> : null}
            <span>{opt.label}</span>
          </button>
        );
      })}
    </div>
  );
}
