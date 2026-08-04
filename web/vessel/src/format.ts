// SPDX-License-Identifier: AGPL-3.0-only

import type { ReportView } from "./api/client";

// Was defined identically in three screens (Home, the old BackupScreen,
// ReportDetailScreen) — one shared copy instead.
export function formatUtc(iso: string): string {
  return iso.replace("T", " ").replace(/(:\d{2})(:\d{2})?\.?\d*Z?$/, "$1") + " UTC";
}

export function voyageNumberOf(report: ReportView): string | undefined {
  const v = report.fields["Voyage_Number"];
  return typeof v === "string" && v !== "" ? v : undefined;
}
