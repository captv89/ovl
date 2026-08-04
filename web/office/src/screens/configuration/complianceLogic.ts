// SPDX-License-Identifier: AGPL-3.0-only

import type { Scope, VesselView } from "../../api/client";

// Labels for pkg/validation.RegulatoryProfile's four values (design
// handoff B7: "toggle cards for MRV/ETS/FuelEU, DCS, CII corrections,
// voyage-level verification").
export const PROFILE_LABELS: Record<string, string> = {
  mrv: "MRV / ETS / FuelEU Maritime",
  dcs: "DCS",
  cii: "CII corrections",
  voyageVerification: "Voyage-level verification",
};

export const ALL_PROFILES = ["mrv", "dcs", "cii", "voyageVerification"];

// Human labels for office/compliance.OverridableRuleIDs/HardRuleIDs —
// the frontend only ever displays these, it never decides what they
// mean (the rule engine in pkg/validation is the single source of
// truth, per CLAUDE.md), so this map exists purely for readability.
export const RULE_LABELS: Record<string, string> = {
  "plausibility.timeBucketSum": "Time bucket sum",
  "plausibility.impliedSpeed": "Implied speed",
  "plausibility.noDistanceStationary": "No distance while stationary",
  "plausibility.positionRequired": "Position required",
  "plausibility.positionConsistency": "Position consistency",
  "continuity.timeChain": "Time chain continuity",
  "continuity.robContinuity": "ROB continuity",
  "continuity.eventOrdering": "Event ordering",
  "continuity.timestampUniqueness": "Timestamp uniqueness",
  "plausibility.consumptionSchemeExclusivity": "Consumption scheme exclusivity",
};

export function ruleLabel(ruleID: string): string {
  return RULE_LABELS[ruleID] ?? ruleID;
}

export function scopeLabel(scope: Scope, vessels: VesselView[]): string {
  if (scope.type === "fleet") return "Fleet-wide";
  if (scope.type === "group") return `Group: ${scope.key}`;
  const vessel = vessels.find((v) => v.id === scope.key);
  return vessel ? `Vessel: ${vessel.name}` : `Vessel: ${scope.key}`;
}

export function scopeKey(scope: Scope): string {
  return `${scope.type}:${scope.key ?? ""}`;
}

export function scopesEqual(a: Scope, b: Scope): boolean {
  return a.type === b.type && (a.key ?? "") === (b.key ?? "");
}

// Icon + container-color pairing for a scope kind, shared by every
// "current assignments" list in Configuration (regulatory profiles,
// bundles) so a scope's kind reads at a glance without spelling out
// "Fleet-wide"/"Group"/"Vessel" every time.
export const SCOPE_ICONS: Record<Scope["type"], { icon: string; bg: string; fg: string }> = {
  fleet: { icon: "public", bg: "var(--color-tertiary-container)", fg: "var(--color-on-tertiary-container)" },
  group: { icon: "workspaces", bg: "var(--color-secondary-container)", fg: "var(--color-on-secondary-container)" },
  vessel: { icon: "directions_boat", bg: "var(--color-primary-container)", fg: "var(--color-on-primary-container)" },
};
