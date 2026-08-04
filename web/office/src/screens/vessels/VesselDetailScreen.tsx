// SPDX-License-Identifier: AGPL-3.0-only

import { useEffect, useState } from "react";
import { Button } from "../../design/components/core/Button.jsx";
import { Card } from "../../design/components/surfaces/Card.jsx";
import { Tabs } from "../../design/components/navigation/Tabs.jsx";
import { Dialog } from "../../design/components/feedback/Dialog.jsx";
import { TextField } from "../../design/components/forms/TextField.jsx";
import { AlertBanner } from "../../design/components/feedback/AlertBanner.jsx";
import { DataTable } from "../../design/components/data/DataTable.jsx";
import { Select } from "../../design/components/forms/Select.jsx";
import {
  api,
  type IssueResultView,
  type QueuedUserCommandResponse,
  type ReportListItemView,
  type ReportState,
  type RestoreCommandView,
  type UserCommandView,
  type VesselDetailView,
  type VesselUserView,
} from "../../api/client";
import { VesselProfileForm } from "./VesselProfileForm";
import { StatePill } from "../reports/ReportDetailScreen";
import { HealthCell } from "../reports/ReportList";
import { syncHealth } from "../../format";

const TABS = ["Profile", "Enrollment", "Users", "Config", "Reports", "DR"] as const;
type Tab = (typeof TABS)[number];

// vessel/auth.Role's own string values (architecture 9.3) — duplicated
// here rather than derived from anything server-provided, same "kept in
// sync by hand, six roles that haven't changed since Phase 2" reasoning
// as pkg/syncproto.IsValidVesselRole on the Go side.
const VESSEL_ROLES: { value: string; label: string }[] = [
  { value: "chiefOfficer", label: "Chief Officer" },
  { value: "secondOfficer", label: "Second Officer" },
  { value: "thirdOfficer", label: "Third Officer" },
  { value: "chiefEngineer", label: "Chief Engineer" },
  { value: "secondEngineer", label: "Second Engineer" },
];
function vesselRoleLabel(value: string): string {
  if (value === "master") return "Master";
  return VESSEL_ROLES.find((r) => r.value === value)?.label ?? value;
}

const ENROLLMENT_LABEL: Record<string, string> = { notIssued: "Not issued", issued: "Issued", enrolled: "Enrolled", revoked: "Revoked" };

// Design handoff B2's vessel detail. All five tabs are real — DR's
// restore-bundle generation (architecture 12.5) landed backend-side
// 2026-07-16 (office/httpapi/restorebundle.go) but this screen was never
// updated to use it; DRTab below is that wiring, not new backend work.
export function VesselDetailScreen({
  vesselId,
  isAdmin,
  onBack,
  onOpenReport,
}: {
  vesselId: string;
  isAdmin: boolean;
  onBack: () => void;
  onOpenReport: (reportId: string) => void;
}) {
  const [detail, setDetail] = useState<VesselDetailView | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [tab, setTab] = useState<Tab>("Profile");
  const [editingProfile, setEditingProfile] = useState(false);

  const reload = () => {
    api
      .getVessel(vesselId)
      .then(setDetail)
      .catch((err) => setError(err instanceof Error ? err.message : "Could not load vessel."));
  };

  useEffect(reload, [vesselId]);

  if (error) {
    return (
      <div style={{ padding: 24 }}>
        <Button variant="text" icon="arrow_back" onClick={onBack}>
          Back to vessels
        </Button>
        <AlertBanner level="warning" title="Couldn't load vessel" message={error} />
      </div>
    );
  }
  if (!detail) {
    return (
      <div className="md-body-medium" style={{ padding: 24, color: "var(--color-on-surface-variant)" }}>
        Loading vessel…
      </div>
    );
  }

  const { vessel } = detail;

  return (
    <div style={{ padding: 24, display: "flex", flexDirection: "column", gap: 16 }}>
      <div>
        <Button variant="text" icon="arrow_back" onClick={onBack}>
          Back to vessels
        </Button>
        <div className="md-headline-small" style={{ marginTop: 4 }}>
          {vessel.name}
        </div>
        <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
          IMO {vessel.imo} · {vessel.type} · {ENROLLMENT_LABEL[vessel.enrollmentState]}
        </div>
      </div>

      <Tabs items={[...TABS]} selected={tab} onSelect={(t) => setTab(t as Tab)} />

      {tab === "Profile" ? (
        <ProfileTab
          detail={detail}
          isAdmin={isAdmin}
          editing={editingProfile}
          onEdit={() => setEditingProfile(true)}
          onCancel={() => setEditingProfile(false)}
          onSaved={() => {
            setEditingProfile(false);
            reload();
          }}
        />
      ) : null}
      {tab === "Enrollment" ? <EnrollmentTab detail={detail} isAdmin={isAdmin} onChanged={reload} /> : null}
      {tab === "Users" ? <UsersTab detail={detail} isAdmin={isAdmin} onChanged={reload} /> : null}
      {tab === "Config" ? <ConfigTab detail={detail} /> : null}
      {tab === "Reports" ? <ReportsTab vesselId={vesselId} onOpenReport={onOpenReport} /> : null}
      {tab === "DR" ? <DRTab detail={detail} isAdmin={isAdmin} onChanged={reload} /> : null}
    </div>
  );
}

