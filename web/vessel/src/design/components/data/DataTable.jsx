// SPDX-License-Identifier: AGPL-3.0-only

import React from "react";

const TONE_MAP = {
  success: { bg: "var(--color-status-underway-container)", fg: "var(--color-status-underway)" },
  warning: { bg: "var(--color-status-caution-container)", fg: "var(--color-status-caution)" },
  error: { bg: "var(--color-status-warning-container)", fg: "var(--color-status-warning)" },
  info: { bg: "var(--color-tertiary-container)", fg: "var(--color-on-tertiary-container)" },
  neutral: { bg: "var(--color-surface-container-highest)", fg: "var(--color-on-surface-variant)" },
};

// Plain-string representation of a cell, used for search / sort / filter —
// independent of how the cell actually renders.
function cellText(col, row) {
  const v = row[col.key];
  if (v == null) return "";
  // A render() escape hatch can back a cell with any real component (e.g.
  // a maritime status badge with its own tone mapping) instead of one of
  // DataTable's own built-in shapes — row[col.key] is then whatever plain
  // value the caller already had (e.g. the raw lifecycle state string),
  // not necessarily the {label}/{text}/{icon} shape col.type implies. Read
  // it as plain text directly rather than reaching for a property that
  // shape doesn't have (previously: a badge column with a custom render()
  // always searched/sorted/filtered as "", since row[col.key] was a plain
  // string with no .label).
  if (col.render) return String(v);
  if (col.type === "iconText") return v.text ?? "";
  if (col.type === "badge") return v.label ?? "";
  if (col.type === "symbol") return v.label ?? v.icon ?? "";
  return String(v);
}

function Cell({ col, row }) {
  // An escape hatch for a real component (e.g. a maritime status badge
  // with its own tone mapping) instead of DataTable's generic
  // success/warning/error/info/neutral tones — cellText still reads
  // row[col.key] as plain text for search/sort/filter, so a render
  // column works with those for free as long as that key holds a
  // sensible string (e.g. the raw state, even though the cell displays
  // it as a styled badge).
  if (col.render) return col.render(row);

  const v = row[col.key];
  if (v == null) return <span className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>—</span>;

  if (col.type === "iconText") {
    return (
      <div style={{ display: "flex", alignItems: "center", gap: "var(--space-3)" }}>
        <span
          className="material-symbols-rounded"
          style={{
            width: 32, height: 32, borderRadius: "var(--shape-small)",
            background: "var(--color-surface-container-highest)", color: "var(--color-primary)",
            display: "inline-flex", alignItems: "center", justifyContent: "center",
            fontSize: 18, flexShrink: 0,
          }}
        >
          {v.icon}
        </span>
        <div>
          <div className="md-title-small">{v.text}</div>
          {v.subtext ? <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)" }}>{v.subtext}</div> : null}
        </div>
      </div>
    );
  }

  if (col.type === "badge") {
    const tone = TONE_MAP[v.tone] || TONE_MAP.neutral;
    return (
      <span
        className="md-label-medium"
        style={{
          display: "inline-flex", alignItems: "center", gap: 6, height: 24, padding: "0 10px",
          borderRadius: "var(--shape-full)", background: tone.bg, color: tone.fg,
        }}
      >
        {v.label}
      </span>
    );
  }

  if (col.type === "symbol") {
    return (
      <span
        title={v.label || ""}
        className="material-symbols-rounded"
        style={{ fontSize: 20, color: v.color || "var(--color-on-surface-variant)" }}
      >
        {v.icon}
      </span>
    );
  }

  if (col.type === "number") {
    return <span className="md-body-medium" style={{ fontFamily: "var(--font-mono)" }}>{v}</span>;
  }

  return <span className="md-body-medium">{v}</span>;
}

function SortIcon({ dir }) {
  const icon = dir === "asc" ? "arrow_upward" : dir === "desc" ? "arrow_downward" : "unfold_more";
  return (
    <span
      className="material-symbols-rounded"
      style={{ fontSize: 16, color: dir ? "var(--color-primary)" : "var(--color-on-surface-variant)" }}
    >
      {icon}
    </span>
  );
}

/**
 * DataTable — dense data grid with icon/badge/symbol cell types, a search box,
 * per-column sort, and per-column value filtering. Trailing column is a fixed
 * action-arrow button (row drill-in), independent of the column set.
 *
 * hideSearch suppresses the built-in search box for screens that already
 * own their own search/filter toolbar above the table (e.g. the field
 * policy editor) — the table itself still renders whatever rows the
 * caller already filtered, just without a second, redundant search
 * input. A column's headerRender() swaps its <th> content for something
 * other than label+sort+filter (e.g. a "select all" checkbox) — the
 * same per-column escape hatch col.render already gives cells, applied
 * to headers, so a fully custom header still keeps sort/filter on every
 * other column rather than forcing a bespoke table for one odd column.
 */
