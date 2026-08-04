// SPDX-License-Identifier: AGPL-3.0-only

import { useEffect, useState, type CSSProperties } from "react";
import { Button } from "../../design/components/core/Button.jsx";
import { WeatherVane } from "../../design/components/maritime/WeatherVane.jsx";
import type { Finding, FieldPolicyState, PrefillEntry, SchemaField } from "../../api/client";
import { effectiveState } from "./fieldPolicy";
import { collapseCompounds, buildRenderGroups, type FieldEntry } from "./fieldLayout";
import { FieldRow, type FieldValue } from "./FieldRow";
import { PositionRow } from "./PositionRow";
import { AttachmentsSection } from "./AttachmentsSection";

interface SectionPanelProps {
  sectionKey: string;
  fields: SchemaField[];
  policy: Record<string, FieldPolicyState>;
  /** Per-field voyage-event narrowing from the applied config bundle; a field absent from it applies to every event. */
  events?: Record<string, string[]>;
  /** The event type this report was created under. Fields the company scoped away from it never render. */
  eventType?: string;
  values: Record<string, FieldValue>;
  prefill: Record<string, PrefillEntry>;
  overridden: Set<string>;
  enumOptions: Record<string, string[]>;
  onChange: (name: string, value: FieldValue, markOverridden: boolean) => void;
  onRestoreComputed: (name: string, computedValue: FieldValue) => void;
  highlightField?: string | null;
  highlightNonce?: number | null;
  /** fieldName -> reviewer comment for unresolved remarks (Phase 5 S5/open question 4) — see HighlightableField's own doc comment. */
  remarkedFieldNames?: Record<string, string>;
  /** Live plausibility findings (pkg/validation, debounced against unsaved edits — see ReportForm's validateSeq effect), looked up per field so each row can show its own inline error/warning instead of only surfacing at Health Check. */
  findings?: Finding[];
  /** Bunker/EDN's "attachments" pseudo-section (Phase 6) needs the report id to list/upload against, and whether the report is locked to gate the add/remove controls. Unused by every other section. */
  reportId?: string | null;
  locked?: boolean;
}

const GRID_TEMPLATE = "repeat(auto-fill, minmax(260px, 1fr))";

// A responsive 2-3 column grid (auto-fill lets the browser settle on
// however many 260px+ tracks the section pane actually has room for)
// rather than one full-width column — most curated OVD fields are short
// numeric readings, so a single column wasted the vast majority of the
// row's width and forced endless scrolling on a 400+ field schema.
//
// Each render group gets its OWN nested grid rather than all groups
// sharing one flat grid track list. A single shared grid let the browser's
// own auto-placement pack a later group's field into an earlier group's
// leftover column (e.g. a lone singleton field snapping into the empty
// cell beside a 2-column-wide compound like PositionField) — reads as an
// orphaned box floating next to an unrelated field, not as its own row.
// Isolating each group in its own grid means a group's fields only ever
// share layout with their own group.
function FieldGrid({ entries, values, prefill, overridden, enumOptions, onChange, onRestoreComputed, highlightField = null, highlightNonce = null, remarkedFieldNames, findings = [] }: {
  entries: FieldEntry[];
  values: Record<string, FieldValue>;
  prefill: Record<string, PrefillEntry>;
  overridden: Set<string>;
  enumOptions: Record<string, string[]>;
  onChange: SectionPanelProps["onChange"];
  onRestoreComputed: SectionPanelProps["onRestoreComputed"];
  highlightField?: string | null;
  highlightNonce?: number | null;
  remarkedFieldNames?: Record<string, string>;
  findings?: Finding[];
}) {
  const renderGroups = buildRenderGroups(collapseCompounds(entries));

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-5)" }}>
      {renderGroups.map((group, gi) => (
        <div key={gi}>
          {group.label ? (
            <div
              className="md-label-large"
              style={{
                color: "var(--color-on-surface-variant)",
                textTransform: "uppercase",
                letterSpacing: "0.04em",
                marginBottom: "var(--space-3)",
                paddingTop: gi > 0 ? "var(--space-2)" : 0,
                borderTop: gi > 0 ? "1px solid var(--color-outline-variant)" : "none",
              }}
            >
              {group.label}
            </div>
          ) : null}
          <div style={{ display: "grid", gridTemplateColumns: GRID_TEMPLATE, gap: "var(--space-5)", alignItems: "start" }}>
            {group.rows.map((row) => {
              if (row.kind === "position") {
                return (
                  <PositionRow
                    key={row.degree.name}
                    axis={row.axis}
                    label={row.label}
                    degreeField={row.degree}
                    minutesField={row.minutes}
                    hemisphereField={row.hemisphere}
                    state={row.state}
                    values={values}
                    onChange={onChange}
                    highlightField={highlightField}
                    highlightNonce={highlightNonce}
                    remarkedFieldNames={remarkedFieldNames}
                    findings={findings}
                  />
                );
              }
              return (
                <FieldRow
                  key={row.field.name}
                  field={row.field}
                  state={row.state}
                  value={values[row.field.name]}
                  prefill={prefill[row.field.name]}
                  overridden={overridden.has(row.field.name)}
                  enumValues={row.field.enumRef ? enumOptions[row.field.enumRef] : undefined}
                  onChange={onChange}
                  onRestoreComputed={onRestoreComputed}
                  highlightField={highlightField}
                  highlightNonce={highlightNonce}
                  remarkedFieldNames={remarkedFieldNames}
                  findings={findings}
                />
              );
            })}
          </div>
        </div>
      ))}
    </div>
  );
}