function ProfileTab({
  detail,
  isAdmin,
  editing,
  onEdit,
  onCancel,
  onSaved,
}: {
  detail: VesselDetailView;
  isAdmin: boolean;
  editing: boolean;
  onEdit: () => void;
  onCancel: () => void;
  onSaved: () => void;
}) {
  const { vessel } = detail;
  if (editing) {
    return (
      <VesselProfileForm
        initial={{ imo: vessel.imo, name: vessel.name, type: vessel.type, groups: vessel.groups }}
        submitLabel="Save changes"
        onCancel={onCancel}
        onSubmit={async ({ name, type, groups }) => {
          await api.updateVessel(vessel.id, name, type, groups);
          onSaved();
        }}
      />
    );
  }
  const health = syncHealth(vessel.lastSyncAt);
  const healthColor = health.tone === "success" ? "var(--color-status-underway)" : health.tone === "warning" ? "var(--color-error)" : "var(--color-on-surface-variant)";

  return (
    <Card variant="outlined" style={{ padding: 20, maxWidth: 480, display: "flex", flexDirection: "column", gap: 12 }}>
      <Field label="IMO number" value={vessel.imo} />
      <Field label="Vessel name" value={vessel.name} />
      <Field label="Type" value={vessel.type} />
      <Field label="Group tags" value={vessel.groups.length > 0 ? vessel.groups.join(", ") : "None"} />
      <div>
        <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)" }}>
          Vessel server
        </div>
        <div className="md-body-large">
          <span style={{ color: healthColor, fontWeight: 600 }}>{health.label}</span>
          {vessel.lastSyncAt ? ` · last synced ${new Date(vessel.lastSyncAt).toLocaleString()}` : ""}
          {vessel.appVersion ? ` · v${vessel.appVersion}` : ""}
        </div>
      </div>
      {isAdmin ? (
        <div>
          <Button variant="outlined" icon="edit" onClick={onEdit}>
            Edit profile
          </Button>
        </div>
      ) : null}
    </Card>
  );
}

function Field({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)" }}>
        {label}
      </div>
      <div className="md-body-large">{value}</div>
    </div>
  );
}