export function DataTable({ columns, rows, onRowAction, searchPlaceholder = "Search", emptyMessage = "No results.", hideSearch = false }) {
  const [query, setQuery] = React.useState("");
  const [sort, setSort] = React.useState({ key: null, dir: null });
  const [filters, setFilters] = React.useState({});
  const [openFilter, setOpenFilter] = React.useState(null);
  const [hoveredRowId, setHoveredRowId] = React.useState(null);

  // The whole row opens the row (not just the trailing arrow) — a
  // custom-rendered cell's own interactive control (a checkbox, a
  // "Delete" button) still needs to work without also triggering
  // navigation, so a click that lands on one of those is left alone
  // rather than bubbling into onRowAction.
  function handleRowClick(e, row) {
    if (!onRowAction) return;
    if (e.target.closest("button, input, a, label")) return;
    onRowAction(row);
  }

  const optionsByCol = React.useMemo(() => {
    const map = {};
    for (const col of columns) {
      if (!col.filterable) continue;
      map[col.key] = [...new Set(rows.map((r) => cellText(col, r)).filter((t) => t !== ""))].sort();
    }
    return map;
  }, [columns, rows]);

  const filtered = React.useMemo(() => {
    let out = rows;
    const q = query.trim().toLowerCase();
    if (q) {
      out = out.filter((r) => columns.some((col) => cellText(col, r).toLowerCase().includes(q)));
    }
    for (const [key, selected] of Object.entries(filters)) {
      if (!selected || selected.size === 0) continue;
      const col = columns.find((c) => c.key === key);
      out = out.filter((r) => selected.has(cellText(col, r)));
    }
    if (sort.key && sort.dir) {
      const col = columns.find((c) => c.key === sort.key);
      out = [...out].sort((a, b) => {
        const av = cellText(col, a), bv = cellText(col, b);
        const an = Number(av), bn = Number(bv);
        const cmp = !isNaN(an) && !isNaN(bn) && av !== "" && bv !== "" ? an - bn : av.localeCompare(bv);
        return sort.dir === "asc" ? cmp : -cmp;
      });
    }
    return out;
  }, [rows, columns, query, filters, sort]);

  function toggleSort(key) {
    setSort((s) => {
      if (s.key !== key) return { key, dir: "asc" };
      if (s.dir === "asc") return { key, dir: "desc" };
      return { key: null, dir: null };
    });
  }

  function toggleFilterValue(key, value) {
    setFilters((f) => {
      const next = new Set(f[key] || []);
      next.has(value) ? next.delete(value) : next.add(value);
      return { ...f, [key]: next };
    });
  }

  const th = { textAlign: "left", padding: "0 var(--space-4) var(--space-2)", color: "var(--color-on-surface-variant)" };

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-3)", fontFamily: "var(--font-body)" }}>
      {hideSearch ? null : (
        <div style={{ position: "relative", width: 260 }}>
          <span className="material-symbols-rounded" style={{ position: "absolute", left: 10, top: 8, fontSize: 18, color: "var(--color-on-surface-variant)" }}>search</span>
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={searchPlaceholder}
            className="md-body-medium"
            style={{
              width: "100%", height: 36, padding: "0 12px 0 34px", boxSizing: "border-box",
              borderRadius: "var(--shape-small)", border: "1px solid var(--color-outline-variant)",
              background: "var(--color-surface)", color: "var(--color-on-surface)", outline: "none",
            }}
          />
        </div>
      )}

      <div style={{ border: "1px solid var(--color-outline-variant)", borderRadius: "var(--shape-medium)", overflow: "visible", background: "var(--color-surface)" }}>
        <table style={{ width: "100%", borderCollapse: "collapse" }}>
          <thead>
            <tr style={{ borderBottom: "1px solid var(--color-outline-variant)" }}>
              {columns.map((col) => {
                const activeCount = filters[col.key] ? filters[col.key].size : 0;
                return (
                  <th key={col.key} className="md-label-medium" style={{ ...th, paddingTop: "var(--space-3)", position: "relative" }}>
                    {col.headerRender ? col.headerRender() : (
                    <React.Fragment>
                    <div style={{ display: "flex", alignItems: "center", gap: 4 }}>
                      <span
                        onClick={() => col.sortable && toggleSort(col.key)}
                        style={{ display: "flex", alignItems: "center", gap: 4, cursor: col.sortable ? "pointer" : "default", userSelect: "none" }}
                      >
                        {col.label}
                        {col.sortable ? <SortIcon dir={sort.key === col.key ? sort.dir : null} /> : null}
                      </span>
                      {col.filterable ? (
                        <span
                          onClick={() => setOpenFilter(openFilter === col.key ? null : col.key)}
                          className="material-symbols-rounded"
                          style={{
                            fontSize: 16, cursor: "pointer", borderRadius: "var(--shape-full)",
                            color: activeCount ? "var(--color-primary)" : "var(--color-on-surface-variant)",
                            background: activeCount ? "var(--color-primary-container)" : "transparent",
                            width: 20, height: 20, display: "inline-flex", alignItems: "center", justifyContent: "center",
                          }}
                        >
                          filter_list
                        </span>
                      ) : null}
                    </div>

                    {openFilter === col.key ? (
                      <React.Fragment>
                        <div onClick={() => setOpenFilter(null)} style={{ position: "fixed", inset: 0, zIndex: 20 }} />
                        <div
                          style={{
                            position: "absolute", top: "100%", left: 0, marginTop: 4, zIndex: 21,
                            minWidth: 180, maxHeight: 220, overflowY: "auto",
                            background: "var(--color-surface-container-high)", borderRadius: "var(--shape-small)",
                            boxShadow: "var(--elevation-2)", padding: "var(--space-2)",
                          }}
                        >
                          {(optionsByCol[col.key] || []).map((opt) => (
                            <label
                              key={opt}
                              className="md-body-medium"
                              style={{ display: "flex", alignItems: "center", gap: 8, padding: "6px 8px", borderRadius: "var(--shape-extra-small)", cursor: "pointer", fontWeight: 400 }}
                            >
                              <input
                                type="checkbox"
                                checked={!!(filters[col.key] && filters[col.key].has(opt))}
                                onChange={() => toggleFilterValue(col.key, opt)}
                              />
                              {opt}
                            </label>
                          ))}
                          {(optionsByCol[col.key] || []).length === 0 ? (
                            <div className="md-body-small" style={{ padding: "6px 8px", color: "var(--color-on-surface-variant)" }}>No values</div>
                          ) : null}
                        </div>
                      </React.Fragment>
                    ) : null}
                    </React.Fragment>
                    )}
                  </th>
                );
              })}
              {onRowAction ? <th className="md-label-medium" style={{ ...th, paddingTop: "var(--space-3)", width: 48 }} /> : null}
            </tr>
          </thead>
          <tbody>
            {filtered.map((row, i) => (
              <tr
                key={row.id ?? i}
                onClick={(e) => handleRowClick(e, row)}
                onMouseEnter={() => onRowAction && setHoveredRowId(row.id ?? i)}
                onMouseLeave={() => setHoveredRowId(null)}
                style={{
                  borderBottom: i === filtered.length - 1 ? "none" : "1px solid var(--color-outline-variant)",
                  cursor: onRowAction ? "pointer" : "default",
                  background: onRowAction && hoveredRowId === (row.id ?? i) ? "var(--color-surface-container-high)" : "transparent",
                }}
              >
                {columns.map((col) => (
                  <td key={col.key} style={{ padding: "var(--space-3) var(--space-4)" }}>
                    <Cell col={col} row={row} />
                  </td>
                ))}
                {onRowAction ? (
                  <td style={{ padding: "var(--space-3) var(--space-4)", textAlign: "right" }}>
                    <button
                      onClick={() => onRowAction(row)}
                      aria-label="Open row"
                      style={{
                        width: 32, height: 32, borderRadius: "var(--shape-full)", border: "none",
                        background: "transparent", color: "var(--color-on-surface-variant)",
                        display: "inline-flex", alignItems: "center", justifyContent: "center", cursor: "pointer",
                      }}
                    >
                      <span className="material-symbols-rounded" style={{ fontSize: 20 }}>arrow_forward</span>
                    </button>
                  </td>
                ) : null}
              </tr>
            ))}
            {filtered.length === 0 ? (
              <tr>
                <td colSpan={onRowAction ? columns.length + 1 : columns.length} className="md-body-medium" style={{ padding: "var(--space-6)", textAlign: "center", color: "var(--color-on-surface-variant)" }}>
                  {emptyMessage}
                </td>
              </tr>
            ) : null}
          </tbody>
        </table>
      </div>
    </div>
  );
}
