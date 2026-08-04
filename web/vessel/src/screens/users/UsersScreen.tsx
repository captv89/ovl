// SPDX-License-Identifier: AGPL-3.0-only

import { useEffect, useState } from "react";
import { Button } from "../../design/components/core/Button.jsx";
import { Switch } from "../../design/components/forms/Switch.jsx";
import { TextField } from "../../design/components/forms/TextField.jsx";
import { Select } from "../../design/components/forms/Select.jsx";
import { AlertBanner } from "../../design/components/feedback/AlertBanner.jsx";
import { Dialog } from "../../design/components/feedback/Dialog.jsx";
import { DataTable, type DataTableColumn } from "../../design/components/data/DataTable.jsx";
import { PageContainer } from "../PageContainer";
import { ROLE_LABELS } from "../account/roleLabels";
import { api, ApiError, type AdminUserView, type Role, type UserView } from "../../api/client";

const ASSIGNABLE_ROLES: Role[] = ["chiefOfficer", "secondOfficer", "thirdOfficer", "chiefEngineer", "secondEngineer"];

function formatUtc(iso: string): string {
  return iso.replace("T", " ").replace(/(:\d{2})(:\d{2})?\.?\d*Z?$/, "$1") + " UTC";
}

interface UserRow {
  id: string;
  username: string;
  isSelf: boolean;
  role: string;
  canSubmit: boolean;
  isMaster: boolean;
  active: boolean;
  createdAtDisplay: string;
}

function toRow(u: AdminUserView, selfId: string): UserRow {
  return {
    id: u.id,
    username: u.username,
    isSelf: u.id === selfId,
    role: ROLE_LABELS[u.role],
    canSubmit: u.canSubmit,
    isMaster: u.role === "master",
    active: u.active,
    createdAtDisplay: formatUtc(u.createdAt),
  };
}