function EnrollmentTab({ detail, isAdmin, onChanged }: { detail: VesselDetailView; isAdmin: boolean; onChanged: () => void }) {
  const { vessel, enrollment } = detail;
  const [issuedResult, setIssuedResult] = useState<IssueResultView | null>(null);
  const [masterUsername, setMasterUsername] = useState("master");
  const [confirmAction, setConfirmAction] = useState<"reissue" | "revoke" | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function issue() {
    setBusy(true);
    setError(null);
    try {
      const result = await api.issueEnrollment(vessel.id, masterUsername);
      setIssuedResult(result);
      onChanged();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not issue enrollment.");
    } finally {
      setBusy(false);
    }
  }

  async function reissue() {
    setConfirmAction(null);
    setBusy(true);
    setError(null);
    try {
      const result = await api.reissueEnrollment(vessel.id, masterUsername);
      setIssuedResult(result);
      onChanged();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not re-issue enrollment.");
    } finally {
      setBusy(false);
    }
  }

  async function revoke() {
    setConfirmAction(null);
    setBusy(true);
    setError(null);
    try {
      await api.revokeEnrollment(vessel.id);
      onChanged();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not revoke enrollment.");
    } finally {
      setBusy(false);
    }
  }

  if (issuedResult) {
    return (
      <Card variant="outlined" style={{ padding: 20, maxWidth: 480, display: "flex", flexDirection: "column", gap: 12 }}>
        <div className="md-title-medium">Enrollment sheet — shown once</div>
        <AlertBanner
          level="caution"
          title="Save these credentials now"
          message="This code and password cannot be shown again. Print or securely note them before continuing."
        />
        <Field label="Office enrollment code" value={issuedResult.code} />
        <Field label="Initial Master username" value={issuedResult.enrollment.initialMasterUsername ?? masterUsername} />
        <Field label="Initial Master password" value={issuedResult.initialMasterPassword} />
        <div>
          <Button variant="filled" onClick={() => setIssuedResult(null)}>
            I've saved this
          </Button>
        </div>
      </Card>
    );
  }

  return (
    <Card variant="outlined" style={{ padding: 20, maxWidth: 480, display: "flex", flexDirection: "column", gap: 12 }}>
      {error ? <AlertBanner level="warning" title="Action failed" message={error} onDismiss={() => setError(null)} /> : null}
      <Field label="State" value={ENROLLMENT_LABEL[enrollment?.state ?? "notIssued"]} />
      {enrollment?.issuedAt ? <Field label="Issued" value={new Date(enrollment.issuedAt).toLocaleString()} /> : null}
      {enrollment?.initialMasterUsername ? <Field label="Initial Master username" value={enrollment.initialMasterUsername} /> : null}
      {enrollment?.revokedAt ? <Field label="Revoked" value={new Date(enrollment.revokedAt).toLocaleString()} /> : null}

      {isAdmin ? (
        <>
          {!enrollment || enrollment.state === "notIssued" ? (
            <div style={{ display: "flex", gap: 8, alignItems: "flex-end" }}>
              <TextField label="Initial Master username" value={masterUsername} onChange={setMasterUsername} style={{ flex: 1 }} />
              <Button variant="filled" onClick={issue} disabled={busy}>
                Issue enrollment
              </Button>
            </div>
          ) : (
            <div style={{ display: "flex", gap: 8 }}>
              <Button variant="outlined" onClick={() => setConfirmAction("reissue")} disabled={busy}>
                Revoke and re-issue
              </Button>
              {enrollment.state !== "revoked" ? (
                <Button variant="outlined" onClick={() => setConfirmAction("revoke")} disabled={busy}>
                  Revoke
                </Button>
              ) : null}
            </div>
          )}
        </>
      ) : null}

      <Dialog
        open={confirmAction === "reissue"}
        title="Re-issue credential?"
        onClose={() => setConfirmAction(null)}
        actions={[
          { label: "Cancel", onClick: () => setConfirmAction(null) },
          { label: "Re-issue", onClick: reissue },
        ]}
      >
        The current code and initial Master password stop working immediately. A new one-time sheet is
        generated in their place.
      </Dialog>
      <Dialog
        open={confirmAction === "revoke"}
        title="Revoke credential?"
        onClose={() => setConfirmAction(null)}
        actions={[
          { label: "Cancel", onClick: () => setConfirmAction(null) },
          { label: "Revoke", onClick: revoke },
        ]}
      >
        The vessel will no longer be able to redeem this enrollment code. This can be undone by issuing a
        fresh one at any time.
      </Dialog>
    </Card>
  );
}

