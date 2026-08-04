// SPDX-License-Identifier: AGPL-3.0-only

import { useEffect, useState } from "react";
import { Button } from "../../design/components/core/Button.jsx";
import { AlertBanner } from "../../design/components/feedback/AlertBanner.jsx";
import { Dialog } from "../../design/components/feedback/Dialog.jsx";
import { api, ApiError, type Backup } from "../../api/client";
import { formatUtc } from "../../format";

// Design handoff A10's Backup section, scoped to what architecture 9.6
// actually specifies for level 1: "nightly SQLite snapshot... Snapshot
// now... restore from snapshot (guarded, two-step)." A10 also lists a
// configurable snapshot schedule/target path and "last 20 sync results" —
// out of scope here (no settings/config-bundle mechanism exists yet to
// hold a schedule, and there's no sync to report on until Phase 4); the
// schedule itself is a fixed 02:00 UTC in vessel/main.go, not editable
// from this screen yet.
//
// Formerly its own full-page screen (admin/BackupScreen.tsx); folded
// into a Settings section here (design handoff A10) since it's one of
// four settings groupings, not a destination that needs its own rail
// icon and "Back to Home" button. Its Dialog works unchanged because
// SettingsScreen mounts inside PageContainer, whose root is
// position:relative — Dialog anchors to the nearest positioned
// ancestor.
export function BackupSection() {
  const [backups, setBackups] = useState<Backup[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [snapshotting, setSnapshotting] = useState(false);
  const [restoreTarget, setRestoreTarget] = useState<Backup | null>(null);
  const [restoring, setRestoring] = useState(false);
  const [restoredMessage, setRestoredMessage] = useState<string | null>(null);

  async function refresh() {
    try {
      const list = await api.listBackups();
      setBackups(list);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not load backups.");
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  async function handleSnapshotNow() {
    setSnapshotting(true);
    setError(null);
    try {
      await api.snapshotNow();
      await refresh();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not create a snapshot.");
    } finally {
      setSnapshotting(false);
    }
  }

  async function handleRestore() {
    if (!restoreTarget) return;
    setRestoring(true);
    setError(null);
    try {
      await api.restoreBackup(restoreTarget.id);
      setRestoreTarget(null);
      setRestoredMessage(`Restored to the snapshot from ${formatUtc(restoreTarget.createdAt)}.`);
      await refresh();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not restore this backup.");
    } finally {
      setRestoring(false);
    }
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
      <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
        A nightly snapshot runs automatically at 02:00 UTC. Snapshots are stored locally, alongside the vessel's own data.
      </div>

      {error ? <AlertBanner level="warning" title="Something went wrong" message={error} /> : null}
      {restoredMessage ? <AlertBanner level="ok" title="Restore complete" message={restoredMessage} /> : null}

      <div>
        <Button variant="filled" disabled={snapshotting} onClick={() => void handleSnapshotNow()}>
          {snapshotting ? "Creating…" : "Snapshot now"}
        </Button>
      </div>

      {backups === null ? (
        <span className="md-body-large" style={{ color: "var(--color-on-surface-variant)" }}>Loading…</span>
      ) : backups.length === 0 ? (
        <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>No snapshots yet.</div>
      ) : (
        <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
          {backups.map((b) => (
            <div
              key={b.id}
              style={{
                display: "flex", justifyContent: "space-between", alignItems: "center",
                padding: "12px 16px", borderRadius: "var(--shape-small)", border: "1px solid var(--color-outline-variant)",
              }}
            >
              <span className="md-body-large" style={{ color: "var(--color-on-surface)" }}>{formatUtc(b.createdAt)}</span>
              <Button variant="outlined" onClick={() => setRestoreTarget(b)}>Restore</Button>
            </div>
          ))}
        </div>
      )}

      <Dialog
        open={restoreTarget !== null}
        title="Restore this snapshot?"
        onClose={() => !restoring && setRestoreTarget(null)}
        actions={[
          { label: "Cancel", onClick: () => setRestoreTarget(null) },
          { label: restoring ? "Restoring…" : "Restore", onClick: () => void handleRestore() },
        ]}
      >
        {restoreTarget ? (
          <>
            This replaces all current reports and data with the snapshot from {formatUtc(restoreTarget.createdAt)}.
            Anything entered after that point will be lost. This cannot be undone.
          </>
        ) : null}
      </Dialog>
    </div>
  );
}
