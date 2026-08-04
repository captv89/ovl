// SPDX-License-Identifier: AGPL-3.0-only

import type { Schema, SchemaField } from "../../api/client";
import type { FieldValue } from "./FieldRow";
import { DATE_TIME_PAIRS } from "./fieldLayout";

// Converts one field's raw form value (always string | boolean in
// FieldRow's own state model, since native inputs only ever hand back
// strings) into the JSON type vessel/httpapi's report endpoints expect:
// pkg/validation.checkFieldFormat requires a real JSON number for
// wholeNumber/decimal fields and a real JSON boolean for boolean fields —
// sending everything as a string would make every populated numeric
// field fail its own type check. An empty/undefined value becomes null
// (not omitted): pkg/validation.Report.IsEmpty already treats a null
// Fields entry the same as an absent one, and unlike omitting the key
// entirely, null survives round-tripping through a save diff as an
// explicit "this was cleared" instruction.
function coerceFieldValue(field: SchemaField, raw: FieldValue | undefined): unknown {
  if (raw === undefined) return null;
  if (field.type === "boolean") return raw;
  if (typeof raw === "string") {
    if (field.type === "wholeNumber" || field.type === "decimal") {
      if (raw.trim() === "") return null;
      const n = Number(raw);
      return Number.isNaN(n) ? raw : n;
    }
    return raw;
  }
  return raw;
}

// The reverse of coerceFieldValue: a persisted report's raw JSON field
// value (number for wholeNumber/decimal, bool for boolean, string
// otherwise) back into FieldRow's string|boolean state model — needed to
// resume an existing draft (design handoff A3's in-progress cards) rather
// than only ever seeding fresh prefill values.
export function decodeFieldsPayload(schema: Schema, fields: Record<string, unknown>): Record<string, FieldValue> {
  const out: Record<string, FieldValue> = {};
  for (const f of schema.fields) {
    const raw = fields[f.name];
    if (raw === undefined || raw === null) continue;
    out[f.name] = typeof raw === "boolean" ? raw : String(raw);
  }
  return out;
}

// Builds the initial fields payload for POST /api/reports: every schema
// field with a non-empty value, coerced to its JSON type. Fields with no
// value yet are simply omitted rather than sent as null — there is
// nothing to "clear" on a report that doesn't exist yet.
export function buildFieldsPayload(schema: Schema, values: Record<string, FieldValue>): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const f of schema.fields) {
    const coerced = coerceFieldValue(f, values[f.name]);
    if (coerced !== null) out[f.name] = coerced;
  }
  return out;
}

// Builds the changes payload for PATCH /api/reports/{id}: every schema
// field whose coerced value differs from what was last persisted,
// including fields cleared back to null.
export function buildChangesPayload(
  schema: Schema,
  values: Record<string, FieldValue>,
  saved: Record<string, FieldValue>,
): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const f of schema.fields) {
    if (values[f.name] === saved[f.name]) continue;
    out[f.name] = coerceFieldValue(f, values[f.name]);
  }
  return out;
}

// Same diff as buildChangesPayload, grouped by schema section
// (architecture 9.5: "section saves are independent writes") so each
// section's changes can be PATCHed separately — one section's lock
// conflict (Phase 4 Step 6) then never blocks another section's save.
export function buildChangesBySection(
  schema: Schema,
  values: Record<string, FieldValue>,
  saved: Record<string, FieldValue>,
): Record<string, Record<string, unknown>> {
  const out: Record<string, Record<string, unknown>> = {};
  for (const f of schema.fields) {
    if (values[f.name] === saved[f.name]) continue;
    const bucket = out[f.section] ?? (out[f.section] = {});
    bucket[f.name] = coerceFieldValue(f, values[f.name]);
  }
  return out;
}

// Combines whichever DATE_TIME_PAIRS split pair the loaded schema actually
// has (Log Abstract's Date_UTC/Time_UTC, or Bunker/EDN's own delivery-date/
// time pair — Phase 6) into the RFC3339 instant pkg/domain.NewReport needs
// for chain ordering (architecture 8.2/8.3). Falls back to "now" when no
// pair is fully populated yet — there is no due-event time to seed from for
// a schema with no cadence concept (Bunker/EDN), and even for Log Abstract
// a fresh report has to start somewhere; the officer can still correct the
// date/time fields before saving.
export function computeEventTime(values: Record<string, FieldValue>): string {
  for (const [dateField, timeField] of DATE_TIME_PAIRS) {
    const date = values[dateField];
    const time = values[timeField];
    if (typeof date === "string" && date && typeof time === "string" && time) {
      return `${date}T${time}:00Z`;
    }
  }
  return new Date().toISOString();
}

// Unlike computeEventTime (which silently falls back to "now" so a fresh
// report always has *some* event time to persist), this distinguishes
// "the officer has actually entered a date/time pair" from that
// fallback — 18.07.26 manual-test items 4/9's "Fetch sensor data" button
// only makes sense once a real event time exists to query the sensor
// service's window against.
export function hasEventTimeEntered(values: Record<string, FieldValue>): boolean {
  return DATE_TIME_PAIRS.some(([dateField, timeField]) => {
    const date = values[dateField];
    const time = values[timeField];
    return typeof date === "string" && date !== "" && typeof time === "string" && time !== "";
  });
}