// B2's Users tab: architecture 9.3/12.4's remote vessel-user
// administration (2026-07-21). Every action here queues a command the
// vessel applies on its own next sync cycle (no immediate remote
// mutation is possible — sync is vessel-initiated only) — same shape as
// DR's push-to-vessel action. This is also the Master-forgot-their-
// password recovery path: resetting the Master's own password is
// allowed here (deliberately, that's the point), but creating a second
// Master or deactivating the existing one is refused server-side so
// office can never accidentally strand a vessel with zero local access.
function UsersTab({ detail, isAdmin, onChanged }: { detail: VesselDetailView; isAdmin: boolean; onChanged: () => void }) {
  const { vessel, vesselUsers, userCommands } = detail;
  const [addOpen, setAddOpen] = useState(false);
  const [addUsername, setAddUsername] = useState("");
  const [addRole, setAddRole] = useState(VESSEL_ROLES[0].label);
  const [roleTarget, setRoleTarget] = useState<VesselUserView | null>(null);
  const [roleChoice, setRoleChoice] = useState(VESSEL_ROLES[0].label);
  const [deactivateTarget, setDeactivateTarget] = useState<VesselUserView | null>(null);
  const [busyUsername, setBusyUsername] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [revealed, setRevealed] = useState<QueuedUserCommandResponse | null>(null);

  if (!isAdmin) {
    return <EmptyState icon="lock" title="Admin only" text="Remote user administration is an Admin action (architecture 9.3/12.4)." />;
  }

  async function run(username: string, action: () => Promise<QueuedUserCommandResponse>) {
    setBusyUsername(username);
    setError(null);
    try {
      const result = await action();
      onChanged();
      if (result.temporaryPassword) setRevealed(result);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Action failed.");
    } finally {
      setBusyUsername(null);
    }
  }

  async function handleAdd() {
    const role = VESSEL_ROLES.find((r) => r.label === addRole)?.value ?? VESSEL_ROLES[0].value;
    await run(addUsername, () => api.createVesselUser(vessel.id, addUsername, role));
    setAddOpen(false);
    setAddUsername("");
  }

  async function handleSetRole() {
    if (!roleTarget) return;
    const role = VESSEL_ROLES.find((r) => r.label === roleChoice)?.value ?? VESSEL_ROLES[0].value;
    await run(roleTarget.username, () => api.setVesselUserRole(vessel.id, roleTarget.username, role));
    setRoleTarget(null);
  }

  async function handleDeactivate() {
    if (!deactivateTarget) return;
    const username = deactivateTarget.username;
    setDeactivateTarget(null);
    await run(username, () => api.deactivateVesselUser(vessel.id, username));
  }

  if (revealed) {
    return (
      <Card variant="outlined" style={{ padding: 20, maxWidth: 480, display: "flex", flexDirection: "column", gap: 12 }}>
        <div className="md-title-medium">Temporary password — shown once</div>
        <AlertBanner
          level="caution"
          title="Relay this to the crew member now"
          message="This will not be shown again. Pass it on over the vessel's usual comms (satellite phone, email) — the account applies automatically on the vessel's next sync."
        />
        <Field label="Username" value={revealed.command.username} />
        <Field label="Temporary password" value={revealed.temporaryPassword ?? ""} />
        <div>
          <Button variant="filled" onClick={() => setRevealed(null)}>
            I've relayed this
          </Button>
        </div>
      </Card>
    );
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 16, maxWidth: 720 }}>
      <Card variant="outlined" style={{ padding: 20, display: "flex", flexDirection: "column", gap: 12 }}>
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
          <div className="md-title-medium">Crew accounts</div>
          <Button variant="filled" icon="person_add" onClick={() => setAddOpen(true)}>
            Add user
          </Button>
        </div>
        {error ? <AlertBanner level="warning" title="Action failed" message={error} onDismiss={() => setError(null)} /> : null}
        {vesselUsers.length === 0 ? (
          <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
            No roster reported yet — this vessel hasn't synced since enrolling, or hasn't enrolled yet.
          </div>
        ) : (
          <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
            {vesselUsers.map((u) => (
              <div key={u.username} style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: 12, padding: "8px 0", borderBottom: "1px solid var(--color-outline-variant)" }}>
                <div>
                  <div className="md-body-medium">
                    {u.username} <span style={{ color: "var(--color-on-surface-variant)" }}>· {vesselRoleLabel(u.role)}</span>
                  </div>
                  <div className="md-body-small" style={{ color: u.active ? "var(--color-status-underway)" : "var(--color-on-surface-variant)" }}>
                    {u.active ? "Active" : "Deactivated"}
                    {u.canSubmit ? " · can submit" : ""}
                  </div>
                </div>
                <div style={{ display: "flex", gap: 4, flexWrap: "wrap", justifyContent: "flex-end" }}>
                  <Button variant="text" size="small" disabled={busyUsername === u.username} onClick={() => void run(u.username, () => api.resetVesselUserPassword(vessel.id, u.username))}>
                    Reset password
                  </Button>
                  {u.role !== "master" ? (
                    <Button
                      variant="text"
                      size="small"
                      disabled={busyUsername === u.username}
                      onClick={() => {
                        setRoleChoice(vesselRoleLabel(u.role));
                        setRoleTarget(u);
                      }}
                    >
                      Change role
                    </Button>
                  ) : null}
                  {u.role !== "master" ? (
                    u.active ? (
                      <Button variant="text" size="small" disabled={busyUsername === u.username} onClick={() => setDeactivateTarget(u)}>
                        Deactivate
                      </Button>
                    ) : (
                      <Button variant="text" size="small" disabled={busyUsername === u.username} onClick={() => void run(u.username, () => api.reactivateVesselUser(vessel.id, u.username))}>
                        Reactivate
                      </Button>
                    )
                  ) : null}
                </div>
              </div>
            ))}
          </div>
        )}
      </Card>

      <Card variant="outlined" style={{ padding: 20, display: "flex", flexDirection: "column", gap: 12 }}>
        <div className="md-title-medium">Pending &amp; recent actions</div>
        {userCommands.length === 0 ? (
          <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
            No remote user actions have been issued for this vessel yet.
          </div>
        ) : (
          <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
            {userCommands.map((cmd) => (
              <div key={cmd.id} style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", gap: 12 }}>
                <div>
                  <div className="md-body-medium">
                    {userCommandLabel(cmd)} — {cmd.username}
                  </div>
                  <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)" }}>
                    {cmd.issuedBy} · {new Date(cmd.issuedAt).toLocaleString()}
                  </div>
                </div>
                <div className="md-body-small" style={{ color: cmd.appliedAt ? "var(--color-status-underway)" : "var(--color-on-surface-variant)", textAlign: "right", whiteSpace: "nowrap" }}>
                  {cmd.appliedAt ? "Applied" : cmd.fetchedAt ? "Delivered to vessel" : "Queued — awaiting next sync"}
                </div>
              </div>
            ))}
          </div>
        )}
      </Card>

      <Dialog
        open={addOpen}
        title="Add crew account"
        onClose={() => setAddOpen(false)}
        actions={[
          { label: "Cancel", onClick: () => setAddOpen(false) },
          { label: "Add", onClick: () => void handleAdd() },
        ]}
      >
        <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
          <TextField label="Username" value={addUsername} onChange={setAddUsername} />
          <Select label="Role" value={addRole} options={VESSEL_ROLES.map((r) => r.label)} onChange={setAddRole} />
          <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)" }}>
            Applies automatically on the vessel's next sync. A temporary password is generated and shown once —
            relay it to the crew member over the vessel's usual comms.
          </div>
        </div>
      </Dialog>

      <Dialog
        open={roleTarget !== null}
        title={roleTarget ? `Change ${roleTarget.username}'s role?` : "Change role?"}
        onClose={() => setRoleTarget(null)}
        actions={[
          { label: "Cancel", onClick: () => setRoleTarget(null) },
          { label: "Change", onClick: () => void handleSetRole() },
        ]}
      >
        <Select label="Role" value={roleChoice} options={VESSEL_ROLES.map((r) => r.label)} onChange={setRoleChoice} />
      </Dialog>

      <Dialog
        open={deactivateTarget !== null}
        title={deactivateTarget ? `Deactivate ${deactivateTarget.username}?` : "Deactivate?"}
        onClose={() => setDeactivateTarget(null)}
        actions={[
          { label: "Cancel", onClick: () => setDeactivateTarget(null) },
          { label: "Deactivate", onClick: () => void handleDeactivate() },
        ]}
      >
        This account will no longer be able to log in on the vessel, once applied on its next sync. This can be
        undone with Reactivate at any time.
      </Dialog>
    </div>
  );
}

