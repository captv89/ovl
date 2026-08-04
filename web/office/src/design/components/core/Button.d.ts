import * as React from "react";

export type ButtonVariant = "filled" | "tonal" | "outlined" | "text" | "elevated";
export type ButtonSize = "small" | "medium" | "large";

export interface ButtonProps {
  children: React.ReactNode;
  /** Visual emphasis variant. @default "filled" */
  variant?: ButtonVariant;
  /** Height/density variant. @default "medium" */
  size?: ButtonSize;
  /** Material Symbols ligature name shown before the label, e.g. "anchor". */
  icon?: string | null;
  disabled?: boolean;
  onClick?: () => void;
  style?: React.CSSProperties;
}

export declare function Button(props: ButtonProps): React.ReactElement;
