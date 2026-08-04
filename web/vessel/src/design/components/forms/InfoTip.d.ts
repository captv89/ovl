import * as React from "react";

export interface InfoTipProps {
  label: React.ReactNode;
  /** Tooltip wrap width in px. Defaults to 220, matching the wireframe's own `.tip` width. */
  maxWidth?: number;
}

export declare function InfoTip(props: InfoTipProps): React.ReactElement;
