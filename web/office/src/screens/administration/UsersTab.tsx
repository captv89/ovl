// SPDX-License-Identifier: AGPL-3.0-only

import { useEffect, useState } from "react";
import { DataTable } from "../../design/components/data/DataTable.jsx";
import { Card } from "../../design/components/surfaces/Card.jsx";
import { AlertBanner } from "../../design/components/feedback/AlertBanner.jsx";
import { Button } from "../../design/components/core/Button.jsx";
import { TextField } from "../../design/components/forms/TextField.jsx";
import { Checkbox } from "../../design/components/forms/Checkbox.jsx";
import { api, ApiError, type Role, type UserView } from "../../api/client";
import { ROLE_LABELS } from "../../roles";
import { RevealSecretCard } from "./RevealSecretCard";

const ALL_ROLES: Role[] = ["admin", "configManager", "commercialEditor", "reviewer", "viewer"];

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

function RoleCheckboxes({ selected, onChange }: { selected: Role[]; onChange: (roles: Role[]) => void }) {
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
      {ALL_ROLES.map((role) => (
        <Checkbox
          key={role}
          label={ROLE_LABELS[role]}
          checked={selected.includes(role)}
          onChange={(checked) => onChange(checked ? [...selected, role] : selected.filter((r) => r !== role))}
        />
      ))}
    </div>
  );
}

// Thin wrapper over the shared RevealSecretCard with this tab's own
// user-specific copy (see RevealSecretCard.tsx for why it's shared with
// Administration's API Access tab instead of duplicated).
function RevealPasswordCard({ username, password, onDone }: { username: string; password: string; onDone: () => void }) {
  return (
    <RevealSecretCard
      title="Temporary password — shown once"
      warningTitle="Save this password now"
      warningMessage="This password cannot be shown again. Share it with the user securely; they'll be required to change it on first login."
      fields={[
        { label: "Username", value: username },
        { label: "Temporary password", value: password },
      ]}
      onDone={onDone}
    />
  );
}

type View = { kind: "list" } | { kind: "create" } | { kind: "detail"; userId: string };