function userCommandLabel(cmd: UserCommandView): string {
  switch (cmd.action) {
    case "create":
      return "Create account";
    case "resetPassword":
      return "Reset password";
    case "setRole":
      return `Set role to ${vesselRoleLabel(cmd.role ?? "")}`;
    case "setActive":
      return "Change active state";
    case "setCanSubmit":
      return "Change submit permission";
    default:
      return cmd.action;
  }
}

function ConfigTab({ detail }: { detail: VesselDetailView }) {
  const { bundleAssignment, vessel } = detail;
  if (!bundleAssignment) {
    return (
      <Card variant="outlined" style={{ padding: 20, maxWidth: 480 }}>
        <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
          No config bundle is assigned to this vessel or any of its groups yet. Bundle publishing and
          assignment is managed from Configuration.
        </div>
      </Card>
    );
  }
  // Applied-vs-assigned status (§3.3): the vessel reports which bundle it is
  // actually running on its SyncStatus check-in, so we can now tell whether
  // it is on its assigned config or hasn't pulled it yet — bundles are
  // immutable (a new bundle is a new id), so matching ids means current.
  const applied = vessel.appliedBundleId;
  const onAssigned = applied === bundleAssignment.bundleId;
  const appliedStatus = !applied
    ? { color: "var(--color-on-surface-variant)", text: "The vessel hasn't reported applying any config bundle yet." }
    : onAssigned
      ? {
          color: "var(--color-primary)",
          text: `Running the assigned bundle${vessel.appliedBundleVersion ? ` (v${vessel.appliedBundleVersion})` : ""}.`,
        }
      : {
          color: "var(--color-error)",
          text: `The vessel is running a different bundle${
            vessel.appliedBundleVersion ? ` (applied v${vessel.appliedBundleVersion})` : ""
          } — the assigned bundle hasn't been pulled and applied yet.`,
        };
  return (
    <Card variant="outlined" style={{ padding: 20, maxWidth: 480, display: "flex", flexDirection: "column", gap: 12 }}>
      <Field label="Assigned bundle" value={bundleAssignment.bundleLabel} />
      <Field label="Published" value={new Date(bundleAssignment.publishedAt).toLocaleString()} />
      <Field
        label="Assigned via"
        value={bundleAssignment.scopeType === "vessel" ? "Direct vessel assignment" : `Group: ${bundleAssignment.scopeKey}`}
      />
      <div className="md-body-medium" style={{ color: appliedStatus.color, fontWeight: 500 }}>{appliedStatus.text}</div>
    </Card>
  );
}

