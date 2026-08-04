// SPDX-License-Identifier: AGPL-3.0-only

import type { CSSProperties } from "react";
import { TextField } from "../../design/components/forms/TextField.jsx";
import { DateTimeField } from "../../design/components/forms/DateTimeField.jsx";
import { Textarea } from "../../design/components/forms/Textarea.jsx";
import { BooleanField } from "../../design/components/forms/BooleanField.jsx";
import { Select } from "../../design/components/forms/Select.jsx";
import { Tooltip } from "../../design/components/feedback/Tooltip.jsx";
import type { Finding, FieldPolicyState, PrefillEntry, SchemaField } from "../../api/client";
import { fieldInfoTip, isGhgRelevant, isRequired } from "./fieldPolicy";
import { fieldSpan, isMultiline, nativeInputType, type FieldSpan } from "./fieldLayout";
import { HighlightableField, type PolicyOutline } from "./HighlightableField";

export function policyOutlineFor(state: FieldPolicyState, relevance: string): PolicyOutline {
  const mandatory = isRequired(state);
  const ghg = isGhgRelevant(relevance);
  if (mandatory && ghg) return "both";
  if (mandatory) return "mandatory";
  if (ghg) return "ghgRelevant";
  return null;
}

// Live plausibility findings (pkg/validation, re-run debounced against the
// unsaved form state — see ReportForm's validateSeq effect) are looked up
// per field here rather than re-derived: the engine is the single source of
// truth for what counts as implausible (CLAUDE.md's validation rule), the
// UI's only job is surfacing whichever finding(s) already named this field.
// A compound row (position triple, date/time pair) passes every name it
// covers, so a finding tied to any part of the compound still surfaces on
// the one control the officer actually sees. Error outranks warning
// outranks info when more than one finding names the same field.
const SEVERITY_RANK: Record<Finding["severity"], number> = { error: 3, warning: 2, info: 1 };

export function pickFinding(findings: Finding[], fieldNames: string[]): Finding | undefined {
  let best: Finding | undefined;
  for (const f of findings) {
    if (!f.field || !fieldNames.includes(f.field)) continue;
    if (!best || SEVERITY_RANK[f.severity] > SEVERITY_RANK[best.severity]) best = f;
  }
  return best;
}

export type FieldValue = string | boolean;

interface FieldRowProps {
  field: SchemaField;
  state: FieldPolicyState;
  value: FieldValue | undefined;
  prefill?: PrefillEntry;
  overridden: boolean;
  /** Resolved codes for field.enumRef, when schema.ResolveEnum knows how to read it (see pkg/schema/enums.go). */
  enumValues?: string[];
  onChange: (name: string, value: FieldValue, markOverridden: boolean) => void;
  onRestoreComputed: (name: string, computedValue: FieldValue) => void;
  highlightField?: string | null;
  highlightNonce?: number | null;
  remarkedFieldNames?: Record<string, string>;
  findings?: Finding[];
}

// Event is set once, from Home's event picker, before the report (and
// therefore this form) ever exists — office may key mandatory-field policy
// off the event type, so letting the officer silently change it here would
// let entered data drift out of sync with the policy it was collected
// under. Wrong event picked = abandon this draft and start a fresh one from
// Home, not edit it in place.
const LOCKED_FIELDS = new Set<string>(["Event"]);

const GRID_COLUMN: Record<FieldSpan, string> = {
  normal: "span 1",
  wide: "span 2",
  full: "1 / -1",
};

const DATE_TIME_MODE = {
  date: "date",
  time: "time",
  "datetime-local": "datetime",
} as const;

const chipStyle = (amber: boolean): CSSProperties => ({
  display: "inline-flex",
  alignItems: "center",
  height: 16,
  padding: "0 6px",
  borderRadius: "var(--shape-full)",
  fontSize: 9,
  fontWeight: 600,
  letterSpacing: "0.02em",
  textTransform: "uppercase",
  cursor: "help",
  // Matches the rest of the codebase's "color, not shape/background,
  // carries the overridden state" convention (see the computed-field doc
  // comment below).
  background: "var(--color-surface-container-highest)",
  color: amber ? "var(--color-status-caution)" : "var(--color-on-surface-variant)",
});

