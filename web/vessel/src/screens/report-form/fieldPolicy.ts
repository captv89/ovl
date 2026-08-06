// SPDX-License-Identifier: AGPL-3.0-only

import type { FieldPolicyState, SchemaField } from "../../api/client";

// CLAUDE.md is explicit: "All timestamps stored and exchanged in UTC. No
// local time anywhere in data entry." This is a standing project rule, not
// something the backend's placeholder field-policy config (a Phase 3
// stand-in) should be trusted to enforce — so it's a hard
// frontend override, ahead of schema-mandatory, rather than a policy entry
// that could be silently left out of some future config bundle.
function isLocalTimeField(field: SchemaField): boolean {
  return (field.type === "date" || field.type === "time") && field.name.endsWith("_Local");
}

// Mirrors pkg/validation/regulatory.go's profileRelevance switch exactly —
// the same fixed, enumerated vocabulary (verified by inspection across all
// 409 Log Abstract fields, not fuzzy-matched), so an OVD revision
// introducing new relevance wording fails loudly (falls through to "not
// GHG-relevant") instead of being silently misclassified. Only the
// case-vs-default split matters here, not which specific profile(s) a
// relevance string maps to — that per-profile detail stays Go-side
// (regulatory readiness), this is just "is this relevant at all."
//
// DRIFT HAZARD (codebase audit 2026-07-22 §6): this 9-string set is
// hand-copied in THREE places — profileRelevance (Go), this file, and
// web/office/src/screens/configuration/fieldPolicyLogic.ts. Any change must
// touch all three; the Go side is guarded by
// TestProfileRelevance_RecognizedStringsSnapshot, which fails loudly and
// names these mirrors. The "verfication" typo below is copied faithfully
// from the source OVD xlsx.
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

// Exported for FieldRow's mandatory/GHG-relevant outline coding — this is a
// fixed fact about the field's own curation, independent of whatever
// visibility state office has chosen for it (a field office demoted to
// optional despite being GHG-relevant still shows the GHG marker, so the
// officer isn't misled about what actually matters for verification).
export function isGhgRelevant(relevance: string): boolean {
  return GHG_RELEVANT_STRINGS.has(relevance.trim());
}

// Mirrors pkg/validation.FieldEvents.AppliesTo exactly. A field absent from
// the map — or one whose list is empty or contains the "*" wildcard — applies
// to every voyage event, and an empty eventType skips the gate entirely (a
// schema with no event concept of its own must never have its fields gated
// away). DRIFT HAZARD: this rule is mirrored in pkg/validation/policy.go and
// web/office's fieldPolicyLogic.ts; the Go side is authoritative.
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

// Mirrors pkg/validation.FieldPolicy.StateForEvent exactly: schema-mandatory
// always wins regardless of what the policy says (and is never event-gated —
// architecture 6.1 calls schema mandatoriness immutable); a field the company
// scoped to specific voyage events is hidden on every other event; an
// unlisted field defaults to "recommended" (shown, not collapsed) when its
// curated relevance ties it to a real regulatory profile, and to "optional"
// (collapsed) otherwise.
//
// events/eventType are optional so callers with no event context behave
// exactly as they did before per-event policy existed.
export function effectiveState(
  field: SchemaField,
  policy: Record<string, FieldPolicyState>,
  events?: Record<string, string[]>,
  eventType?: string,
): FieldPolicyState {
  if (isLocalTimeField(field)) return "hidden";
  if (field.schemaMandatory) return "schemaMandatory";
  if (!appliesToEvent(field.name, events, eventType)) return "hidden";
  const explicit = policy[field.name];
  if (explicit) return explicit;
  return isGhgRelevant(field.relevance) ? "recommended" : "optional";
}

export function isRequired(state: FieldPolicyState): boolean {
  return state === "schemaMandatory" || state === "companyMandatory";
}

// The wireframes' field anatomy has exactly one info affordance (the
// always-visible ⓘ) — folds a field's plain-language description and its
// (separately curated, sometimes several-sentence) conditional-mandatory
// note into that single tooltip's content instead of a second icon, so
// there's never a competing second "what does this mean" control on the
// same field.
export function fieldInfoTip(field: { description?: string; mandatoryNote?: string }): string | undefined {
  if (field.description && field.mandatoryNote) return `${field.description}\n\n${field.mandatoryNote}`;
  return field.description || field.mandatoryNote || undefined;
}