// Design handoff A9, Master only (the rail already hides this destination
// for anyone else; the render-null guard below is defense in depth to
// match AppShell's own). The Master's own row (always the only "self"
// row, since only Master can reach this screen) never shows reset-
// password/deactivate — deactivating it is rejected server-side anyway
// (vessel/httpapi/users.go), and resetting your own password here would
// just duplicate the user-menu's self-service Change password.
export function UsersScreen({ user }: { user: UserView }) {
  const [users, setUsers] = useState<AdminUserView[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busyId, setBusyId] = useState<string | null>(null);

  const [addOpen, setAddOpen] = useState(false);
  const [addUsername, setAddUsername] = useState("");
  const [addRole, setAddRole] = useState<Role>(ASSIGNABLE_ROLES[0]);
  const [addError, setAddError] = useState<string | null>(null);
  const [addLoading, setAddLoading] = useState(false);

  const [tempPassword, setTempPassword] = useState<{ username: string; password: string } | null>(null);
  const [deactivateTarget, setDeactivateTarget] = useState<AdminUserView | null>(null);
  const [deactivating, setDeactivating] = useState(false);

  async function refresh() {
    try {
      const list = await api.listUsers();
      setUsers(list);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not load users.");
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  if (user.role !== "master") {
    return null;
  }

  async function toggleCanSubmit(u: AdminUserView) {
    setBusyId(u.id);
    setError(null);
    try {
      await api.updateUser(u.id, { canSubmit: !u.canSubmit });
      await refresh();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not update this user.");
    } finally {
      setBusyId(null);
    }
  }

  async function resetPassword(u: AdminUserView) {
    setBusyId(u.id);
    setError(null);
    try {
      const result = await api.resetUserPassword(u.id);
      setTempPassword({ username: u.username, password: result.temporaryPassword });
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not reset this user's password.");
    } finally {
      setBusyId(null);
    }
  }

  async function reactivate(u: AdminUserView) {
    setBusyId(u.id);
    setError(null);
    try {
      await api.updateUser(u.id, { active: true });
      await refresh();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not reactivate this user.");
    } finally {
      setBusyId(null);
    }
  }

  async function handleDeactivate() {
    if (!deactivateTarget) return;
    setDeactivating(true);
    setError(null);
    try {
      await api.updateUser(deactivateTarget.id, { active: false });
      setDeactivateTarget(null);
      await refresh();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not deactivate this user.");
    } finally {
      setDeactivating(false);
    }
  }

  async function handleAddUser() {
    setAddLoading(true);
    setAddError(null);
    try {
      const result = await api.createUser(addUsername.trim(), addRole);
      setAddOpen(false);
      setAddUsername("");
      setAddRole(ASSIGNABLE_ROLES[0]);
      setTempPassword({ username: result.user.username, password: result.temporaryPassword });
      await refresh();
    } catch (err) {
      setAddError(err instanceof ApiError ? err.message : "Could not create this user.");
    } finally {
      setAddLoading(false);
    }
  }

  const rows: UserRow[] = (users ?? []).map((u) => toRow(u, user.id));

  const columns: DataTableColumn[] = [
    {
      key: "username",
      label: "Username",
      type: "text",
      sortable: true,
      render: (row) => (
        <span style={{ display: "flex", alignItems: "center", gap: "var(--space-2)" }}>
          <span className="md-body-large" style={{ color: "var(--color-on-surface)" }}>{row.username}</span>
          {row.isSelf ? (
            <span className="md-label-small" style={{ color: "var(--color-on-surface-variant)" }}>(you)</span>
          ) : null}
        </span>
      ),
    },
    { key: "role", label: "Role", type: "text", sortable: true, filterable: true },
    {
      key: "canSubmit",
      label: "Can submit",
      type: "text",
      render: (row) => {
        const u = users?.find((x) => x.id === row.id);
        return (
          <Switch
            checked={row.isMaster ? true : row.canSubmit}
            disabled={row.isMaster || busyId === row.id}
            onChange={() => u && void toggleCanSubmit(u)}
          />
        );
      },
    },
    {
      key: "active",
      label: "Status",
      type: "text",
      filterable: true,
      render: (row) => (
        <span className="md-body-medium" style={{ color: row.active ? "var(--color-status-underway)" : "var(--color-on-surface-variant)" }}>
          {row.active ? "Active" : "Deactivated"}
        </span>
      ),
    },
    { key: "createdAtDisplay", label: "Created", type: "text", sortable: true },
    {
      key: "actions",
      label: "",
      type: "text",
      render: (row) => {
        if (row.isMaster) return null;
        const u = users?.find((x) => x.id === row.id);
        if (!u) return null;
        return (
          <div style={{ display: "flex", gap: "var(--space-2)", justifyContent: "flex-end" }}>
            <Button variant="text" size="small" disabled={busyId === u.id} onClick={() => void resetPassword(u)}>
              Reset password…
            </Button>
            {u.active ? (
              <Button variant="text" size="small" disabled={busyId === u.id} onClick={() => setDeactivateTarget(u)}>
                Deactivate
              </Button>
            ) : (
              <Button variant="text" size="small" disabled={busyId === u.id} onClick={() => void reactivate(u)}>
                Reactivate
              </Button>
            )}
          </div>
        );
      },
    },
  ];

  return (
    <PageContainer
      title="Users"
      actions={
        <Button variant="filled" icon="person_add" onClick={() => setAddOpen(true)}>
          Add user
        </Button>
      }
    >
      {error ? <AlertBanner level="warning" title="Something went wrong" message={error} /> : null}

      {users === null ? (
        <span className="md-body-large" style={{ color: "var(--color-on-surface-variant)" }}>Loading…</span>
      ) : (
        <DataTable columns={columns} rows={rows} searchPlaceholder="Search users…" emptyMessage="No users yet." />
      )}

      <Dialog
        open={addOpen}
        title="Add user"
        onClose={() => {
          if (addLoading) return;
          setAddOpen(false);
          setAddError(null);
        }}
        actions={[
          { label: "Cancel", onClick: () => setAddOpen(false) },
          { label: addLoading ? "Creating…" : "Add user", onClick: () => void handleAddUser() },
        ]}
      >
        <div style={{ display: "flex", flexDirection: "column", gap: 16, textAlign: "left" }}>
          <TextField label="Username" value={addUsername} onChange={setAddUsername} leadingIcon="badge" style={{ width: "100%" }} />
          <Select
            label="Role"
            value={ROLE_LABELS[addRole]}
            options={ASSIGNABLE_ROLES.map((r) => ROLE_LABELS[r])}
            onChange={(label: string) => {
              const role = ASSIGNABLE_ROLES.find((r) => ROLE_LABELS[r] === label);
              if (role) setAddRole(role);
            }}
            style={{ width: "100%" }}
          />
          {addError ? <AlertBanner level="warning" title="Couldn't add user" message={addError} /> : null}
        </div>
      </Dialog>

      <Dialog
        open={tempPassword !== null}
        title="Temporary password"
        onClose={() => setTempPassword(null)}
        actions={[{ label: "Done", onClick: () => setTempPassword(null) }]}
      >
        {tempPassword ? (
          <div style={{ display: "flex", flexDirection: "column", gap: 12, textAlign: "left" }}>
            <div>A temporary password was generated for <strong>{tempPassword.username}</strong> — shown only once, the user must change it at first sign-in.</div>
            <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
              <span className="md-title-medium" style={{ fontFamily: "var(--font-mono)", color: "var(--color-on-surface)" }}>{tempPassword.password}</span>
              <Button variant="text" size="small" icon="content_copy" onClick={() => void navigator.clipboard.writeText(tempPassword.password)}>
                Copy
              </Button>
            </div>
          </div>
        ) : null}
      </Dialog>

      <Dialog
        open={deactivateTarget !== null}
        title="Deactivate this user?"
        onClose={() => !deactivating && setDeactivateTarget(null)}
        actions={[
          { label: "Cancel", onClick: () => setDeactivateTarget(null) },
          { label: deactivating ? "Deactivating…" : "Deactivate", onClick: () => void handleDeactivate() },
        ]}
      >
        {deactivateTarget ? (
          <>{deactivateTarget.username} will no longer be able to sign in, and their current session will end immediately.</>
        ) : null}
      </Dialog>
    </PageContainer>
  );
}
