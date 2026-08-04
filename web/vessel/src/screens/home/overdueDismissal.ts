// SPDX-License-Identifier: AGPL-3.0-only

// 18.07.26 manual-test item 14: "I still see the ugly banner 'Report
// overdue - Overdue by 361h 6m'. I thought we agreed that these banners
// should be removable which then can reside in the notification
// section." Deferred at the time (2026-07-14 session, "Item 7") because
// no notification concept existed anywhere in the codebase yet — one now
// does (NotificationBell.tsx, built since). This is the dismiss half:
// same localStorage-only pattern as report-detail/chatReadMarker.ts (a
// convenience indicator, not synced state that matters beyond this one
// browser/session).
//
// Keyed by a deterministic id derived from the vessel's *current*
// overdue cycle (see overdueKeyFor) rather than a fixed string, so a
// dismissal doesn't silently suppress every future overdue banner
// forever — once a new report is submitted and the cadence clock resets,
// the key changes and the banner is un-dismissed for the new cycle.
const STORAGE_KEY_PREFIX = "ovl.overdueDismissed.";

export function overdueKeyFor(lastSubmittedReportId: string | undefined): string {
  return lastSubmittedReportId ?? "no-report-yet";
}

export function dismissOverdue(key: string): void {
  try {
    localStorage.setItem(STORAGE_KEY_PREFIX + key, "1");
  } catch {
    // localStorage can throw (private browsing, quota) — dismissal is a
    // convenience, not worth failing anything over.
  }
}

export function isOverdueDismissed(key: string): boolean {
  try {
    return localStorage.getItem(STORAGE_KEY_PREFIX + key) === "1";
  } catch {
    return false;
  }
}
