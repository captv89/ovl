// SPDX-License-Identifier: AGPL-3.0-only

import type { FieldPolicyState, SchemaField } from "../../api/client";

export type FieldSpan = "normal" | "wide" | "full";

// Free-text fields long enough to need a multiline box (Remarks, voyage-leg
// narrative fields, etc.). maxLength alone isn't a perfect proxy for
// "multiline" — Contact_Email has a 254-char maxLength (the RFC 5321
// ceiling) but is still a single-line value — hence the small named
// override below rather than a length threshold alone.
const SINGLE_LINE_TEXT_OVERRIDE = new Set<string>(["Contact_Email"]);
const MULTILINE_MAX_LENGTH_THRESHOLD = 200;
const WIDE_MAX_LENGTH_THRESHOLD = 60;
const LONG_LABEL_THRESHOLD = 32;

export function isMultiline(field: SchemaField): boolean {
  return (
    field.type === "text" &&
    (field.maxLength ?? 0) >= MULTILINE_MAX_LENGTH_THRESHOLD &&
    !SINGLE_LINE_TEXT_OVERRIDE.has(field.name)
  );
}

// How much of the section grid a field's row should occupy. Numeric fields
// (the large majority of the curated schemas) and short text fields stay
// "normal" (one grid track); longer free text gets "wide" (two tracks);
// genuinely narrative fields (isMultiline) take the full row as a textarea.
export function fieldSpan(field: SchemaField): FieldSpan {
  if (isMultiline(field)) return "full";
  if (field.type === "text" && (field.maxLength ?? 0) > WIDE_MAX_LENGTH_THRESHOLD) return "wide";
  // A long label on a narrow numeric field (several OVD fields run
  // 40-70 characters, e.g. the Emissions/Extras section) has nowhere to
  // go in a single 260px track and ellipsis-clips no matter how the
  // label itself is styled — give it a second track's worth of room
  // regardless of the field's own type/maxLength.
  if (field.label.length > LONG_LABEL_THRESHOLD) return "wide";
  return "normal";
}

// Native <input> type per OVD field type, so numeric fields get a numeric
// keypad/spinner and date/time fields get the browser's own picker instead
// of every field rendering as an identical plain text box. Architecture 6
// is silent on input widgets; this is the standard HTML mapping.
export function nativeInputType(field: SchemaField): "text" | "number" | "date" | "time" | "datetime-local" {
  switch (field.type) {
    case "wholeNumber":
    case "decimal":
      return "number";
    case "date":
      return "date";
    case "time":
      return "time";
    case "dateTime":
      return "datetime-local";
    default:
      return "text";
  }
}

export interface FieldGroup {
  label: string | null;
  fields: SchemaField[];
}

const MIN_GROUP_SIZE = 2;

// The name-token pass below derives a group heading straight from the
// fields' shared leading token (e.g. "ME", "Wind") — usually fine as-is,
// but "Entry" (Entry_Made_By_1/2) reads as a fragment rather than a
// heading, since these fields were only ever a name-token pair, not a
// group with a naturally headline-worthy shared word.
const GROUP_LABEL_OVERRIDES: Record<string, string> = {
  Entry: "Entered by",
};

// Hand-curated exceptions to the two mechanical passes below, for runs
// they split in a genuinely misleading way. Checked before either pass
// runs, on an exact contiguous field-name match only — a no-op for every
// schema that doesn't carry this exact run.
//
// log-abstract's "voyage" section: Activity_Mode shares a name-token
// ("Activity") with Activity_1, so the primary pass groups just those two
// as "Activity"; Time_Elapsed_Activity_1/Activity_2/Time_Elapsed_Activity_2
// then get merged separately by the label-prefix pass (their labels all
// share the leading word "Activity") — landing as a *second*, identically-
// labeled "Activity" group right next to the first. Per the OVD interface
// description's "Offshore modes and Activities" sheet, Activity_Mode
// (current operational mode) and the Activity_1/2 + duration pairs (a
// distinct activity-time log) are two related but separate concepts —
// still one group's worth of UI, not two boxes with the same unexplained
// heading.
const MANUAL_CLUSTERS: ReadonlyArray<{ label: string; names: readonly string[] }> = [
  {
    label: "Offshore Activity",
    names: ["Activity_Mode", "Activity_1", "Time_Elapsed_Activity_1", "Activity_2", "Time_Elapsed_Activity_2"],
  },
];

function firstToken(name: string): string {
  const idx = name.indexOf("_");
  return idx === -1 ? name : name.slice(0, idx);
}

