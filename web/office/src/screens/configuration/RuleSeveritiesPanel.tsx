// SPDX-License-Identifier: AGPL-3.0-only

import { useEffect, useState } from "react";
import { Button } from "../../design/components/core/Button.jsx";
import { Card } from "../../design/components/surfaces/Card.jsx";
import { Select } from "../../design/components/forms/Select.jsx";
import { AlertBanner } from "../../design/components/feedback/AlertBanner.jsx";
import { api, type RuleCatalog, type RuleSeverityAssignmentView, type Scope, type VesselView } from "../../api/client";
import { ScopeSelector } from "./ScopeSelector";
import { OverridePrecedenceBanner } from "./OverridePrecedenceBanner";
import { ruleLabel, scopeLabel, scopesEqual } from "./complianceLogic";

const SEVERITIES = ["error", "warning", "info"];

// Icon color per current severity — a rule's row reads at a glance
// without having to read the Select's value, matching the visual weight
// an "error" override should carry over an unset "(default)" one.
const SEVERITY_ICON_COLOR: Record<string, string> = {
  "(default)": "var(--color-on-surface-variant)",
  info: "var(--color-tertiary)",
  warning: "var(--color-status-caution)",
  error: "var(--color-status-warning)",
};

// Design handoff B7: "Rule severities: table of plausibility rules with
// severity selector where allowed (hard OVD rules shown locked as
// error)."
export function RuleSeveritiesPanel({ vessels, canEdit }: { vessels: VesselView[]; canEdit: boolean }) {
  const [catalog, setCatalog] = useState<RuleCatalog | null>(null);
  const [assignments, setAssignments] = useState<RuleSeverityAssignmentView[] | null>(null);
  const [scope, setScope] = useState<Scope>({ type: "fleet" });
  const [severities, setSeverities] = useState<Record<string, string>>({});
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    api
      .listOverridableRules()
      .then(setCatalog)
      .catch((err) => setError(err instanceof Error ? err.message : "Could not load the rule catalog."));
  }, []);

  const reload = () => {
    api
      .listRuleSeverityAssignments()
      .then(setAssignments)
      .catch((err) => setError(err instanceof Error ? err.message : "Could not load rule severities."));
  };

  useEffect(reload, []);

  useEffect(() => {
    const current = assignments?.find((a) => scopesEqual(a.scope, scope));
    setSeverities(current?.severities ?? {});
  }, [scope, assignments]);

  async function save() {
    setSaving(true);
    setError(null);
    try {
      await api.saveRuleSeverityAssignment(scope, severities);
      reload();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not save rule severities.");
    } finally {
      setSaving(false);
    }
  }

  if (!catalog) {
    return (
      <div className="md-body-medium" style={{ padding: 24, color: "var(--color-on-surface-variant)" }}>
        Loading…
      </div>
    );
  }

  return (
    <div style={{ padding: 24, display: "flex", flexDirection: "column", gap: 16, maxWidth: 720 }}>
      {error ? <AlertBanner level="warning" title="Something went wrong" message={error} onDismiss={() => setError(null)} /> : null}
      <OverridePrecedenceBanner />
      <ScopeSelector scope={scope} onChange={setScope} vessels={vessels} />

      <Card variant="outlined" style={{ padding: 0, display: "flex", flexDirection: "column" }}>
        <div className="md-title-medium" style={{ padding: "14px 18px" }}>Rule severities for {scopeLabel(scope, vessels)}</div>
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 16, padding: "0 20px 10px" }}>
          <span className="md-label-large" style={{ color: "var(--color-on-surface-variant)" }}>Rule</span>
          <span className="md-label-large" style={{ color: "var(--color-on-surface-variant)" }}>Severity</span>
        </div>
        {[
          ...catalog.overridable.map((ruleID) => ({ id: ruleID, rule: ruleLabel(ruleID), locked: false })),
          ...catalog.hard.map((ruleID) => ({ id: ruleID, rule: ruleLabel(ruleID), locked: true })),
        ].map((row) => {
          const severity = severities[row.id] ?? "(default)";
          return (
            <div
              key={row.id}
              style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 16, padding: "16px 20px", borderTop: "1px solid var(--color-outline-variant)" }}
            >
              <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
                <span
                  className="material-symbols-rounded"
                  style={{ fontSize: 20, color: row.locked ? "var(--color-on-surface-variant)" : SEVERITY_ICON_COLOR[severity] ?? "var(--color-on-surface-variant)" }}
                >
                  {row.locked ? "gpp_maybe" : "rule"}
                </span>
                <span className="md-body-medium">{row.rule}</span>
              </div>
              {row.locked ? (
                <span
                  className="md-label-medium"
                  style={{
                    display: "inline-flex", alignItems: "center", gap: 6, padding: "6px 12px",
                    borderRadius: "var(--shape-full)", background: "var(--color-surface-container-highest)", color: "var(--color-on-surface-variant)",
                  }}
                >
                  <span className="material-symbols-rounded" style={{ fontSize: 15 }}>lock</span>
                  Error (locked)
                </span>
              ) : (
                <Select
                  label="Severity"
                  value={severity}
                  options={["(default)", ...SEVERITIES]}
                  onChange={(v) =>
                    canEdit &&
                    setSeverities((prev) => {
                      const next = { ...prev };
                      if (v === "(default)") delete next[row.id];
                      else next[row.id] = v;
                      return next;
                    })
                  }
                  style={{ width: 150 }}
                />
              )}
            </div>
          );
        })}
        {canEdit ? (
          <div style={{ padding: "16px 20px" }}>
            <Button variant="filled" onClick={save} disabled={saving}>
              Save
            </Button>
          </div>
        ) : null}
      </Card>
    </div>
  );
}