function numericValue(values: Record<string, FieldValue>, name: string): string | undefined {
  const v = values[name];
  return typeof v === "string" ? v : undefined;
}

// Explains the field outline coding (see HighlightableField's `outline`
// prop): a small, always-visible key rather than a one-time dismissible
// tip, since a new officer could open any section first. Both this legend
// and the borders it explains are unconditional now (2026-07-13 manual-
// test feedback, two rounds: a field's mandatory/GHG-relevant
// classification is static schema/policy metadata, not a validation
// result — there's nothing to wait for a Check to compute, unlike the
// red/amber error/warning findings, which genuinely do wait for one).
function FieldPolicyLegend() {
  const itemStyle: CSSProperties = { display: "flex", alignItems: "center", gap: 6 };
  const swatchBase: CSSProperties = { width: 20, height: 12, borderRadius: 3, flexShrink: 0 };
  return (
    <div
      className="md-label-small"
      style={{ display: "flex", flexWrap: "wrap", gap: "var(--space-5)", color: "var(--color-on-surface-variant)" }}
    >
      <span style={itemStyle}>
        <span style={{ color: "var(--color-error)", fontWeight: 600 }}>*</span> Required
      </span>
      <span style={itemStyle}>
        <span style={{ ...swatchBase, border: "1.5px solid var(--color-secondary)" }} /> Mandatory
      </span>
      <span style={itemStyle}>
        <span style={{ ...swatchBase, border: "1.5px dashed var(--color-secondary)" }} /> Recommended
      </span>
      <span style={itemStyle}>
        <span style={{ ...swatchBase, border: "1.5px solid var(--color-secondary)", position: "relative" }}>
          <span style={{ position: "absolute", top: -4, right: -4, width: 6, height: 6, borderRadius: "var(--shape-full)", background: "var(--color-secondary)" }} />
        </span>
        Both
      </span>
    </div>
  );
}

