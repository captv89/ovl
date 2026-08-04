// SPDX-License-Identifier: AGPL-3.0-only

import type { SchemaField } from "../../api/client";

// Mirrors pkg/validation.FieldPolicyState's five values (architecture
// 6.1) — kept as plain string literals here since the frontend only
// ever displays/round-trips them, never interprets their meaning itself
// (the rule engine is the single source of truth, per CLAUDE.md).
export const POLICY_STATES = [
  { value: "hidden", label: "Hidden" },
  { value: "optional", label: "Optional" },
  { value: "recommended", label: "Recommended" },
  { value: "companyMandatory", label: "Mandatory" },
  { value: "schemaMandatory", label: "Schema" },
] as const;

export const PREFILL_CLASSES = ["none", "carryForward", "computed", "ghost"];

// Mirrors pkg/validation/regulatory.go's profileRelevance switch exactly —
// see web/vessel's identical copy (screens/report-form/fieldPolicy.ts) for
// the full rationale. DRIFT HAZARD (codebase audit 2026-07-22 §6): this
// 9-string set is hand-copied in THREE places (Go + both frontends); any
// change must touch all three, and the Go side is guarded by
// TestProfileRelevance_RecognizedStringsSnapshot. The "verfication" typo is
// copied faithfully from the source OVD xlsx.
const GHG_RELEVANT_STRINGS = new Set<string>([
  "mandatory for MRV&DCS",
  "recommended for MRV&DCS",
  "mandatory for MRV",
  "voluntary wrt MRV",
  "for CII correction, voluntary wrt MRV",
  "for CII correction",
  "DSC only, voluntary wrt MRV",
  "mandatory for FEUM and in case of no fuel consumption for any verification",
  "recommended for voyage level verfication schemes",
]);

// appliesToEvent mirrors pkg/validation.FieldEvents.AppliesTo exactly. A
// field absent from the map — or one whose list is empty or contains the "*"
// wildcard — applies to every voyage event, and an empty eventType skips the
// gate entirely. DRIFT HAZARD: also mirrored in web/vessel's
// screens/report-form/fieldPolicy.ts; the Go side is authoritative.
export function appliesToEvent(
  fieldName: string,
  events: Record<string, string[]> | undefined,
  eventType: string | undefined,
): boolean {
  if (!eventType) return true;
  const list = events?.[fieldName];
  if (!list || list.length === 0) return true;
  return list.some((e) => e === "*" || e === eventType);
}

// effectiveState mirrors pkg/validation.FieldPolicy.StateForEvent: a
// schemaMandatory field is always schemaMandatory regardless of any stored
// override (SetPolicy never allows one to exist) and is never event-gated;
// a field the company scoped to specific voyage events is hidden on every
// other event; otherwise the stored override, or else "recommended" when the
// field's relevance ties it to a real regulatory profile, or else "optional".
//
// events/eventType are optional — omitting them gives the event-agnostic
// answer, which is what the editor shows when previewing "All events".
export function effectiveState(
  field: SchemaField,
  policy: Record<string, string>,
  events?: Record<string, string[]>,
  eventType?: string,
): string {
  if (field.schemaMandatory) return "schemaMandatory";
  if (!appliesToEvent(field.name, events, eventType)) return "hidden";
  const explicit = policy[field.name];
  if (explicit) return explicit;
  return GHG_RELEVANT_STRINGS.has(field.relevance.trim()) ? "recommended" : "optional";
}

export function effectivePrefill(field: SchemaField, prefill: Record<string, string>): string {
  return prefill[field.name] ?? "none";
}

// visibleFieldCount powers design handoff B6's "a Noon at sea report will
// show 47 fields in 6 sections" readout, now that the editor can actually
// preview one event: pass the previewed event type to count what that event
// renders, or omit it to count across all events (the "All events" default).
export function visibleFieldCount(
  fields: SchemaField[],
  policy: Record<string, string>,
  events?: Record<string, string[]>,
  eventType?: string,
): { total: number; bySection: Record<string, number> } {
  const bySection: Record<string, number> = {};
  let total = 0;
  for (const f of fields) {
    if (effectiveState(f, policy, events, eventType) === "hidden") continue;
    total++;
    bySection[f.section] = (bySection[f.section] ?? 0) + 1;
  }
  return { total, bySection };
}

// sectionsInOrder returns each field.section value once, in the order it
// first appears in fields — the curated schema's own field array is
// already section-grouped, so this needs no separate sections list.
export function sectionsInOrder(fields: SchemaField[]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const f of fields) {
    if (!seen.has(f.section)) {
      seen.add(f.section);
      out.push(f.section);
    }
  }
  return out;
}
