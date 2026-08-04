// SPDX-License-Identifier: AGPL-3.0-only

import * as React from "react";

export interface SegmentedButtonMenuOption {
  value: string;
  label: string;
  /** Rendered dimmed with a lock icon and disabled — used for a choice the user cannot make themselves (e.g. schemaMandatory). */
  locked?: boolean;
}

export interface SegmentedButtonMenuProps {
  options: SegmentedButtonMenuOption[];
  value: string;
  onChange?: (value: string) => void;
  style?: React.CSSProperties;
}

export declare function SegmentedButtonMenu(props: SegmentedButtonMenuProps): React.ReactElement;