// Design handoff B10's Users tab: list every local account (username,
// roles, active state), provision new ones, reassign roles, deactivate/
// reactivate, and reset a forgotten password (fulfills Login.tsx's own
// existing "Ask an Admin to reset it" copy). Local accounts only — real
// OIDC (Authentik) is separate, larger infra work tracked elsewhere
// (see PROJECT.md's Office UI rework plan).
export function UsersTab() {
  const [view, setView] = useState<View>({ kind: "list" });
  const [users, setUsers] = useState<UserView[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  function reload() {
    setError(null);
    api
      .listUsers()
      .then(setUsers)
      .catch((err) => setError(err instanceof Error ? err.message : "Could not load users."));
  }

  useEffect(reload, []);

  if (view.kind === "create") {
    return <CreateUserView onCancel={() => setView({ kind: "list" })} onDone={() => { setView({ kind: "list" }); reload(); }} />;
  }
  if (view.kind === "detail") {
    const user = users?.find((u) => u.id === view.userId);
    if (!user) {
      return (
        <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
          Loading…
        </div>
      );
    }
    return (
      <UserDetailView
        user={user}
        onBack={() => setView({ kind: "list" })}
        onChanged={() => reload()}
      />
    );
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
      <div style={{ display: "flex", justifyContent: "flex-end" }}>
        <Button variant="filled" onClick={() => setView({ kind: "create" })}>
          New user
        </Button>
      </div>

      {error ? <AlertBanner level="warning" title="Couldn't load users" message={error} /> : null}

      {!users ? (
        <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
          Loading…
        </div>
      ) : (
        <DataTable
          columns={[
            { key: "username", label: "Username", type: "text", sortable: true },
            { key: "roles", label: "Roles", type: "text" },
            { key: "active", label: "Active", type: "symbol" },
            { key: "createdAt", label: "Created", type: "text", sortable: true },
          ]}
          rows={users.map((u) => ({
            id: u.id,
            username: u.username,
            roles: u.roles.map((r) => ROLE_LABELS[r]).join(", "),
            active: u.active
              ? { icon: "check_circle", label: "Active", color: "var(--color-status-underway)" }
              : { icon: "block", label: "Deactivated", color: "var(--color-on-surface-variant)" },
            createdAt: new Date(u.createdAt).toLocaleDateString(),
          }))}
          onRowAction={(row) => setView({ kind: "detail", userId: row.id as string })}
          searchPlaceholder="Search users"
          emptyMessage="No users yet."
        />
      )}
    </div>
  );
}

function CreateUserView({ onCancel, onDone }: { onCancel: () => void; onDone: () => void }) {
  const [username, setUsername] = useState("");
  const [roles, setRoles] = useState<Role[]>(["viewer"]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [created, setCreated] = useState<{ username: string; password: string } | null>(null);

  async function submit() {
    setBusy(true);
    setError(null);
    try {
      const resp = await api.createUser(username, roles);
      setCreated({ username: resp.user.username, password: resp.temporaryPassword });
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not create this user.");
    } finally {
      setBusy(false);
    }
  }

  if (created) {
    return <RevealPasswordCard username={created.username} password={created.password} onDone={onDone} />;
  }

  return (
    <Card variant="outlined" style={{ padding: 20, maxWidth: 400, display: "flex", flexDirection: "column", gap: 16 }}>
      <div className="md-title-medium">New user</div>
      {error ? <AlertBanner level="warning" title="Couldn't create user" message={error} onDismiss={() => setError(null)} /> : null}
      <TextField label="Username" value={username} onChange={setUsername} style={{ width: "100%" }} />
      <RoleCheckboxes selected={roles} onChange={setRoles} />
      <div style={{ display: "flex", gap: 8, justifyContent: "flex-end" }}>
        <Button variant="text" onClick={onCancel} disabled={busy}>
          Cancel
        </Button>
        <Button variant="filled" onClick={() => void submit()} disabled={busy || !username.trim() || roles.length === 0}>
          {busy ? "Creating…" : "Create"}
        </Button>
      </div>
    </Card>
  );
}

function UserDetailView({ user, onBack, onChanged }: { user: UserView; onBack: () => void; onChanged: () => void }) {
  const [roles, setRoles] = useState<Role[]>(user.roles);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [resetResult, setResetResult] = useState<string | null>(null);

  async function saveRoles() {
    setBusy(true);
    setError(null);
    try {
      await api.updateUserRoles(user.id, roles);
      onChanged();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not update roles.");
    } finally {
      setBusy(false);
    }
  }

  async function toggleActive() {
    setBusy(true);
    setError(null);
    try {
      if (user.active) await api.deactivateUser(user.id);
      else await api.reactivateUser(user.id);
      onChanged();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not update this user's active status.");
    } finally {
      setBusy(false);
    }
  }

  async function resetPassword() {
    setBusy(true);
    setError(null);
    try {
      const resp = await api.resetUserPassword(user.id);
      setResetResult(resp.temporaryPassword);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not reset this user's password.");
    } finally {
      setBusy(false);
    }
  }

  if (resetResult) {
    return (
      <RevealPasswordCard
        username={user.username}
        password={resetResult}
        onDone={() => {
          setResetResult(null);
          onChanged();
        }}
      />
    );
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
      <Button variant="text" icon="arrow_back" onClick={onBack}>
        Back to users
      </Button>
      <Card variant="outlined" style={{ padding: 20, maxWidth: 480, display: "flex", flexDirection: "column", gap: 16 }}>
        <div className="md-title-medium">{user.username}</div>
        {error ? <AlertBanner level="warning" title="Couldn't update user" message={error} onDismiss={() => setError(null)} /> : null}
        <Field label="Status" value={user.active ? "Active" : "Deactivated"} />
        <Field label="Created" value={new Date(user.createdAt).toLocaleString()} />
        <div>
          <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)", marginBottom: 4 }}>
            Roles
          </div>
          <RoleCheckboxes selected={roles} onChange={setRoles} />
          <div style={{ marginTop: 8 }}>
            <Button variant="outlined" onClick={() => void saveRoles()} disabled={busy || roles.length === 0}>
              Save roles
            </Button>
          </div>
        </div>
        <div style={{ display: "flex", gap: 8 }}>
          <Button variant={user.active ? "outlined" : "filled"} onClick={() => void toggleActive()} disabled={busy}>
            {user.active ? "Deactivate" : "Reactivate"}
          </Button>
          <Button variant="outlined" onClick={() => void resetPassword()} disabled={busy}>
            Reset password
          </Button>
        </div>
      </Card>
    </div>
  );
}
