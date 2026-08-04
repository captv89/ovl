// SPDX-License-Identifier: AGPL-3.0-only

import { useEffect, useMemo, useState } from "react";
import { Button } from "../../design/components/core/Button.jsx";
import { Card } from "../../design/components/surfaces/Card.jsx";
import { TextField } from "../../design/components/forms/TextField.jsx";
import { Select } from "../../design/components/forms/Select.jsx";
import { Dialog } from "../../design/components/feedback/Dialog.jsx";
import { AlertBanner } from "../../design/components/feedback/AlertBanner.jsx";
import { DataTable } from "../../design/components/data/DataTable.jsx";
import { api, type BundleAssignmentView, type ConfigBundleSummary, type Scope, type VesselView } from "../../api/client";
import { ScopeSelector } from "./ScopeSelector";
import { OverridePrecedenceBanner } from "./OverridePrecedenceBanner";
import { scopeLabel } from "./complianceLogic";

// Design handoff B7's bundle composer: "showing exactly what goes in
// (schema versions, policies, profiles, cadence, severities),
// assignment picker (vessels or groups), a diff against each target's
// current bundle, publish with confirmation. Bundle history ... and
// per-vessel applied-state (pulled or pending next sync)." Per-target
// diff and applied-state both need Phase 4 sync (no sync cursor concept
// exists anywhere in office/store yet) — every assignment is shown as
// "Pending next sync" rather than a faked pulled/pending distinction.
//
// 2026-07-26 redesign: publishing and assigning are two distinct steps
// (compose+publish just creates an immutable snapshot; nothing is "live"
// anywhere until a separate Assign targets it at fleet/group/vessel
// scope — see README's "Config bundle model" section) — a bundle history
// list that only showed publish metadata made it easy to mistake
// "published" for "applied". History is now one table joining publish
// info with current assignment(s), so "this bundle exists" and "this
// bundle is live somewhere" are never two things you have to
// cross-reference by hand. This replaced separate "Latest published" and
// "Bundle assignments" sections that duplicated data already in the
// history list/table.
export function BundleComposerPanel({ vessels, canEdit }: { vessels: VesselView[]; canEdit: boolean }) {
  const [preview, setPreview] = useState<ConfigBundleSummary | null>(null);
  const [history, setHistory] = useState<ConfigBundleSummary[] | null>(null);
  const [assignments, setAssignments] = useState<BundleAssignmentView[] | null>(null);
  const [label, setLabel] = useState("");
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const [assignScope, setAssignScope] = useState<Scope>({ type: "fleet" });
  const [assignBundleID, setAssignBundleID] = useState("");

  const reload = () => {
    api.previewConfigBundle().then(setPreview).catch((err) => setError(err instanceof Error ? err.message : "Could not load the bundle preview."));
    api.listConfigBundles().then(setHistory).catch(() => {});
    api.listBundleAssignments().then(setAssignments).catch(() => {});
  };

  useEffect(reload, []);

  async function publish() {
    setConfirmOpen(false);
    setBusy(true);
    setError(null);
    try {
      await api.publishConfigBundle(label);
      setLabel("");
      reload();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not publish the bundle.");
    } finally {
      setBusy(false);
    }
  }

  async function assign() {
    setBusy(true);
    setError(null);
    try {
      await api.saveBundleAssignment(assignScope, assignBundleID);
      reload();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not save the bundle assignment.");
    } finally {
      setBusy(false);
    }
  }

  // scopeLabel per assigned scope, grouped by bundle id — a bundle can
  // simultaneously be the live assignment for more than one scope (e.g.
  // fleet default *and* a vessel-specific override both happen to point
  // at the same bundle), so this is a list per bundle, not a 1:1 lookup.
  const assignedToByBundle = useMemo(() => {
    const map = new Map<string, string[]>();
    for (const a of assignments ?? []) {
      const list = map.get(a.bundleId) ?? [];
      list.push(scopeLabel({ type: a.scopeType, key: a.scopeKey }, vessels));
      map.set(a.bundleId, list);
    }
    return map;
  }, [assignments, vessels]);

  const historyRows = (history ?? []).map((b) => {
    const assignedTo = assignedToByBundle.get(b.id) ?? [];
    return {
      id: b.id,
      bundle: { icon: "inventory_2", text: b.label || "(unlabeled)", subtext: `by ${b.publishedBy}` },
      publishedAt: b.publishedAt,
      contents: `${b.schemaVersionCount} schemas, ${b.fieldPolicyRows} field policy, ${b.regulatoryProfileRows} profiles, ${b.cadenceRuleRows} cadence, ${b.ruleSeverityRows} severity`,
      contentsChips: [
        `${b.schemaVersionCount} schemas`,
        `${b.fieldPolicyRows} field policy`,
        `${b.regulatoryProfileRows} profiles`,
        `${b.cadenceRuleRows} cadence`,
        `${b.ruleSeverityRows} severity`,
      ],
      assignedTo: assignedTo.length > 0 ? assignedTo.join(", ") : "Not assigned",
      assignedToList: assignedTo,
    };
  });

  return (
    <div style={{ padding: 24, display: "flex", flexDirection: "column", gap: 24, position: "relative" }}>
      {error ? <AlertBanner level="warning" title="Something went wrong" message={error} onDismiss={() => setError(null)} /> : null}

      <div className="md-title-large">Bundles</div>

      <div style={{ display: "flex", gap: 20, alignItems: "flex-start", flexWrap: "wrap" }}>
        <Card variant="outlined" style={{ flex: "1 1 45%", minWidth: 360, padding: 18, display: "flex", flexDirection: "column", gap: 12 }}>
          <div className="md-label-medium" style={{ color: "var(--color-on-surface-variant)", textTransform: "uppercase", letterSpacing: "0.04em" }}>
            Compose new bundle
          </div>
          <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)" }}>
            Snapshots the field policy, regulatory profile, cadence rule, and rule severity scopes set on the other
            tabs — every fleet/group/vessel override, as they currently stand — into one immutable packet.
          </div>
          <div style={{ display: "flex", gap: 12, alignItems: "flex-end", flexWrap: "wrap" }}>
            {canEdit ? (
              <>
                <TextField label="Bundle label" value={label} onChange={setLabel} supportingText='e.g. "2026-07 fleet update"' />
                <Button variant="filled" onClick={() => setConfirmOpen(true)} disabled={busy || !label.trim()}>
                  Create bundle
                </Button>
              </>
            ) : null}
          </div>
          {preview ? (
            <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)" }}>
              Snapshots {preview.schemaVersionCount} schema versions, {preview.fieldPolicyRows} field policy scopes,{" "}
              {preview.regulatoryProfileRows} regulatory profile scopes, {preview.cadenceRuleRows} cadence rule scopes,{" "}
              {preview.ruleSeverityRows} rule severity scopes across the fleet.
            </div>
          ) : null}
        </Card>

        <Card variant="outlined" style={{ flex: "1 1 45%", minWidth: 360, padding: 18, display: "flex", flexDirection: "column", gap: 12 }}>
          <div className="md-label-medium" style={{ color: "var(--color-on-surface-variant)", textTransform: "uppercase", letterSpacing: "0.04em" }}>
            Assign a bundle
          </div>
          <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)" }}>
            Publishing alone changes nothing on any vessel. A bundle only takes effect once it's assigned here — that
            assignment is what a vessel actually pulls on its next sync.
          </div>
          <OverridePrecedenceBanner />
          {canEdit ? (
            <>
              <div style={{ display: "flex", gap: 12, alignItems: "flex-end", flexWrap: "wrap" }}>
                <ScopeSelector scope={assignScope} onChange={setAssignScope} vessels={vessels} />
                <Select
                  label="Bundle"
                  placeholder="Select a bundle…"
                  value={bundleOptionLabel(history, assignBundleID)}
                  options={(history ?? []).map((b) => bundleOptionLabel(history, b.id))}
                  onChange={(label) => setAssignBundleID((history ?? []).find((b) => bundleOptionLabel(history, b.id) === label)?.id ?? "")}
                />
                <Button
                  variant="outlined"
                  onClick={assign}
                  disabled={busy || !assignBundleID || (assignScope.type !== "fleet" && !assignScope.key)}
                >
                  Assign
                </Button>
              </div>
              {!assignBundleID || (assignScope.type !== "fleet" && !assignScope.key) ? (
                <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)" }}>
                  {!assignBundleID
                    ? "Select a bundle above to enable Assign."
                    : `Pick a ${assignScope.type} before assigning.`}
                </div>
              ) : null}
            </>
          ) : null}
        </Card>
      </div>

      <div>
        <div className="md-title-large" style={{ marginBottom: 10 }}>
          Bundle history
        </div>
        <DataTable
          columns={[
            { key: "bundle", label: "Bundle", type: "iconText", sortable: true },
            {
              key: "publishedAt",
              label: "Published",
              type: "text",
              sortable: true,
              render: (row) => (
                <span className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
                  {new Date(row.publishedAt).toLocaleString()}
                </span>
              ),
            },
            {
              key: "contents",
              label: "Contents",
              render: (row) => (
                <div style={{ display: "flex", gap: 6, flexWrap: "wrap" }}>
                  {row.contentsChips.map((c: string) => (
                    <HistoryChip key={c} label={c} />
                  ))}
                </div>
              ),
            },
            {
              key: "assignedTo",
              label: "Assigned to",
              filterable: true,
              render: (row) =>
                row.assignedToList.length > 0 ? (
                  <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
                    <div style={{ display: "flex", gap: 6, flexWrap: "wrap" }}>
                      {row.assignedToList.map((s: string) => (
                        <span
                          key={s}
                          className="md-label-medium"
                          style={{
                            padding: "4px 10px", borderRadius: "var(--shape-full)",
                            background: "var(--color-primary-container)", color: "var(--color-on-primary-container)",
                          }}
                        >
                          {s}
                        </span>
                      ))}
                    </div>
                    <span className="md-body-small" style={{ color: "var(--color-on-surface-variant)" }}>
                      Pending next sync
                    </span>
                  </div>
                ) : (
                  <span className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
                    Not assigned
                  </span>
                ),
            },
          ]}
          rows={historyRows}
          searchPlaceholder="Search bundle history"
          emptyMessage="No bundles published yet."
          maxRows={6}
        />
      </div>

      <Dialog
        open={confirmOpen}
        title="Publish this bundle?"
        onClose={() => setConfirmOpen(false)}
        actions={[
          { label: "Cancel", onClick: () => setConfirmOpen(false) },
          { label: "Publish", onClick: publish },
        ]}
      >
        This creates a new immutable configuration snapshot labeled "{label}". It does not change what's assigned to
        any vessel or group until you assign it below.
      </Dialog>
    </div>
  );
}

// Select's options are plain display strings (no separate id/label
// pairs), so the bundle picker looks its id up by re-deriving the same
// label string it renders — label+publish-date is unique enough in
// practice, matching the exact display text this panel's raw <select>
// already used before this Select swap (design handoff B7 wireframe fix
// #3's other half).
function bundleOptionLabel(history: ConfigBundleSummary[] | null, id: string): string {
  const b = (history ?? []).find((b) => b.id === id);
  return b ? `${b.label || b.id} (${new Date(b.publishedAt).toLocaleDateString()})` : "";
}

function HistoryChip({ label }: { label: string }) {
  return (
    <span
      className="md-label-medium"
      style={{ padding: "4px 10px", borderRadius: "var(--shape-full)", background: "var(--color-surface-container-highest)", color: "var(--color-on-surface-variant)" }}
    >
      {label}
    </span>
  );
}
