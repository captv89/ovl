// SPDX-License-Identifier: AGPL-3.0-only

import { useState } from "react";
import { Button } from "../../design/components/core/Button.jsx";
import { Chip } from "../../design/components/surfaces/Chip.jsx";
import type { ContinuityImpact, Finding, ProfileReadiness, RegulatoryProfile, SchemaField } from "../../api/client";
import { formatUtc } from "../../format";

const PROFILE_LABEL: Record<RegulatoryProfile, string> = {
  mrv: "MRV",
  dcs: "DCS",
  cii: "CII correction",
  voyageVerification: "Voyage verification",
};

// The key an acknowledgement is tracked by, both here and in ReportForm
// (which folds finding_acknowledged audit events into an acknowledgedKeys
// set using this same function) — a finding has no stable ID of its own,
// but ruleId+field together identify "this rule, on this field" uniquely
// enough for one report's current findings.
export function findingKey(f: Pick<Finding, "ruleId" | "field">): string {
  return `${f.ruleId}|${f.field ?? ""}`;
}

interface HealthCheckPanelProps {
  fields: SchemaField[];
  findings: Finding[];
  regulatoryReadiness: ProfileReadiness[];
  continuityImpact: ContinuityImpact[];
  onJumpToField: (fieldName: string) => void;
  /** Which findings (by findingKey) are currently acknowledged, derived from finding_acknowledged audit events for the report's current version — not local component state, so it survives a panel close/reopen and syncs to office. */
  acknowledgedKeys: Set<string>;
  /** Toggles one finding's acknowledgement — ReportForm owns the actual API call + optimistic update, this component only asks for it. */
  onToggleAcknowledge: (finding: Finding, nextAcknowledged: boolean) => void;
}

// The post-"Check report" review content, extended into FormWizard's own
// footer panel (via its `panel` prop) rather than a separate A6 screen —
// see PROJECT.md's "Vessel UI rework" section for why. Errors are the
// only thing that reads as blocking here — everything else (warnings,
// regulatory readiness, continuity impact) is informational, so only the
// errors card gets the bordered/red treatment; the rest is plain list
// rhythm on the surface, distinguished by icon+text rather than competing
// alarm colors. Submit itself and its confirm dialog live in ReportForm's
// own footer actions (alongside Back/Save draft/Check report), not here —
// this component is the review content only.
export function HealthCheckPanel({ fields, findings, regulatoryReadiness, continuityImpact, onJumpToField, acknowledgedKeys, onToggleAcknowledge }: HealthCheckPanelProps) {
  const [expandedProfiles, setExpandedProfiles] = useState<Set<RegulatoryProfile>>(new Set());

  const fieldLabel = (name: string) => fields.find((f) => f.name === name)?.label ?? name;

  const errors = findings.filter((f) => f.severity === "error");
  const warnings = findings.filter((f) => f.severity === "warning");
  const hasErrors = errors.length > 0;

  function toggleProfile(p: RegulatoryProfile) {
    setExpandedProfiles((prev) => {
      const next = new Set(prev);
      if (next.has(p)) next.delete(p);
      else next.add(p);
      return next;
    });
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-5)", maxWidth: 800 }}>
      {hasErrors ? (
        <section
          style={{
            border: "1px solid var(--color-error)",
            borderRadius: "var(--shape-medium)",
            padding: "var(--space-4)",
            background: "var(--color-error-container)",
          }}
        >
          <h2 className="md-title-small" style={{ color: "var(--color-on-error-container)", marginBottom: "var(--space-2)" }}>
            {errors.length} error{errors.length === 1 ? "" : "s"} — must be fixed before submitting
          </h2>
          <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-2)" }}>
            {errors.map((f, i) => (
              <FindingRow key={i} finding={f} onJumpToField={onJumpToField} />
            ))}
          </div>
        </section>
      ) : (
        <section style={{ display: "flex", alignItems: "center", gap: "var(--space-2)", color: "var(--color-status-underway)" }}>
          <span className="material-symbols-rounded">check_circle</span>
          <span className="md-title-small">No errors</span>
        </section>
      )}

      {warnings.length > 0 ? (
        <section>
          <h2 className="md-title-small" style={{ color: "var(--color-on-surface)", marginBottom: "var(--space-2)" }}>
            {warnings.length} warning{warnings.length === 1 ? "" : "s"}
          </h2>
          <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-2)" }}>
            {warnings.map((f, i) => {
              const acked = acknowledgedKeys.has(findingKey(f));
              return (
                <div key={i} style={{ display: "flex", alignItems: "flex-start", gap: "var(--space-3)", opacity: acked ? 0.55 : 1 }}>
                  <div style={{ flex: 1 }}>
                    <FindingRow finding={f} onJumpToField={onJumpToField} />
                  </div>
                  <Button
                    variant="text"
                    size="small"
                    icon={acked ? "check" : undefined}
                    onClick={() => onToggleAcknowledge(f, !acked)}
                  >
                    {acked ? "Acknowledged" : "Acknowledge"}
                  </Button>
                </div>
              );
            })}
          </div>
        </section>
      ) : null}

      {regulatoryReadiness.length > 0 ? (
        <section>
          <h2 className="md-title-small" style={{ color: "var(--color-on-surface)", marginBottom: "var(--space-2)" }}>
            Regulatory readiness
          </h2>
          <div style={{ display: "flex", flexWrap: "wrap", gap: "var(--space-3)" }}>
            {regulatoryReadiness.map((p) => (
              <Chip
                key={p.profile}
                label={p.ready ? `${PROFILE_LABEL[p.profile]} ✓` : `${PROFILE_LABEL[p.profile]}: ${p.missingFields.length} field${p.missingFields.length === 1 ? "" : "s"} missing`}
                icon={p.ready ? "check_circle" : "info"}
                selected={!p.ready && expandedProfiles.has(p.profile)}
                onClick={p.ready ? undefined : () => toggleProfile(p.profile)}
              />
            ))}
          </div>
          {[...expandedProfiles].map((p) => {
            const profile = regulatoryReadiness.find((r) => r.profile === p);
            if (!profile || profile.ready) return null;
            return (
              <div key={p} style={{ marginTop: "var(--space-2)" }}>
                <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)", marginBottom: "var(--space-1)" }}>
                  Missing for {PROFILE_LABEL[p]}:
                </div>
                <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-1)" }}>
                  {profile.missingFields.map((name) => (
                    <MissingFieldRow key={name} fieldName={name} label={fieldLabel(name)} onJumpToField={onJumpToField} />
                  ))}
                </div>
              </div>
            );
          })}
        </section>
      ) : null}

      {continuityImpact.length > 0 ? (
        <section>
          <h2 className="md-title-small" style={{ color: "var(--color-on-surface)", marginBottom: "var(--space-2)" }}>
            This affects {continuityImpact.length} other report{continuityImpact.length === 1 ? "" : "s"}
          </h2>
          <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-2)" }}>
            {continuityImpact.map((c) => (
              <div key={c.reportId} className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
                {c.eventType} at {formatUtc(c.eventTime)} — invalidated: {c.invalidatedRules.join(", ")}
              </div>
            ))}
          </div>
        </section>
      ) : null}
    </div>
  );
}

