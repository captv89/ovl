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
}

export declare function DataTable(props: DataTableProps): React.ReactElement;
