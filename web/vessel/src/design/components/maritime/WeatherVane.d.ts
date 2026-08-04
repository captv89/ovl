import * as React from "react";

export interface WeatherVaneProps {
  windDirDeg?: string | number;
  windForceBft?: string | number;
  seaDirDeg?: string | number;
  seaForceDouglas?: string | number;
  swellDirDeg?: string | number;
  swellForceM?: string | number;
  currentDirDeg?: string | number;
  currentSpeedKn?: string | number;
  size?: number;
}

export declare function WeatherVane(props: WeatherVaneProps): React.ReactElement;
