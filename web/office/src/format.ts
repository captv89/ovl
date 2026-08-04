// SPDX-License-Identifier: AGPL-3.0-only

// "2d 8h" / "9h 40m" — design handoff B1/B2's own overdue-by wording
// (dashboard's Overdue vessels table shows exactly this format). Shared
// by VesselList.tsx and Dashboard.tsx, both of which render the same
// overdueHours value. Distinct from web/vessel's own formatDurationShort
// ("1h 20m"), which never needs a days component since a vessel's own
// due-soon window is well under 24h; an office-side overdue gap can span
// days.
export function formatOverdueDuration(hours: number): string {
  const totalMinutes = Math.max(0, Math.round(hours * 60));
  const days = Math.floor(totalMinutes / 1440);
  const remHours = Math.floor((totalMinutes % 1440) / 60);
  const minutes = totalMinutes % 60;
  if (days > 0) return `${days}d ${remHours}h`;
  return `${remHours}h ${minutes}m`;
}

// syncStaleHoursThreshold: a vessel's own configured sync interval
// (vessel/httpapi's syncSettingsView) isn't synced to office, so there's
// no per-vessel cadence to compare against the way overdueHours compares
// against a compliance cadence rule. This is a flat heuristic instead —
// twice the vessel side's own allowed maximum interval
// (syncIntervalMaxSeconds = 24h in vessel/httpapi/settings.go), so a
// vessel configured for the least-frequent allowed sync still reads
// "Online" right up to a missed cycle before flipping to "Stale".
const syncStaleHoursThreshold = 48;

// syncHealth summarizes a vessel's VesselView.lastSyncAt for the fleet
// list and vessel detail screens — "when did we last hear from this
// vessel, and does that look healthy." Shared so both screens read the
// same threshold rather than drifting apart.
export function syncHealth(lastSyncAt: string | undefined): { label: string; tone: "success" | "warning" | "neutral" } {
  if (!lastSyncAt) return { label: "Never synced", tone: "neutral" };
  const hoursSince = (Date.now() - new Date(lastSyncAt).getTime()) / (1000 * 60 * 60);
  if (hoursSince > syncStaleHoursThreshold) return { label: "Stale", tone: "warning" };
  return { label: "Online", tone: "success" };
}