function matchManualCluster(fields: SchemaField[], i: number): FieldGroup | null {
  for (const cluster of MANUAL_CLUSTERS) {
    if (fields[i].name !== cluster.names[0]) continue;
    const run = fields.slice(i, i + cluster.names.length);
    if (run.length === cluster.names.length && run.every((f, k) => f.name === cluster.names[k])) {
      return { label: cluster.label, fields: run };
    }
  }
  return null;
}

// Longest shared leading run of words between two label strings, e.g.
// "Direction Course" vs. "Direction Heading" -> ["Direction"]. Case-
// insensitive so mechanically-derived and hand-authored labels compare
// the same way.
function commonLeadingWords(a: string[], b: string[]): string[] {
  let n = 0;
  while (n < a.length && n < b.length && a[n].toLowerCase() === b[n].toLowerCase()) n++;
  return a.slice(0, n);
}

// Fields whose curated *names* don't share a leading token (so the
// primary pass below leaves them as singletons) can still be obviously
// related through their *labels* — e.g. Course/True_Heading, labeled
// "Direction Course"/"Direction Heading", share no name-token but
// plainly belong under one "Direction" heading. This second pass only
// ever merges runs of fields that the primary pass already left
// ungrouped (singletons), and only when consecutive singletons share a
// literal leading label word — conservative enough not to second-guess
// a grouping the name-token pass already made.
function mergeByLabelPrefix(singletons: SchemaField[]): FieldGroup[] {
  const out: FieldGroup[] = [];
  let k = 0;
  while (k < singletons.length) {
    let prefix = singletons[k].label.split(" ");
    let m = k + 1;
    while (m < singletons.length) {
      const next = commonLeadingWords(prefix, singletons[m].label.split(" "));
      if (next.length === 0) break;
      prefix = next;
      m++;
    }
    const run = singletons.slice(k, m);
    out.push({ label: run.length >= MIN_GROUP_SIZE ? prefix.join(" ") : null, fields: run });
    k = m;
  }
  return out;
}

// Clusters consecutive fields (in the schema's own curated order) into
// named groups, in two passes. No per-field category data survived OVD
// curation (only name/label/type/section were kept), so both passes are
// mechanical,
// schema-agnostic stand-ins rather than an editorial grouping — still a
// large legibility win over one flat list for sections with 40-150
// fields. Runs shorter than MIN_GROUP_SIZE render without a heading (a
// single field with no grouped neighbor isn't a "group").
//   1. Share a leading name token — e.g. ME_Consumption_HFO/LFO/MGO/...
//      under "ME", Wind_Dir/Wind_Force_* under "Wind".
//   2. Whatever's left ungrouped by (1) gets a second pass by shared
//      leading label word(s) — e.g. Course/True_Heading share no name
//      token but are both labeled "Direction ..." and belong together.
export function groupFields(fields: SchemaField[]): FieldGroup[] {
  const primary: FieldGroup[] = [];
  let i = 0;
  while (i < fields.length) {
    const manual = matchManualCluster(fields, i);
    if (manual) {
      primary.push(manual);
      i += manual.fields.length;
      continue;
    }
    const key = firstToken(fields[i].name);
    let j = i + 1;
    while (j < fields.length && firstToken(fields[j].name) === key) j++;
    const run = fields.slice(i, j);
    primary.push({ label: run.length >= MIN_GROUP_SIZE ? (GROUP_LABEL_OVERRIDES[key] ?? key) : null, fields: run });
    i = j;
  }

  // Second pass: try to merge consecutive runs of primary-pass singletons
  // by shared label prefix. Any already-labeled (size >= MIN_GROUP_SIZE)
  // primary group is left exactly as-is and acts as a hard boundary.
  const groups: FieldGroup[] = [];
  let buffer: SchemaField[] = [];
  function flushBuffer() {
    if (buffer.length > 0) groups.push(...mergeByLabelPrefix(buffer));
    buffer = [];
  }
  for (const g of primary) {
    if (g.label === null && g.fields.length === 1) {
      buffer.push(g.fields[0]);
    } else {
      flushBuffer();
      groups.push(g);
    }
  }
  flushBuffer();
  return groups;
}

export interface FieldEntry {
  field: SchemaField;
  state: FieldPolicyState;
}

export interface PlainEntry extends FieldEntry {
  kind: "field";
}

export interface PositionEntry {
  kind: "position";
  axis: "lat" | "lon";
  label: string;
  degree: SchemaField;
  minutes: SchemaField;
  hemisphere: SchemaField;
  state: FieldPolicyState;
}

export type MixedEntry = PlainEntry | PositionEntry;

