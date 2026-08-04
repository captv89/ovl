// SPDX-License-Identifier: AGPL-3.0-only

import { useEffect, useState } from "react";
import { Button } from "../../design/components/core/Button.jsx";
import { Card } from "../../design/components/surfaces/Card.jsx";
import { TextField } from "../../design/components/forms/TextField.jsx";
import { AlertBanner } from "../../design/components/feedback/AlertBanner.jsx";
import { api, type CadenceRuleView, type Scope, type VesselView } from "../../api/client";
import { ScopeSelector } from "./ScopeSelector";
import { OverridePrecedenceBanner } from "./OverridePrecedenceBanner";
import { SCOPE_ICONS, scopeLabel, scopesEqual } from "./complianceLogic";

// Design handoff B7: "Cadence rules: min one report per rolling N
// hours, max gap M hours, defaults 24 and 12 ... Preview sentence in
// plain language."
export function CadenceRulesPanel({ vessels, canEdit }: { vessels: VesselView[]; canEdit: boolean }) {
  const [rules, setRules] = useState<CadenceRuleView[] | null>(null);
  const [scope, setScope] = useState<Scope>({ type: "fleet" });
  const [minInterval, setMinInterval] = useState("24");
  const [maxGap, setMaxGap] = useState("12");
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  const reload = () => {
    api
      .listCadenceRules()
      .then(setRules)
      .catch((err) => setError(err instanceof Error ? err.message : "Could not load cadence rules."));
  };

  useEffect(reload, []);

  useEffect(() => {
    const current = rules?.find((r) => scopesEqual(r.scope, scope));
    setMinInterval(String(current?.minReportIntervalHours ?? 24));
    setMaxGap(String(current?.maxGapHours ?? 12));
  }, [scope, rules]);

  async function save() {
    setSaving(true);
    setError(null);
    try {
      await api.saveCadenceRule(scope, Number(minInterval), Number(maxGap));
      reload();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not save the cadence rule.");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div style={{ padding: 24, display: "flex", flexDirection: "column", gap: 16, maxWidth: 720 }}>
      {error ? <AlertBanner level="warning" title="Something went wrong" message={error} onDismiss={() => setError(null)} /> : null}
      <OverridePrecedenceBanner />
      <ScopeSelector scope={scope} onChange={setScope} vessels={vessels} />

      <Card variant="outlined" style={{ padding: 20, display: "flex", flexDirection: "column", gap: 12 }}>
        <div className="md-title-medium">Cadence for {scopeLabel(scope, vessels)}</div>
        <div style={{ display: "flex", gap: 16 }}>
          <TextField label="Min report interval (hours)" value={minInterval} onChange={setMinInterval} type="number" />
          <TextField label="Max gap (hours)" value={maxGap} onChange={setMaxGap} type="number" />
        </div>
        <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
          Vessels {scope.type === "fleet" ? "fleet-wide" : scopeLabel(scope, vessels).toLowerCase()} must report at
          least every {maxGap || "?"} hours, with no more than {minInterval || "?"} hours between the start of
          consecutive reporting windows.
        </div>
        {canEdit ? (
          <div>
            <Button variant="filled" onClick={save} disabled={saving || !minInterval || !maxGap}>
              Save
            </Button>
          </div>
        ) : null}
      </Card>

      <div>
        <div className="md-title-small" style={{ marginBottom: 8 }}>
          Current cadence rules
        </div>
        {rules && rules.length > 0 ? (
          <div style={{ borderRadius: "var(--shape-medium)", border: "1px solid var(--color-outline-variant)", background: "var(--color-surface)", overflow: "hidden" }}>
            <div style={{ display: "grid", gridTemplateColumns: "1.1fr 2.5fr", padding: "12px 20px", background: "var(--color-surface-container-low)", borderBottom: "1px solid var(--color-outline-variant)" }}>
              <span className="md-label-large" style={{ color: "var(--color-on-surface-variant)" }}>Scope</span>
              <span className="md-label-large" style={{ color: "var(--color-on-surface-variant)" }}>Cadence</span>
            </div>
            {rules.map((r) => {
              const icons = SCOPE_ICONS[r.scope.type];
              return (
                <div
                  key={scopeLabel(r.scope, vessels)}
                  style={{ display: "grid", gridTemplateColumns: "1.1fr 2.5fr", alignItems: "center", padding: "14px 20px", borderBottom: "1px solid var(--color-outline-variant)" }}
                >
                  <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
                    <span
                      style={{
                        width: 32, height: 32, borderRadius: "var(--shape-small)", background: icons.bg, color: icons.fg,
                        display: "flex", alignItems: "center", justifyContent: "center", flexShrink: 0,
                      }}
                    >
                      <span className="material-symbols-rounded" style={{ fontSize: 18 }}>{icons.icon}</span>
                    </span>
                    <span className="md-title-small">{scopeLabel(r.scope, vessels)}</span>
                  </div>
                  <span className="md-body-medium">
                    report at least every {r.maxGapHours}h, min interval {r.minReportIntervalHours}h
                  </span>
                </div>
              );
            })}
          </div>
        ) : (
          <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
            No cadence overrides yet — every vessel uses the default (report at least every 12h, min interval 24h).
          </div>
        )}
      </div>
    </div>
  );
}
