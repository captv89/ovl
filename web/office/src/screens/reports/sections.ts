// SPDX-License-Identifier: AGPL-3.0-only

import type { SchemaDetail } from "../../api/client";

// Section keys in schema-declaration order. Ported from
// web/vessel/src/screens/report-form/sections.ts (apps don't share code
// — see that file's own precedent for vendored-copy duplication across
// vessel/office — office's SchemaDetail.sections is already always
// populated by office/httpapi, unlike vessel's optional field, but the
// same first-seen-field fallback is kept for robustness rather than
// assuming that invariant holds forever).
export function sectionsInOrder(schema: SchemaDetail): string[] {
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

// Human-readable labels for the curated schemas' section keys — verbatim
// copy of web/vessel/src/screens/report-form/sections.ts's own map, kept
// in sync by hand the same way the two apps' vendored Tideline
// components are (see CLAUDE.md's design-system-maintenance rule).
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

export function sectionLabel(key: string): string {
  if (SECTION_LABELS[key]) return SECTION_LABELS[key];
  return key
    .replace(/\./g, " ")
    .replace(/([a-z])([A-Z])/g, "$1 $2")
    .replace(/^./, (c) => c.toUpperCase());
}