// This vessel's own reports (design handoff B2), scoped via the same
// ReportFilter.vesselId the main Reports explorer already supports —
// this data has been available since Phase 4/5 landed, the placeholder
// this replaced was just never wired to it. No row action: opening one
// specific report from here would need the same cross-section
// navigation lift NotificationPanel's own doc comment already flags as
// out of scope (no router yet) — the Reports section's own vesselId
// filter is the way to drill in for now.
function ReportsTab({ vesselId, onOpenReport }: { vesselId: string; onOpenReport: (reportId: string) => void }) {
  const [reports, setReports] = useState<ReportListItemView[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    setReports(null);
    api
      .listReports({ vesselId })
      .then((list) => {
        if (!cancelled) setReports(list ?? []);
      })
      .catch((err) => {
        if (!cancelled) setError(err instanceof Error ? err.message : "Could not load this vessel's reports.");
      });
    return () => {
      cancelled = true;
    };
  }, [vesselId]);

  if (error) {
    return <AlertBanner level="warning" title="Couldn't load reports" message={error} />;
  }
  if (reports === null) {
    return (
      <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
        Loading…
      </div>
    );
  }

  return (
    <DataTable
      emptyMessage="This vessel hasn't synced any reports yet."
      columns={[
        { key: "eventType", label: "Event type", type: "text", sortable: true },
        { key: "schemaName", label: "Schema", type: "text", filterable: true },
        {
          key: "state",
          label: "State",
          type: "badge",
          filterable: true,
          render: (row) => <StatePill state={row.state as ReportState} resubmitted={(row.versionNo as number) > 1} />,
        },
        { key: "health", label: "Health", type: "text", render: (row) => <HealthCell health={row.health as ReportListItemView["health"]} /> },
        {
          key: "eventTime",
          label: "Event time",
          type: "text",
          sortable: true,
          render: (row) => <span className="md-body-medium">{new Date(row.eventTime as string).toLocaleString()}</span>,
        },
      ]}
      rows={reports.map((r) => ({ id: `${r.reportId}:${r.versionNo}`, ...r }))}
      onRowAction={(row) => onOpenReport(row.reportId as string)}
    />
  );
}

