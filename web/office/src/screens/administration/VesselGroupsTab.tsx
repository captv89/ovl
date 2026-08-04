// SPDX-License-Identifier: AGPL-3.0-only

import { useEffect, useState } from "react";
import { DataTable } from "../../design/components/data/DataTable.jsx";
import { Card } from "../../design/components/surfaces/Card.jsx";
import { AlertBanner } from "../../design/components/feedback/AlertBanner.jsx";
import { Button } from "../../design/components/core/Button.jsx";
import { TextField } from "../../design/components/forms/TextField.jsx";
import { api, ApiError } from "../../api/client";

interface GroupRow {
  name: string;
  vesselCount: number;
}

// Design handoff B10's group management, layered directly on the
// existing free-form JSONB tag model (architecture 12.4) — there is no
// dedicated groups endpoint to list from, so the group catalog itself
// is derived the same way App.tsx's own global filter already derives
// it: the union of every vessel's Groups.
export function VesselGroupsTab() {
  const [groups, setGroups] = useState<GroupRow[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [renaming, setRenaming] = useState<string | null>(null);
  const [renameValue, setRenameValue] = useState("");
  const [busy, setBusy] = useState(false);

  function reload() {
    setError(null);
    api
      .listVessels()
      .then((vessels) => {
        const counts = new Map<string, number>();
        for (const v of vessels) for (const g of v.groups) counts.set(g, (counts.get(g) ?? 0) + 1);
        setGroups([...counts.entries()].map(([name, vesselCount]) => ({ name, vesselCount })).sort((a, b) => a.name.localeCompare(b.name)));
      })
      .catch((err) => setError(err instanceof Error ? err.message : "Could not load vessel groups."));
  }

  useEffect(reload, []);

  async function commitRename() {
    if (!renaming) return;
    setBusy(true);
    setError(null);
    try {
      await api.renameVesselGroup(renaming, renameValue);
      setRenaming(null);
      reload();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not rename this group.");
    } finally {
      setBusy(false);
    }
  }

  async function deleteGroup(name: string) {
    setBusy(true);
    setError(null);
    try {
      await api.deleteVesselGroup(name);
      reload();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not delete this group.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
      {error ? <AlertBanner level="warning" title="Couldn't complete that action" message={error} onDismiss={() => setError(null)} /> : null}

      {renaming ? (
        <Card variant="outlined" style={{ padding: 16, maxWidth: 400, display: "flex", flexDirection: "column", gap: 12 }}>
          <div className="md-title-medium">Rename "{renaming}"</div>
          <TextField label="New name" value={renameValue} onChange={setRenameValue} style={{ width: "100%" }} />
          <div style={{ display: "flex", gap: 8, justifyContent: "flex-end" }}>
            <Button variant="text" onClick={() => setRenaming(null)} disabled={busy}>
              Cancel
            </Button>
            <Button variant="filled" onClick={() => void commitRename()} disabled={busy || !renameValue.trim()}>
              Rename
            </Button>
          </div>
        </Card>
      ) : null}

      {!groups ? (
        <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
          Loading…
        </div>
      ) : (
        <DataTable
          columns={[
            { key: "name", label: "Group", type: "text", sortable: true },
            { key: "vesselCount", label: "Vessels", type: "number", sortable: true },
            {
              key: "actions",
              label: "",
              type: "iconText",
              render: (row) => (
                <div style={{ display: "flex", gap: 8 }} onClick={(e) => e.stopPropagation()}>
                  <Button
                    variant="text"
                    disabled={busy}
                    onClick={() => {
                      setRenaming(row.name as string);
                      setRenameValue(row.name as string);
                    }}
                  >
                    Rename
                  </Button>
                  <Button variant="text" disabled={busy} onClick={() => void deleteGroup(row.name as string)}>
                    Delete
                  </Button>
                </div>
              ),
            },
          ]}
          rows={groups.map((g) => ({ id: g.name, ...g }))}
          searchPlaceholder="Search groups"
          emptyMessage="No vessel groups yet — add group tags from a vessel's Profile tab."
        />
      )}
    </div>
  );
}
