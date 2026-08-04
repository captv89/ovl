import * as React from "react";

export interface SegmentedButtonOption {
  value: string;
  label: string;
  /** Rendered dimmed with a lock icon and disabled — used for a choice the user cannot make themselves (e.g. schemaMandatory). */
  locked?: boolean;
}

export interface SegmentedButtonProps {
  options: SegmentedButtonOption[];
  value: string;
  onChange?: (value: string) => void;
  style?: React.CSSProperties;
}

export declare function SegmentedButton(props: SegmentedButtonProps): React.ReactElement;
