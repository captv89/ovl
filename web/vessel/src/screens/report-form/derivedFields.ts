// SPDX-License-Identifier: AGPL-3.0-only

import type { FieldValue } from "./FieldRow";

// Vessel-relative compass sector (1-8), confirmed against the actual DNV
// reference diagram (not guessed — an earlier pass in this codebase
// explicitly declined to invent this mapping since the xlsx only says
// "in sectors of 1 to 8 as shown in graph" without providing that graph;
// the user supplied the real graph on 2026-07-05). Sector 1 is centered
// on 0° (dead ahead/bow) and sectors run clockwise in 45° steps, each
// spanning ±22.5° of its center — sector boundaries sit at 22.5, 67.5,
// 112.5, ... matching the reference exactly.
export function degreeToCompassSector(deg: number): number {
  const norm = ((deg % 360) + 360) % 360;
  return (Math.floor((norm + 22.5) / 45) % 8) + 1;
}

// Standard WMO Beaufort scale, upper bound in knots for force 0-11
// (anything at or above the last bound is force 12) — a universal
// meteorological conversion, not an OVD-specific ambiguity.
const BEAUFORT_KN_UPPER = [1, 4, 7, 11, 17, 22, 28, 34, 41, 48, 56, 64];

export function knotsToBeaufort(kn: number): number {
  for (let force = 0; force < BEAUFORT_KN_UPPER.length; force++) {
    if (kn < BEAUFORT_KN_UPPER[force]) return force;
  }
  return 12;
}

interface DerivedFieldSpec {
  source: string;
  formulaLabel: (sourceValue: number) => string;
  compute: (sourceValue: number) => number;
}

// Fields whose value is a pure function of exactly one sibling field
// already present on the same form — the one case Phase 2 can compute
// for real today (no report history or backend needed), unlike the
// still-static ROB-style "computed" demo entries in
// vessel/httpapi/schemas.go's fieldConfigFor. Wind's pair was the user's
// explicit request (enter relative wind course + speed; derive sector +
// Beaufort force); Sea state/Swell/Current's compass-sector fields get
// identical treatment since they share the exact same *_Dir/*_Dir_Degree
// relationship and DNV sector convention confirmed above.
export const DERIVED_FIELDS: Record<string, DerivedFieldSpec> = {
  Wind_Dir: {
    source: "Wind_Dir_Degree",
    formulaLabel: (v) => `Sector for relative wind direction ${Math.round(v)}°`,
    compute: degreeToCompassSector,
  },
  Wind_Force_Bft: {
    source: "Wind_Force_Kn",
    formulaLabel: (v) => `Beaufort force for relative wind speed ${v} kn`,
    compute: knotsToBeaufort,
  },
  Sea_state_Dir: {
    source: "Sea_state_Dir_Degree",
    formulaLabel: (v) => `Sector for sea state direction ${Math.round(v)}°`,
    compute: degreeToCompassSector,
  },
  Swell_Dir: {
    source: "Swell_Dir_Degree",
    formulaLabel: (v) => `Sector for swell direction ${Math.round(v)}°`,
    compute: degreeToCompassSector,
  },
  Current_Dir: {
    source: "Current_Dir_Degree",
    formulaLabel: (v) => `Sector for current direction ${Math.round(v)}°`,
    compute: degreeToCompassSector,
  },
};

function asNumber(v: FieldValue | undefined): number | null {
  if (typeof v !== "string" || v === "") return null;
  const n = Number(v);
  return Number.isFinite(n) ? n : null;
}

export interface DerivedResult {
  value: string;
  formula: string;
}

// Recomputes every live-derivable field from its source field's current
// value. A derived field with no valid (or not-yet-entered) source is
// left out entirely — no phantom zero — same principle WeatherVane
// already uses for its own "no data" state.
export function computeDerivedValues(values: Record<string, FieldValue>): Record<string, DerivedResult> {
  const out: Record<string, DerivedResult> = {};
  for (const [name, spec] of Object.entries(DERIVED_FIELDS)) {
    const sourceValue = asNumber(values[spec.source]);
    if (sourceValue === null) continue;
    out[name] = { value: String(spec.compute(sourceValue)), formula: spec.formulaLabel(sourceValue) };
  }
  return out;
}

// Real live recompute for the ROB continuity class (architecture 8.3:
// ROB(n) = ROB(n-1) - consumption(n) + bunkered(n), bunkered omitted here
// since no schema wired to this app tracks bunkered amounts yet — see
// pkg/validation.Config.BunkeredAmounts' own doc comment) and for
// Time_Since_Previous_Report — both used to be fixed demo constants
// (vessel/httpapi/schemas.go's fieldConfigFor), which fed the exact same
// number into every report regardless of history and made cascade
// revalidation (runCascade) cascade-invalidate reports the moment real
// entered consumption/elapsed-time diverged from that fake baseline
// (18.07.26 manual-test items 3/6/8/10). `robBases`/`lastReportEventTime`
// come from the schema config response's own real computed prefill
// (vessel/httpapi/schemas.go's handleGetSchema) — this only recomputes
// the *display* live as sibling fields change, exactly like
// DERIVED_FIELDS already does for Wind_Dir etc.; pkg/validation's
// EvaluateROBContinuity/EvaluateTimeChain remain the authoritative check.
export interface RobSeriesSpec {
  robField: string;
  consumptionFields: string[];
}

// 18.07.26 manual-test triage ("why limit the auto calculation to just
// these 2 fields... peripheral vision"): a single ROB tank is drawn down
// by more than one consumer on a real vessel (Main Engine and Auxiliary
// Engine both burn HFO from the same tank, plus Boiler/Inert Gas
// Generator/Cargo Heating for some fuel types) — summing every
// consumptionFields entry mirrors pkg/validation.EvaluateROBContinuity's
// own summation exactly, so the live pre-check and the authoritative
// server-side check never disagree about which fields count.
export function computeRobDerivedValues(
  values: Record<string, FieldValue>,
  robSeriesList: RobSeriesSpec[],
  robBases: Record<string, number>,
): Record<string, DerivedResult> {
  const out: Record<string, DerivedResult> = {};
  for (const series of robSeriesList) {
    const base = robBases[series.robField];
    if (base === undefined) continue; // no prior submitted report to continue from
    const consumption = series.consumptionFields.reduce((sum, f) => sum + (asNumber(values[f]) ?? 0), 0);
    const expected = base - consumption;
    out[series.robField] = {
      value: String(Math.round(expected * 100) / 100),
      formula: `${base} previous − ${Math.round(consumption * 100) / 100} consumed`,
    };
  }
  return out;
}

export function computeTimeSincePreviousReport(
  eventTimeIso: string,
  lastReportEventTimeIso: string | undefined,
): DerivedResult | undefined {
  if (!lastReportEventTimeIso) return undefined;
  const eventTime = new Date(eventTimeIso).getTime();
  const lastReportEventTime = new Date(lastReportEventTimeIso).getTime();
  if (!Number.isFinite(eventTime) || !Number.isFinite(lastReportEventTime)) return undefined;
  const hours = (eventTime - lastReportEventTime) / 3_600_000;
  return {
    value: String(Math.round(hours * 100) / 100),
    formula: `Elapsed since the previous report at ${new Date(lastReportEventTime).toISOString().slice(0, 16).replace("T", " ")} UTC`,
  };
}
