import * as React from "react";

export type AlertLevel = "ok" | "caution" | "warning";
export interface AlertBannerProps {
  /** Bridge alert convention: ok (green), caution (amber), warning (red). @default "caution" */
  level?: AlertLevel;
  title: string;
  message: string;
  onDismiss?: () => void;
}

export declare function AlertBanner(props: AlertBannerProps): React.ReactElement;
