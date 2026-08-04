// SPDX-License-Identifier: AGPL-3.0-only

import { useEffect, useState } from "react";
import { Button } from "../../design/components/core/Button.jsx";
import { Card } from "../../design/components/surfaces/Card.jsx";
import { AlertBanner } from "../../design/components/feedback/AlertBanner.jsx";
import { api, type ProfileAssignmentView, type Scope, type VesselView } from "../../api/client";
import { ScopeSelector } from "./ScopeSelector";
import { OverridePrecedenceBanner } from "./OverridePrecedenceBanner";
import { ALL_PROFILES, PROFILE_LABELS, SCOPE_ICONS, scopeLabel, scopesEqual } from "./complianceLogic";

// Design handoff B7: "Regulatory profiles: toggle cards ... assignable
// fleet-wide, per group or per vessel."
export function RegulatoryProfilesPanel({ vessels, canEdit }: { vessels: VesselView[]; canEdit: boolean }) {
  const [assignments, setAssignments] = useState<ProfileAssignmentView[] | null>(null);
  const [scope, setScope] = useState<Scope>({ type: "fleet" });
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  const reload = () => {
    api
      .listProfileAssignments()
      .then(setAssignments)
      .catch((err) => setError(err instanceof Error ? err.message : "Could not load regulatory profiles."));
  };

  useEffect(reload, []);

  useEffect(() => {
    const current = assignments?.find((a) => scopesEqual(a.scope, scope));
    setSelected(new Set(current?.profiles ?? []));
  }, [scope, assignments]);

  const scopeExists = assignments?.some((a) => scopesEqual(a.scope, scope)) ?? false;

  function toggle(profile: string) {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(profile)) {
        next.delete(profile);
      } else {
        next.add(profile);
      }
      return next;
    });
  }

  async function save() {
    setSaving(true);
    setError(null);
    try {
      await api.saveProfileAssignment(scope, [...selected]);
      reload();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not save regulatory profiles.");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div style={{ padding: 24, display: "flex", flexDirection: "column", gap: 16, maxWidth: 720 }}>
      {error ? <AlertBanner level="warning" title="Something went wrong" message={error} onDismiss={() => setError(null)} /> : null}
      <OverridePrecedenceBanner />
      <ScopeSelector scope={scope} onChange={setScope} vessels={vessels} />

      <Card variant="outlined" style={{ padding: 0, display: "flex", flexDirection: "column" }}>
        <div style={{ padding: "16px 18px", display: "flex", flexDirection: "column", gap: 10 }}>
          <div className="md-label-medium" style={{ color: "var(--color-on-surface-variant)" }}>Profiles enabled for {scopeLabel(scope, vessels)}</div>
          <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(230px, 1fr))", gap: 8 }}>
            {ALL_PROFILES.map((p) => {
              const isSelected = selected.has(p);
              return (
                <div
                  key={p}
                  onClick={() => canEdit && toggle(p)}
                  style={{
                    display: "flex", alignItems: "center", gap: 10, padding: "10px 12px",
                    borderRadius: "var(--shape-small)", cursor: canEdit ? "pointer" : "default",
                    background: isSelected ? "var(--color-secondary-container)" : "var(--color-surface-container-low)",
                    border: `1px solid ${isSelected ? "transparent" : "var(--color-outline-variant)"}`,
                    color: isSelected ? "var(--color-on-secondary-container)" : "var(--color-on-surface)",
                  }}
                >
                  <span
                    style={{
                      width: 18, height: 18, borderRadius: "50%", flexShrink: 0,
                      display: "flex", alignItems: "center", justifyContent: "center",
                      background: isSelected ? "var(--color-secondary)" : "transparent",
                      border: isSelected ? "none" : "2px solid var(--color-outline)",
                      color: "var(--color-on-secondary)",
                    }}
                  >
                    {isSelected ? <span className="material-symbols-rounded" style={{ fontSize: 14 }}>check</span> : null}
                  </span>
                  <span className="md-body-medium">{PROFILE_LABELS[p]}</span>
                </div>
              );
            })}
          </div>
        </div>
        {canEdit ? (
          <div style={{ padding: "12px 18px", background: "var(--color-surface-container-low)", display: "flex", alignItems: "center", justifyContent: "space-between", flexWrap: "wrap", gap: 12 }}>
            <span className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
              {scopeExists ? "This scope already has profiles assigned — saving will update them." : "No profiles assigned to this scope yet."}
            </span>
            <Button variant="filled" onClick={save} disabled={saving}>
              {scopeExists ? "Update" : "Save"}
            </Button>
          </div>
        ) : null}
      </Card>

      <div>
        <div className="md-title-small" style={{ marginBottom: 8 }}>
          Current regulatory profiles
        </div>
        {assignments && assignments.length > 0 ? (
          <div style={{ borderRadius: "var(--shape-medium)", border: "1px solid var(--color-outline-variant)", background: "var(--color-surface)", overflow: "hidden" }}>
            <div style={{ display: "grid", gridTemplateColumns: "1.1fr 2.5fr", padding: "12px 20px", background: "var(--color-surface-container-low)", borderBottom: "1px solid var(--color-outline-variant)" }}>
              <span className="md-label-large" style={{ color: "var(--color-on-surface-variant)" }}>Scope</span>
              <span className="md-label-large" style={{ color: "var(--color-on-surface-variant)" }}>Profiles applied</span>
            </div>
            {assignments.map((a) => {
              const icons = SCOPE_ICONS[a.scope.type];
              return (
                <div
                  key={scopeLabel(a.scope, vessels)}
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
                    <span className="md-title-small">{scopeLabel(a.scope, vessels)}</span>
                  </div>
                  <div style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
                    {a.profiles.length > 0 ? (
                      a.profiles.map((p) => (
                        <span
                          key={p}
                          className="md-label-medium"
                          style={{ padding: "5px 12px", borderRadius: "var(--shape-full)", background: "var(--color-secondary-container)", color: "var(--color-on-secondary-container)" }}
                        >
                          {PROFILE_LABELS[p] ?? p}
                        </span>
                      ))
                    ) : (
                      <span className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>none enabled</span>
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        ) : (
          <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
            No regulatory profiles assigned anywhere yet.
          </div>
        )}
      </div>
    </div>
  );
}