// Architecture 6.4's four prefill classes, rendered:
// - carryForward: prefilled normally, no special marking beyond the
//   standard edited state (design handoff A5).
// - computed: a "calculated" chip in the label row (tap/hover shows the
//   formula) plus a restore control in the suffix slot — always rendered
//   once a field is computed, so overriding it never costs the officer
//   the ability to get back to the calculated answer; only its color
//   flags "this no longer matches its calculated value" (amber), not its
//   shape (an earlier version swapped icons on override and silently
//   removed the only way back — see PROJECT.md decisions log,
//   2026-07-05). Reinstated as a wireframe-matching chip on 2026-07-12
//   (also PROJECT.md, "Vessel UI rework" section) after having been
//   simplified to a lone icon in that same 2026-07-05 pass — a recorded
//   reversal, not new scope.
// - ghost: previous report's value shown as the input's own placeholder
//   when the field is empty, never auto-filled.
// - none / unset: no treatment.
export function FieldRow({ field, state, value, prefill, overridden, enumValues, onChange, onRestoreComputed, highlightField = null, highlightNonce = null, remarkedFieldNames, findings = [] }: FieldRowProps) {
  const label = field.label;
  const required = isRequired(state);
  const infoTip = fieldInfoTip(field);
  const finding = pickFinding(findings, [field.name]);
  const hasError = finding?.severity === "error";
  const hasWarning = finding?.severity === "warning";
  const supportingText = finding?.message;
  const ghostPlaceholder = prefill?.class === "ghost" && (value === undefined || value === "")
    ? `— last: ${prefill.value}${field.unit ? ` ${field.unit}` : ""}`
    : undefined;
  const gridColumn = GRID_COLUMN[fieldSpan(field)];
  const isComputed = prefill?.class === "computed";
  // Always on — a field's mandatory/GHG-relevant classification is static
  // schema/policy metadata, not a validation result, so there's nothing to
  // wait for a Check to compute (2026-07-13 manual-test feedback: this
  // border was wrongly gated the same way the red/amber error/warning
  // findings are, which really do need to wait for Check). Downstream
  // usages of `outline` (both the primitive's own border and
  // HighlightableField's "also GHG-relevant" corner dot) all read this one
  // value rather than each needing their own check.
  const outline = policyOutlineFor(state, field.relevance);

  if (LOCKED_FIELDS.has(field.name)) {
    return (
      <HighlightableField fieldNames={[field.name]} highlightField={highlightField} highlightNonce={highlightNonce} remarkedFieldNames={remarkedFieldNames} outline={outline} style={{ gridColumn }}>
        <TextField
          label={label}
          required={required}
          value={typeof value === "string" ? value : ""}
          onChange={() => undefined}
          disabled
          suffix={
            <Tooltip label="Set when this report was created. Start a new report from Home to change it." maxWidth={220}>
              <span tabIndex={-1} className="material-symbols-rounded" style={{ fontSize: 16, color: "var(--color-on-surface-variant)", cursor: "help" }}>lock</span>
            </Tooltip>
          }
          infoTip={infoTip}
          policyOutline={outline}
          style={{ width: "100%" }}
        />
      </HighlightableField>
    );
  }

  if (field.type === "enum" && enumValues && enumValues.length > 0) {
    return (
      <HighlightableField fieldNames={[field.name]} highlightField={highlightField} highlightNonce={highlightNonce} remarkedFieldNames={remarkedFieldNames} outline={outline} style={{ gridColumn }}>
        <Select
          label={label}
          required={required}
          value={typeof value === "string" ? value : ""}
          options={enumValues}
          onChange={(v: string) => onChange(field.name, v, false)}
          error={hasError}
          warning={hasWarning}
          supportingText={supportingText}
          infoTip={infoTip}
          policyOutline={outline}
          style={{ width: "100%" }}
        />
      </HighlightableField>
    );
  }

  if (field.type === "boolean") {
    return (
      <HighlightableField
        fieldNames={[field.name]}
        highlightField={highlightField}
        highlightNonce={highlightNonce}
        remarkedFieldNames={remarkedFieldNames}
        outline={outline}
        style={{ gridColumn }}
      >
        <BooleanField
          label={label}
          required={required}
          infoTip={infoTip}
          checked={value === true}
          onChange={(checked: boolean) => onChange(field.name, checked, false)}
          error={hasError}
          warning={hasWarning}
          supportingText={supportingText}
          policyOutline={outline}
          style={{ width: "100%" }}
        />
      </HighlightableField>
    );
  }

  if (isMultiline(field)) {
    return (
      <HighlightableField fieldNames={[field.name]} highlightField={highlightField} highlightNonce={highlightNonce} remarkedFieldNames={remarkedFieldNames} outline={outline} style={{ gridColumn }}>
        <Textarea
          label={label}
          required={required}
          value={typeof value === "string" ? value : ""}
          onChange={(v) => onChange(field.name, v, false)}
          placeholder={ghostPlaceholder}
          supportingText={supportingText}
          error={hasError}
          warning={hasWarning}
          infoTip={infoTip}
          maxLength={field.maxLength ?? null}
          rows={4}
          policyOutline={outline}
          style={{ width: "100%" }}
        />
      </HighlightableField>
    );
  }

  const nativeType = nativeInputType(field);
  if (nativeType === "date" || nativeType === "time" || nativeType === "datetime-local") {
    return (
      <HighlightableField fieldNames={[field.name]} highlightField={highlightField} highlightNonce={highlightNonce} remarkedFieldNames={remarkedFieldNames} outline={outline} style={{ gridColumn }}>
        <DateTimeField
          mode={DATE_TIME_MODE[nativeType]}
          label={label}
          required={required}
          value={typeof value === "string" ? value : ""}
          onChange={(v) => onChange(field.name, v, false)}
          error={hasError}
          warning={hasWarning}
          supportingText={supportingText}
          infoTip={infoTip}
          policyOutline={outline}
          style={{ width: "100%" }}
        />
      </HighlightableField>
    );
  }

  const hasComputedValue = isComputed && prefill?.value !== undefined && prefill.value !== null;
  const computedTooltip = isComputed
    ? hasComputedValue
      ? `${prefill?.formula ?? "Calculated automatically"}${overridden ? " — click to overwrite with this calculated value" : ""}`
      : "Enter the fields this is calculated from to see a value here"
    : undefined;

  const computedChip = isComputed ? (
    <Tooltip label={computedTooltip ?? ""} maxWidth={220}>
      <span style={chipStyle(overridden)}>calculated</span>
    </Tooltip>
  ) : null;

  const restoreSuffix = isComputed ? (
    <button
      type="button"
      aria-label="Restore calculated value"
      title={computedTooltip}
      disabled={!hasComputedValue}
      onClick={hasComputedValue ? () => onRestoreComputed(field.name, String(prefill!.value)) : undefined}
      className="material-symbols-rounded"
      style={{
        fontSize: 16,
        color: overridden ? "var(--color-status-caution)" : "var(--color-on-surface-variant)",
        cursor: hasComputedValue ? "pointer" : "default",
        opacity: hasComputedValue ? 1 : 0.4,
        background: "none",
        border: "none",
        padding: 2,
        display: "inline-flex",
      }}
    >
      restart_alt
    </button>
  ) : field.unit ? (
    <span className="md-label-medium" style={{ color: "var(--color-on-surface-variant)" }}>{field.unit}</span>
  ) : null;

  return (
    <HighlightableField fieldNames={[field.name]} highlightField={highlightField} highlightNonce={highlightNonce} remarkedFieldNames={remarkedFieldNames} outline={outline} style={{ gridColumn }}>
      <TextField
        label={label}
        required={required}
        value={typeof value === "string" ? value : ""}
        onChange={(v) => onChange(field.name, v, isComputed)}
        type={nativeType}
        placeholder={ghostPlaceholder}
        chip={computedChip}
        suffix={restoreSuffix}
        filledTint={isComputed}
        supportingText={supportingText}
        error={hasError}
        warning={hasWarning}
        infoTip={infoTip}
        policyOutline={outline}
        style={{ width: "100%" }}
      />
    </HighlightableField>
  );
}
