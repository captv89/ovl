// SPDX-License-Identifier: AGPL-3.0-only

import type { EventSuggestion, FieldPolicyState, ReportView, SchemaField } from "../../api/client";
import { effectiveState, isRequired } from "../report-form/fieldPolicy";

// Architecture 6.3's own stated default ("maximum gap between consecutive
// reports (default 12 hours)"); real per-vessel/group cadence rules are
// config-bundle data (6.5), which is Phase 3 — this is the same
// "local test config" stand-in pattern already used for field policy and
// prefill classes in Phase 2, just living on the frontend since nothing
// here is a validation rule the engine needs to enforce identically on
// both sides — it only drives an informational banner.
const MAX_GAP_HOURS = 12;
// How far ahead of the deadline the amber "due soon" banner starts
// showing — design handoff A3's own example ("due in 2h 10m") implies a
// window on this order, not the full 12 hours.
const DUE_SOON_WINDOW_HOURS = 2;

export type CadenceStatus =
  | { kind: "none" }
  | { kind: "dueSoon"; dueAt: Date }
  | { kind: "overdue"; dueAt: Date };

export function cadenceStatus(
  lastReportEventTime: string | undefined,
  now: Date,
  // The resolved cadence ceiling from this vessel's applied config bundle
  // (GET /api/vessel-config). Defaults to MAX_GAP_HOURS so a caller with no
  // bundle loaded yet — or an un-synced vessel — behaves exactly as before.
  maxGapHours: number = MAX_GAP_HOURS,
): CadenceStatus {
  if (!lastReportEventTime) return { kind: "none" };
  const dueAt = new Date(new Date(lastReportEventTime).getTime() + maxGapHours * 3600_000);
  if (now.getTime() >= dueAt.getTime()) return { kind: "overdue", dueAt };
  if (dueAt.getTime() - now.getTime() <= DUE_SOON_WINDOW_HOURS * 3600_000) return { kind: "dueSoon", dueAt };
  return { kind: "none" };
}

// "1h 20m" — matches design handoff A3's own wording exactly ("due in 2h
// 10m", "overdue by 1h 20m"), not a generic duration formatter.
export function formatDurationShort(ms: number): string {
  const totalMinutes = Math.max(0, Math.round(Math.abs(ms) / 60_000));
  const h = Math.floor(totalMinutes / 60);
  const m = totalMinutes % 60;
  if (h === 0) return `${m}m`;
  return `${h}h ${m}m`;
}

// No suggestion-table entry exists for a vessel's very first-ever report
// (there is no "after" state yet) — Departure is the natural start of a
// voyage log, a judgment call rather than spec text.
const FIRST_EVER_SUGGESTION = ["Departure"];

export function suggestedEventTypes(suggestions: EventSuggestion[], lastEventType: string | undefined): string[] {
  if (!lastEventType) return FIRST_EVER_SUGGESTION;
  const entry = suggestions.find((s) => s.after === lastEventType);
  return entry && entry.suggest.length > 0 ? entry.suggest : FIRST_EVER_SUGGESTION;
}

export function latestReport(reports: ReportView[]): ReportView | undefined {
  return reports.reduce<ReportView | undefined>((latest, r) => {
    if (!latest || new Date(r.eventTime).getTime() > new Date(latest.eventTime).getTime()) return r;
    return latest;
  }, undefined);
}

// Same as latestReport, restricted to reports that were actually
// submitted at some point (submittedAt set — this covers a report
// invalidated-after-submit too, unlike a plain state === "submitted"
// check, since cascade invalidation flips state without clearing
// submittedAt). 18.07.26 manual-test item 3: the cadence/"time since
// last report" banner used to source from latestReport (any state,
// including an in-progress draft or a still-draft report that cascade
// invalidated before it was ever submitted — vessel/httpapi/
// reports.go's runCascade doc comment on InvalidatedFrom), which could
// make the overdue calculation run against a report that was never a
// real report at all.
export function lastSubmittedReport(reports: ReportView[]): ReportView | undefined {
  return latestReport(reports.filter((r) => !!r.submittedAt));
}

// Fraction of this report's required (schema/company-mandatory) fields
// that are filled — the same "required" notion FieldRow's own asterisk
// uses (isRequired), not a separate metric invented for this card.
export function completionFraction(
  fields: SchemaField[],
  policy: Record<string, FieldPolicyState>,
  values: Record<string, unknown>,
  events?: Record<string, string[]>,
  eventType?: string,
): number {
  const required = fields.filter((f) => isRequired(effectiveState(f, policy, events, eventType)));
  if (required.length === 0) return 1;
  const filled = required.filter((f) => {
    const v = values[f.name];
    return v !== undefined && v !== null && v !== "";
  });
  return filled.length / required.length;
}

export function sortByEventTimeDesc(reports: ReportView[]): ReportView[] {
  return [...reports].sort((a, b) => new Date(b.eventTime).getTime() - new Date(a.eventTime).getTime());
}
