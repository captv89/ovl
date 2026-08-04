// SPDX-License-Identifier: AGPL-3.0-only

import { useState } from "react";

// Design handoff Part B's global group filter ("narrows every screen —
// dashboard, vessels, reports"), rendered as the top bar's `.gfilter`
// pill. Single-select (a vessel can carry multiple group tags, but the
// global filter narrows to one at a time, same as the wireframe's own
// "All groups ▾" control) — VesselList's own local group chips treat
// this as a starting default, not a replacement for that local control.
// (ReportList used to have its own local Group field too, but the
// 2026-08-02 Reports redesign dropped it — group scoping now lives only
// here.)
// Popover anatomy (fixed backdrop + absolute panel) matches DataTable's
// own column-filter dropdown, the only existing dropdown-menu pattern in
// this design system — kept consistent rather than inventing a second one.
export function GroupFilterMenu({
  groups,
  selected,
  onChange,
}: {
  groups: string[];
  selected: string | null;
  onChange: (group: string | null) => void;
}) {
  const [open, setOpen] = useState(false);

  return (
    <div style={{ position: "relative" }}>
      <button
        onClick={() => setOpen((v) => !v)}
        className="md-label-large"
        style={{
          display: "inline-flex",
          alignItems: "center",
          gap: 6,
          height: 32,
          padding: "0 12px",
          borderRadius: "var(--shape-full)",
          border: "1px solid var(--color-outline)",
          background: "var(--color-surface-container-low)",
          color: "var(--color-on-surface)",
          cursor: "pointer",
        }}
      >
        <span className="material-symbols-rounded" style={{ fontSize: 17, color: "var(--color-on-surface-variant)" }}>
          filter_list
        </span>
        {selected ?? "All groups"}
        <span className="material-symbols-rounded" style={{ fontSize: 18, color: "var(--color-on-surface-variant)" }}>
          expand_more
        </span>
      </button>

      {open ? (
        <>
          <div onClick={() => setOpen(false)} style={{ position: "fixed", inset: 0, zIndex: 20 }} />
          <div
            style={{
              position: "absolute",
              top: "100%",
              left: 0,
              marginTop: 4,
              zIndex: 21,
              minWidth: 200,
              maxHeight: 280,
              overflowY: "auto",
              background: "var(--color-surface-container-high)",
              borderRadius: "var(--shape-small)",
              boxShadow: "var(--elevation-2)",
              padding: "var(--space-2)",
            }}
          >
            <MenuItem label="All groups" active={selected === null} onClick={() => { onChange(null); setOpen(false); }} />
            {groups.map((g) => (
              <MenuItem key={g} label={g} active={selected === g} onClick={() => { onChange(g); setOpen(false); }} />
            ))}
            {groups.length === 0 ? (
              <div className="md-body-small" style={{ padding: "6px 8px", color: "var(--color-on-surface-variant)" }}>No groups yet</div>
            ) : null}
          </div>
        </>
      ) : null}
    </div>
  );
}

function MenuItem({ label, active, onClick }: { label: string; active: boolean; onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      className="md-body-medium"
      style={{
        display: "block",
        width: "100%",
        textAlign: "left",
        padding: "8px 10px",
        borderRadius: "var(--shape-extra-small)",
        border: "none",
        cursor: "pointer",
        background: active ? "var(--color-secondary-container)" : "transparent",
        color: active ? "var(--color-on-secondary-container)" : "var(--color-on-surface)",
      }}
    >
      {label}
    </button>
  );
}