function EmptyState({ icon, title, text }: { icon: string; title: string; text: string }) {
  return (
    <div style={{ padding: 48, display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", gap: 12, textAlign: "center" }}>
      <span className="material-symbols-rounded" style={{ fontSize: 40, color: "var(--color-on-surface-variant)" }}>
        {icon}
      </span>
      <div className="md-title-medium">{title}</div>
      <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)", maxWidth: 420 }}>
        {text}
      </div>
    </div>
  );
}

// B2's DR tab: architecture 12.5's restore bundle (reports, every
// version, audit trail, chat, and the vessel's currently-resolved config
// bundle — attachments are the one deliberate exclusion, see
// office/restorebundle.BuildBundle's own doc comment for why). Encrypted
// on generation against the vessel's own DR public key
// (enrollment.hasDRKey), minted fresh every time the vessel redeems an
// enrollment code — so a vessel that lost everything, including its old
// DR private key, recovers by re-enrolling first (Enrollment tab:
// revoke, re-issue, have the vessel redeem the new code) and only then
// pushing a bundle here, not the other way round. "Push to vessel"
// (architecture 11.2's PullInbox restore_commands) is the normal path —
// the vessel auto-fetches and applies it on its own next sync cycle, no
// manual file handling. The plain download link stays available as a
// fallback for the "any means, email included" out-of-band case 12.5
// itself describes, but importing a downloaded file still has no
// vessel-side screen (Master-only local HTTP endpoint).
function DRTab({ detail, isAdmin, onChanged }: { detail: VesselDetailView; isAdmin: boolean; onChanged: () => void }) {
  const { vessel, enrollment, restoreCommands } = detail;
  const [pushOpen, setPushOpen] = useState(false);
  const [reason, setReason] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  if (!isAdmin) {
    return <EmptyState icon="lock" title="Admin only" text="Restore-bundle generation is an Admin action (design handoff B2)." />;
  }

  if (!enrollment || !enrollment.hasDRKey) {
    return (
      <EmptyState
        icon="restore"
        title="No restore bundle available yet"
        text={
          enrollment && enrollment.state !== "notIssued"
            ? "This vessel hasn't (re-)redeemed its enrollment code since restore-bundle support landed, so office has no key to encrypt a bundle against yet. If the vessel lost its data, revoke and re-issue its enrollment on the Enrollment tab and have it redeem the new code — that mints a fresh restore key — then come back here."
            : "Issue this vessel an enrollment code (Enrollment tab) and have it redeemed before a restore bundle can be generated."
        }
      />
    );
  }

  async function push() {
    setBusy(true);
    setError(null);
    try {
      await api.pushRestoreBundle(vessel.id, reason);
      setPushOpen(false);
      setReason("");
      onChanged();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not push restore bundle.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 16, maxWidth: 640 }}>
      <Card variant="outlined" style={{ padding: 20, display: "flex", flexDirection: "column", gap: 12 }}>
        <div className="md-title-medium">Restore bundle</div>
        <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
          A point-in-time snapshot of this vessel's reports (every version), audit trail, chat, and
          currently-assigned config bundle (architecture 12.5). Encrypted against this vessel's own restore
          key — only that vessel can decrypt it.
        </div>
        {error ? <AlertBanner level="warning" title="Push failed" message={error} onDismiss={() => setError(null)} /> : null}
        <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
          <Button variant="filled" icon="cloud_upload" onClick={() => setPushOpen(true)}>
            Push to vessel
          </Button>
          <a href={api.restoreBundleUrl(vessel.id)} download={`${vessel.imo}-restore-bundle.age`}>
            <Button variant="outlined" icon="download">
              Download instead
            </Button>
          </a>
        </div>
      </Card>

      <Card variant="outlined" style={{ padding: 20, display: "flex", flexDirection: "column", gap: 12 }}>
        <div className="md-title-medium">Push history</div>
        {restoreCommands.length === 0 ? (
          <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
            No restore bundle has been pushed to this vessel yet.
          </div>
        ) : (
          <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
            {restoreCommands.map((cmd) => (
              <div key={cmd.id} style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", gap: 12 }}>
                <div>
                  <div className="md-body-medium">{cmd.reason}</div>
                  <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)" }}>
                    Pushed by {cmd.issuedBy} · {new Date(cmd.issuedAt).toLocaleString()}
                  </div>
                </div>
                <RestoreCommandStatus command={cmd} />
              </div>
            ))}
          </div>
        )}
      </Card>

      <Dialog
        open={pushOpen}
        title="Push restore bundle to vessel?"
        onClose={() => setPushOpen(false)}
        actions={[
          { label: "Cancel", onClick: () => setPushOpen(false) },
          { label: busy ? "Pushing…" : "Push", onClick: () => void push() },
        ]}
      >
        <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
          <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
            The vessel will fetch and apply this bundle automatically on its next sync — no action needed
            on the vessel beyond that. This can overwrite locally-drafted work at matching report versions,
            so use this for a vessel that has actually lost data, not a healthy one.
          </div>
          <TextField label="Reason (optional)" value={reason} onChange={setReason} supportingText="e.g. power outage — vessel re-enrolled" />
        </div>
      </Dialog>
    </div>
  );
}

function RestoreCommandStatus({ command }: { command: RestoreCommandView }) {
  if (command.appliedAt) {
    return (
      <div className="md-body-small" style={{ color: "var(--color-status-underway)", textAlign: "right", whiteSpace: "nowrap" }}>
        Applied
        <br />
        {new Date(command.appliedAt).toLocaleString()}
      </div>
    );
  }
  if (command.fetchedAt) {
    return (
      <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)", textAlign: "right", whiteSpace: "nowrap" }}>
        Delivered to vessel
        <br />
        {new Date(command.fetchedAt).toLocaleString()}
      </div>
    );
  }
  return (
    <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)", textAlign: "right", whiteSpace: "nowrap" }}>
      Queued — awaiting next vessel sync
    </div>
  );
}
