// SPDX-License-Identifier: AGPL-3.0-only

import * as React from "react";

export type DataTableColumnType = "text" | "number" | "iconText" | "badge" | "symbol";

export type BadgeTone = "success" | "warning" | "error" | "info" | "neutral";

export interface DataTableColumn {
  key: string;
  label: string;
  /** Omit for a pure `render`/`headerRender` column with no built-in cell type. */
  type?: DataTableColumnType;
  /** Enables the header sort toggle (asc → desc → none) for this column. */
  sortable?: boolean;
  /** Enables the header filter dropdown (checkbox list of unique values) for this column. */
  filterable?: boolean;
  /**
   * Renders the cell as arbitrary content instead of a `type`-based
   * built-in (e.g. a real status-badge component with its own tone
   * mapping, not DataTable's generic success/warning/error/info/
   * neutral). Search/sort/filter still read `row[column.key]` as plain
   * text, so give that key a sensible string value even when `render`
   * is set.
   */
  render?: (row: Record<string, any>) => React.ReactNode;
  /**
   * Renders this column's <th> content as arbitrary content instead of
   * the default label + sort toggle + filter dropdown (e.g. a
   * "select all" checkbox for a leading selection column). sortable/
   * filterable are ignored on a column that sets this.
   */
  headerRender?: () => React.ReactNode;
}

/** Shape of row[column.key] when column.type === "iconText". */
export interface IconTextCell {
  icon: string;
  text: string;
  subtext?: string;
}

/** Shape of row[column.key] when column.type === "badge". */
export interface BadgeCell {
  label: string;
  tone?: BadgeTone;
}

/** Shape of row[column.key] when column.type === "symbol" — a standalone centered icon glyph. */
export interface SymbolCell {
  icon: string;
  label?: string;
  color?: string;
}

export interface DataTableProps {
  columns: DataTableColumn[];
  /** Each row is a plain object keyed by column.key; id (if present) is used as the React key. */
  rows: Record<string, any>[];
  /** When set, adds a trailing action-arrow column and calls this on click — omit entirely for tables with no single per-row drill-in (e.g. one with its own inline per-row controls via column `render` instead). */
  onRowAction?: (row: Record<string, any>) => void;
  searchPlaceholder?: string;
  emptyMessage?: string;
  /** Suppresses the built-in search box for a screen that already owns its own search/filter toolbar above the table. */
  hideSearch?: boolean;
  /** Optional per-row style (e.g. a colored left border to flag a locked/mandatory row) — merged onto the <tr> after DataTable's own border/cursor/hover styles. */
  rowStyle?: (row: Record<string, any>) => React.CSSProperties | undefined;
  /** When set, renders an uppercase section-divider row before each run of consecutive rows sharing a group key. Suppressed while a column sort is active. */
  groupBy?: (row: Record<string, any>) => string | null | undefined;
  /** Called with the post-search/filter/sort row array whenever it changes — lets a caller with its own selection state (e.g. a "select all" checkbox) track what's actually visible. */
  onVisibleRowsChange?: (rows: Record<string, any>[]) => void;
  /** Caps the table at this many visible rows and scrolls the remainder inside itself with the header pinned. Measured from real row heights, so variable-height rows are handled. Group-divider rows count toward the cap. */
  maxRows?: number;
}

export declare function DataTable(props: DataTableProps): React.ReactElement;
