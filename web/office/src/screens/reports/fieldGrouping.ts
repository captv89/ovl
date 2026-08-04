// SPDX-License-Identifier: AGPL-3.0-only

import type { SchemaField } from "../../api/client";

// Vendored subset of web/vessel/src/screens/report-form/fieldLayout.ts's
// groupFields (apps don't share code — same vendored-copy precedent as
// HighlightableField.tsx/ReportValuesTab). Only the grouping pass itself
// is ported, not fieldSpan/isMultiline/nativeInputType/collapseCompounds/
// buildRenderGroups — office's read-only report view has no editable
// compound-entry control to collapse the lat/long triple into, and no
// input widget sizing to compute, so those would be dead code here.
//
// 18.07.26 manual-test item 11: this view's field grid had no grouping at
// all (unlike vessel's edit-mode SectionPanel.tsx, which has always had
// it) — a section with 40+ fields read as one undifferentiated grid.

export interface FieldGroup {
  label: string | null;
  fields: SchemaField[];
}

const MIN_GROUP_SIZE = 2;

const GROUP_LABEL_OVERRIDES: Record<string, string> = {
  Entry: "Entered by",
};

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

function commonLeadingWords(a: string[], b: string[]): string[] {
  let n = 0;
  while (n < a.length && n < b.length && a[n].toLowerCase() === b[n].toLowerCase()) n++;
  return a.slice(0, n);
}

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
