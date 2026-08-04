// SPDX-License-Identifier: AGPL-3.0-only

import * as React from "react";

export type IconButtonVariant = "standard" | "filled" | "tonal" | "outlined";

export interface IconButtonProps {
  /** Material Symbols ligature name, e.g. "navigation". */
  icon: string;
  variant?: IconButtonVariant;
  size?: "small" | "standard" | "large";
  selected?: boolean;
  disabled?: boolean;
  onClick?: () => void;
  "aria-label"?: string;
  style?: React.CSSProperties;
}

export declare function IconButton(props: IconButtonProps): React.ReactElement;
