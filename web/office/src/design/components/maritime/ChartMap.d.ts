import * as React from "react";

export interface ChartVessel {
  /** 0–1 normalized x within the chart panel. */
  x: number;
  /** 0–1 normalized y within the chart panel. */
  y: number;
  heading?: number;
  label: string;
  /** @deprecated use `status: "alert"` instead — kept as a red/navy-only shorthand. */
  alert?: boolean;
  /** Fixed bridge-alert marker color (green ok / amber caution / red alert). Wins over `alert` when both are given. */
  status?: "ok" | "caution" | "alert";
}
export interface ChartTrackPoint {
  x: number;
  y: number;
}

export interface ChartMapProps {
  vessels?: ChartVessel[];
  track?: ChartTrackPoint[];
  width?: number;
  height?: number;
  style?: React.CSSProperties;
  /** Called with the vessel's index in `vessels` when its marker (SVG shape or label) is clicked. Omit for a non-interactive map. */
  onVesselClick?: (index: number) => void;
}

export declare function ChartMap(props: ChartMapProps): React.ReactElement;