// A regulatory-readiness missing field, made clickable/navigable the same
// way FindingRow's error/warning rows already are (2026-07-14 manual-test
// feedback: "these fields are not clickable and navigatable like the
// errors and warning ... User expects the same behaviour").
function MissingFieldRow({ fieldName, label, onJumpToField }: { fieldName: string; label: string; onJumpToField: (fieldName: string) => void }) {
  return (
    <div
      role="button"
      tabIndex={0}
      onClick={() => onJumpToField(fieldName)}
      onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); onJumpToField(fieldName); } }}
      className="md-body-medium"
      style={{
        display: "flex", alignItems: "center", gap: "var(--space-2)",
        color: "var(--color-on-surface-variant)",
        cursor: "pointer",
      }}
    >
      <span className="material-symbols-rounded" style={{ fontSize: 18, color: "var(--color-status-caution)" }}>info</span>
      <span>{label}</span>
      <span className="material-symbols-rounded" aria-hidden="true" style={{ fontSize: 14, color: "var(--color-on-surface-variant)" }}>arrow_forward</span>
    </div>
  );
}

function FindingRow({ finding, onJumpToField }: { finding: Finding; onJumpToField: (fieldName: string) => void }) {
  const jumpable = Boolean(finding.field);
  return (
    <div
      role={jumpable ? "button" : undefined}
      tabIndex={jumpable ? 0 : undefined}
      onClick={jumpable ? () => onJumpToField(finding.field!) : undefined}
      onKeyDown={jumpable ? (e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); onJumpToField(finding.field!); } } : undefined}
      className="md-body-medium"
      style={{
        display: "flex", alignItems: "center", gap: "var(--space-2)",
        color: finding.severity === "error" ? "var(--color-on-error-container)" : "var(--color-on-surface)",
        cursor: jumpable ? "pointer" : "default",
      }}
    >
      <span className="material-symbols-rounded" style={{ fontSize: 18, color: finding.severity === "error" ? "var(--color-error)" : "var(--color-status-caution)" }}>
        {finding.severity === "error" ? "error" : "warning"}
      </span>
      <span>{finding.message}</span>
      {jumpable ? (
        <span className="material-symbols-rounded" aria-hidden="true" style={{ fontSize: 14, color: "var(--color-on-surface-variant)" }}>arrow_forward</span>
      ) : null}
    </div>
  );
}
