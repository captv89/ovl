// SPDX-License-Identifier: AGPL-3.0-only

// Design handoff A8's unread indicator on the report list/home: a plain
// localStorage "last opened this report's Chat tab at" timestamp per
// reportId is enough for Phase 5 — no server-side read cursor needed,
// since "unread" here only ever matters to the one browser/session that
// opened it, not something synced across devices or between vessel and
// office. isChatUnread is exercised by ReportDetailScreen's own
// mark-read call, but not yet wired onto ReportsScreen/Home's list rows
// (a known, noted gap rather than a silent omission): doing that needs
// each visible report's latest chat activity timestamp, which
// listReports/getReport don't carry today — adding it is a small,
// separate backend change (a field on reportView, or a per-report
// summary), not built in this pass since it wasn't load-bearing for
// T3.5's actual acceptance criteria (the round-trip + char-counter
// checks), only its task description.
const STORAGE_KEY_PREFIX = "ovl.chatReadAt.";

export function markReportChatRead(reportId: string): void {
  try {
    localStorage.setItem(STORAGE_KEY_PREFIX + reportId, new Date().toISOString());
  } catch {
    // localStorage can throw (private browsing, quota) — unread state is
    // a convenience indicator, not worth failing anything over.
  }
}

export function isChatUnread(reportId: string, latestMessageSentAt: string | null): boolean {
  if (!latestMessageSentAt) return false;
  try {
    const readAt = localStorage.getItem(STORAGE_KEY_PREFIX + reportId);
    if (!readAt) return true;
    return new Date(latestMessageSentAt).getTime() > new Date(readAt).getTime();
  } catch {
    return false;
  }
}