// Field-name triples the curated Log Abstract schema models as 3 separate
// scalar fields (degree/minutes/hemisphere) that read far better as one
// compound entry field ("49° 12.345' N") than three side-by-side boxes.
const LAT_TRIPLE = ["Latitude_Degree", "Latitude_Minutes", "Latitude_North_South"] as const;
const LON_TRIPLE = ["Longitude_Degree", "Longitude_Minutes", "Longitude_East_West"] as const;

// Date/time field pairs that are stored (and, per the 2026-07-13 manual-
// test feedback, now also rendered) as two separate scalar fields — Log
// Abstract's Date_UTC/Time_UTC (the only header-level split pair once
// Date_Local/Time_Local are hidden per the UTC-only rule) plus Bunker/
// EDN's own delivery-date/time pairs (Phase 6). Not a UI-collapsing list
// (see collapseCompounds's own doc comment on why the earlier one-combined-
// control treatment was reverted): this is purely so
// reportPersistence.ts's computeEventTime can derive a report's event
// time from whichever pair the loaded schema actually has, instead of
// assuming Log Abstract's own field names.
export const DATE_TIME_PAIRS: ReadonlyArray<readonly [string, string]> = [
  ["Date_UTC", "Time_UTC"],
  ["Bunker_Delivery_Date", "Bunker_Delivery_Time"],
  ["Electricity_Delivery_Date", "Electricity_Delivery_Time"],
];

// Collapses known multi-field compounds into single synthetic entries,
// preserving original field order otherwise. Only collapses when every
// part of the compound is actually present in `entries` (i.e. survived
// field-policy visibility filtering) — a partially-hidden compound falls
// back to rendering its remaining parts as plain fields rather than
// silently dropping them.
//
// Lat/long triples are the only compound left here. Date/time pairs
// (DATE_TIME_PAIRS above) were collapsed into one combined control the
// same way until 2026-07-13 manual-test feedback: the curated JSON schema
// genuinely stores Date_UTC/Time_UTC (etc.) as two separate fields, and
// combining them in the UI only bought a code-based date<->time mapping
// that has to be kept in sync with the schema by hand on every future
// revision, for no real UX gain the lat/long triple doesn't already
// deliver more simply. Reverted; each half now renders as its own plain
// field via the normal date/time-typed FieldRow branch.
export function collapseCompounds(entries: FieldEntry[]): MixedEntry[] {
  const byName = new Map(entries.map((e) => [e.field.name, e]));
  const consumed = new Set<string>();
  const result: MixedEntry[] = [];

  for (const entry of entries) {
    if (consumed.has(entry.field.name)) continue;

    if (entry.field.name === LAT_TRIPLE[0] || entry.field.name === LON_TRIPLE[0]) {
      const triple = entry.field.name === LAT_TRIPLE[0] ? LAT_TRIPLE : LON_TRIPLE;
      const parts = triple.map((n) => byName.get(n));
      if (parts.every((p): p is FieldEntry => !!p)) {
        parts.forEach((p) => consumed.add(p!.field.name));
        result.push({
          kind: "position",
          axis: triple === LAT_TRIPLE ? "lat" : "lon",
          label: triple === LAT_TRIPLE ? "Latitude" : "Longitude",
          degree: parts[0]!.field,
          minutes: parts[1]!.field,
          hemisphere: parts[2]!.field,
          state: parts[0]!.state,
        });
        continue;
      }
    }

    result.push({ kind: "field", field: entry.field, state: entry.state });
  }
  return result;
}

export interface RenderGroup {
  label: string | null;
  rows: MixedEntry[];
}

// Groups plain fields by shared name-token runs exactly as groupFields does
// (delegated to it, unchanged), while compound entries are never grouped
// under a name-token header — the compound control's own label already
// says "Latitude"/"Longitude"/etc., so a group heading above it would be
// redundant. A compound also flushes whatever plain-field run preceded it,
// so grouping still only spans genuinely adjacent plain fields.
export function buildRenderGroups(mixed: MixedEntry[]): RenderGroup[] {
  const output: RenderGroup[] = [];
  let buffer: PlainEntry[] = [];

  function flush() {
    if (buffer.length === 0) return;
    const groups = groupFields(buffer.map((b) => b.field));
    let idx = 0;
    for (const g of groups) {
      const rows = g.fields.map(() => buffer[idx++]);
      output.push({ label: g.label, rows });
    }
    buffer = [];
  }

  for (const entry of mixed) {
    if (entry.kind === "field") {
      buffer.push(entry);
    } else {
      flush();
      output.push({ label: null, rows: [entry] });
    }
  }
  flush();
  return output;
}
