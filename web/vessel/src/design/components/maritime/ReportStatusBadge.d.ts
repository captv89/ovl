import * as React from "react";

export type ReportLifecycleState =
  | "draft"
  | "ready"
  | "submitted"
  | "synced"
  | "pushed"
  | "remarked"
  | "invalidated"
  | "failed";

export interface ReportStatusBadgeProps {
  status: ReportLifecycleState;
  /** Small accent dot marking a v2+ (corrected/resubmitted) report. */
  resubmitted?: boolean;
}

export declare function ReportStatusBadge(props: ReportStatusBadgeProps): React.ReactElement;