// Architecture 9.4/design handoff A5: hidden fields never render;
// optional fields collapse behind a quiet "+ Add N optional fields"
// affordance instead of always showing all of them at once.
export function SectionPanel({ sectionKey, fields, policy, events, eventType, values, prefill, overridden, enumOptions, onChange, onRestoreComputed, highlightField = null, highlightNonce = null, remarkedFieldNames, findings = [], reportId = null, locked = false }: SectionPanelProps) {
  const [expanded, setExpanded] = useState(false);

  const visible = fields
    .map((field) => ({ field, state: effectiveState(field, policy, events, eventType) }))
    .filter(({ state }) => state !== "hidden");
  const always = visible.filter(({ state }) => state !== "optional");
  const optional = visible.filter(({ state }) => state === "optional");
  const isWeatherSection = sectionKey === "weather";

  // A jump target can land inside a collapsed optional group (e.g. a soft
  // plausibility warning on an optional field) — auto-expand rather than
  // scrolling to a field that isn't actually rendered yet. Runs
  // unconditionally (rules-of-hooks) even for the Attachments pseudo-
  // section below, where it's just always a no-op (no optional fields).
  useEffect(() => {
    if (highlightField && optional.some(({ field }) => field.name === highlightField)) {
      setExpanded(true);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [highlightField, highlightNonce]);

  // The Attachments pseudo-section (Phase 6, Bunker/EDN forms) has no
  // schema fields at all — it renders entirely differently. Checked
  // after every hook above has run (rules-of-hooks), not as an early
  // return before them.
  if (sectionKey === "attachments") {
    return <AttachmentsSection reportId={reportId} locked={locked} />;
  }

  const grid = (
    <FieldGrid
      entries={always}
      values={values}
      prefill={prefill}
      overridden={overridden}
      enumOptions={enumOptions}
      onChange={onChange}
      onRestoreComputed={onRestoreComputed}
      highlightField={highlightField}
      highlightNonce={highlightNonce}
      remarkedFieldNames={remarkedFieldNames}
      findings={findings}
    />
  );

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-6)", maxWidth: 1040 }}>
      <FieldPolicyLegend />
      {isWeatherSection ? (
        <div style={{ display: "flex", gap: "var(--space-6)", alignItems: "flex-start", flexWrap: "wrap" }}>
          <div style={{ flex: 1, minWidth: 280 }}>{grid}</div>
          <div
            style={{
              flexShrink: 0,
              padding: "var(--space-5)",
              borderRadius: "var(--shape-medium)",
              border: "1px solid var(--color-outline-variant)",
              background: "var(--color-surface-container-low)",
            }}
          >
            <div className="md-label-large" style={{ color: "var(--color-on-surface-variant)", marginBottom: "var(--space-3)", textTransform: "uppercase", letterSpacing: "0.04em" }}>
              Vessel-Relative
            </div>
            <WeatherVane
              windDirDeg={numericValue(values, "Wind_Dir_Degree")}
              windForceBft={numericValue(values, "Wind_Force_Bft")}
              seaDirDeg={numericValue(values, "Sea_state_Dir_Degree")}
              seaForceDouglas={numericValue(values, "Sea_state_Force_Douglas")}
              swellDirDeg={numericValue(values, "Swell_Dir_Degree")}
              swellForceM={numericValue(values, "Swell_Force")}
              currentDirDeg={numericValue(values, "Current_Dir_Degree")}
              currentSpeedKn={numericValue(values, "Current_Speed")}
              size={200}
            />
          </div>
        </div>
      ) : (
        grid
      )}

      {optional.length > 0 ? (
        <div>
          <Button variant="text" icon={expanded ? "expand_less" : "add"} onClick={() => setExpanded((e) => !e)}>
            {expanded ? "Hide optional fields" : `Add ${optional.length} optional field${optional.length === 1 ? "" : "s"}`}
          </Button>
          {expanded ? (
            <div style={{ marginTop: "var(--space-5)" }}>
              <FieldGrid
                entries={optional}
                values={values}
                prefill={prefill}
                overridden={overridden}
                enumOptions={enumOptions}
                onChange={onChange}
                onRestoreComputed={onRestoreComputed}
                highlightField={highlightField}
                highlightNonce={highlightNonce}
                remarkedFieldNames={remarkedFieldNames}
                findings={findings}
              />
            </div>
          ) : null}
        </div>
      ) : null}

      {always.length === 0 && optional.length === 0 ? (
        <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
          Every field in this section is hidden by the current field policy.
        </div>
      ) : null}
    </div>
  );
}
