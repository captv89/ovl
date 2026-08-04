import * as React from "react";

export interface ProgressIndicatorProps {
  variant?: "linear" | "circular";
  /** Omit for indeterminate; 0-100 for determinate. */
  value?: number | null;
  size?: number;
}

export declare function ProgressIndicator(props: ProgressIndicatorProps): React.ReactElement;
