import * as React from "react";

export type ChipType = "assist" | "filter" | "input" | "suggestion";

export interface ChipProps {
  label: string;
  icon?: string | null;
  type?: ChipType;
  selected?: boolean;
  /** Provide to render a trailing remove (x) affordance — used for input chips. */
  onDelete?: (() => void) | null;
  onClick?: () => void;
  style?: React.CSSProperties;
}

export declare function Chip(props: ChipProps): React.ReactElement;
