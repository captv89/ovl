import * as React from "react";

export interface DataReadoutProps {
  label: string;
  value: string | number;
  unit?: string;
  size?: "medium" | "large";
  trend?: "up" | "down" | null;
}

export declare function DataReadout(props: DataReadoutProps): React.ReactElement;
