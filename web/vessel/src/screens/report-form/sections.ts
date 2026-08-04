// SPDX-License-Identifier: AGPL-3.0-only

import type { FieldPolicyState, Schema } from "../../api/client";
import { effectiveState } from "./fieldPolicy";

// Section keys in schema-declaration order, falling back to first-seen
// field order for curated schemas that don't list `sections` explicitly.
// Shared by ReportForm and the read-only report-detail screen (design
// handoff A7) — both need to walk a schema's sections in the same order.
export function sectionsInOrder(schema: Schema): string[] {
  if (schema.sections && schema.sections.length > 0) return schema.sections;
  const seen = new Set<string>();
  const out: string[] = [];
  for (const f of schema.fields) {
    if (!seen.has(f.section)) {
      seen.add(f.section);
      out.push(f.section);
    }
  }
  return out;
}

// Human-readable labels for the curated schemas' section keys
// (schemas/ovd-3.13/*.json — raw camelCase/dotted keys, architecture
// 5.1). Presentation-only; unknown keys fall back to a title-cased
// version of the raw key so other schemas' sections still render
// sensibly.
const SECTION_LABELS: Record<string, string> = {
  header: "Basic",
  voyage: "Voyage",
  position: "Position",
  times: "Times",
  distanceAndSpeed: "Distance & Speed",
  cargo: "Cargo",
  weather: "Weather",
  "engine.consumption": "Engine Consumption",
  "engine.performance": "Engine Performance",
  rob: "ROB",
  emissionsExtras: "Emissions & Extras",
  remarks: "Remarks",
  details: "Details",
  fuelProperties: "Fuel Properties",
  attachments: "Attachments",
};

// sectionsInOrder filtered to drop any section whose every field resolves
// to "hidden" (company policy hid them all, or none apply to eventType) —
// a section like Emissions & Extras shouldn't occupy a nav slot the crew
// can never do anything with. Mirrors effectiveState's own hidden test
// (fieldPolicy.ts), field by field.
export function visibleSectionsInOrder(
  schema: Schema,
  policy: Record<string, FieldPolicyState>,
  events?: Record<string, string[]>,
  eventType?: string,
): string[] {
  const bySection = new Map<string, typeof schema.fields>();
  for (const f of schema.fields) {
    const list = bySection.get(f.section) ?? [];
    list.push(f);
    bySection.set(f.section, list);
  }
  return sectionsInOrder(schema).filter((key) =>
    (bySection.get(key) ?? []).some((f) => effectiveState(f, policy, events, eventType) !== "hidden"),
  );
}

export function sectionLabel(key: string): string {
  if (SECTION_LABELS[key]) return SECTION_LABELS[key];
  // camelCase/dotted -> Title Case, best-effort fallback.
  return key
    .replace(/\./g, " ")
    .replace(/([a-z])([A-Z])/g, "$1 $2")
    .replace(/^./, (c) => c.toUpperCase());
}
