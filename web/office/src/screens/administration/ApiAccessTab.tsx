// SPDX-License-Identifier: AGPL-3.0-only

import { useEffect, useState } from "react";
import { DataTable } from "../../design/components/data/DataTable.jsx";
import { Card } from "../../design/components/surfaces/Card.jsx";
import { IconButton } from "../../design/components/core/IconButton.jsx";
import { AlertBanner } from "../../design/components/feedback/AlertBanner.jsx";
import { Button } from "../../design/components/core/Button.jsx";
import { TextField } from "../../design/components/forms/TextField.jsx";
import { api, ApiError, type ApiKeyEventView, type ApiKeyView } from "../../api/client";
import { RevealSecretCard } from "./RevealSecretCard";

type View = { kind: "list" } | { kind: "create" };

const EVENT_LABEL: Record<ApiKeyEventView["kind"], string> = {
  created: "Key created",
  revoked: "Key revoked",
  usedGraphQL: "Used to query the GraphQL API",
  usedCSV: "Used for a CSV export",
};

const EVENT_ICON: Record<ApiKeyEventView["kind"], string> = {
  created: "add_circle",
  revoked: "block",
  usedGraphQL: "sync_alt",
  usedCSV: "sync_alt",
};

// Administration > API Access (architecture 13.1): admin-issued bearer
// keys for the external data API (GraphQL + CSV, Phase 6) — a separate
// credential from office staff's own session-cookie login, never usable
// to sign into this UI. Same list/create-view shape as UsersTab, since
// issuing a key is exactly as privileged an act as provisioning a local
// account.
//
// 2026-07-25 redesign added Delete (only once a key is revoked — the
// backend 409s otherwise, so it's not even offered here for an active
// key) and a per-key activity-log side panel.
export function ApiAccessTab() {
  const [view, setView] = useState<View>({ kind: "list" });
  const [keys, setKeys] = useState<ApiKeyView[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [logKeyId, setLogKeyId] = useState<string | null>(null);
  const [events, setEvents] = useState<ApiKeyEventView[] | null>(null);

  function reload() {
    setError(null);
    api
      .listApiKeys()
      .then(setKeys)
      .catch((err) => setError(err instanceof Error ? err.message : "Could not load API keys."));
  }

  useEffect(reload, []);

  useEffect(() => {
    if (!logKeyId) {
      setEvents(null);
      return;
    }
    setEvents(null);
    api
      .listApiKeyEvents(logKeyId)
      .then(setEvents)
      .catch((err) => setError(err instanceof ApiError ? err.message : "Could not load this key's activity log."));
  }, [logKeyId]);

  async function revoke(id: string) {
    try {
      await api.revokeApiKey(id);
      reload();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not revoke this API key.");
    }
  }

  async function remove(id: string) {
    try {
      await api.deleteApiKey(id);
      if (logKeyId === id) setLogKeyId(null);
      reload();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not delete this API key.");
    }
  }

  if (view.kind === "create") {
    return <CreateApiKeyView onCancel={() => setView({ kind: "list" })} onDone={() => { setView({ kind: "list" }); reload(); }} />;
  }

  const selectedKey = keys?.find((k) => k.id === logKeyId) ?? null;

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
      <div style={{ display: "flex", justifyContent: "flex-end" }}>
        <Button variant="filled" onClick={() => setView({ kind: "create" })}>
          New API key
        </Button>
      </div>

      {error ? <AlertBanner level="warning" title="Something went wrong" message={error} onDismiss={() => setError(null)} /> : null}

      {!keys ? (
        <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
          Loading…
        </div>
      ) : (
        <div style={{ display: "flex", gap: 16, alignItems: "flex-start" }}>
          <div style={{ flex: logKeyId ? "0 0 73%" : "1 1 100%", minWidth: 0 }}>
            <DataTable
              columns={[
                { key: "label", label: "Label", type: "text", sortable: true },
                { key: "groupId", label: "Group", type: "text" },
                { key: "createdBy", label: "Created by", type: "text" },
                { key: "createdAt", label: "Created", type: "text", sortable: true },
                { key: "lastUsedAt", label: "Last used", type: "text" },
                {
                  key: "status",
                  label: "Status",
                  type: "text",
                  filterable: true,
                  render: (row) => {
                    const active = row.status === "active";
                    return (
                      <span
                        className="md-label-medium"
                        style={{
                          display: "inline-flex", alignItems: "center", gap: 6, padding: "4px 10px",
                          borderRadius: "var(--shape-full)", fontWeight: 700, fontSize: 12,
                          background: active ? "var(--color-status-underway-container)" : "var(--color-surface-container-highest)",
                          color: active ? "var(--color-status-underway)" : "var(--color-on-surface-variant)",
                        }}
                      >
                        <span style={{ width: 6, height: 6, borderRadius: "50%", background: "currentColor", display: "inline-block" }} />
                        {active ? "Active" : "Revoked"}
                      </span>
                    );
                  },
                },
                {
                  key: "actions",
                  label: "",
                  type: "text",
                  render: (row) => (
                    <div style={{ display: "flex", justifyContent: "flex-end", gap: 2 }}>
                      {row.status === "active" ? (
                        <IconButton icon="block" size="small" aria-label="Revoke" style={{ color: "var(--color-error)" }} onClick={() => void revoke(row.id as string)} />
                      ) : null}
                      {row.status === "revoked" ? (
                        <IconButton icon="delete" size="small" aria-label="Delete" onClick={() => void remove(row.id as string)} />
                      ) : null}
                      <IconButton
                        icon="history"
                        size="small"
                        aria-label="Activity log"
                        style={logKeyId === row.id ? { color: "var(--color-primary)", background: "var(--color-primary-container)" } : undefined}
                        onClick={() => setLogKeyId((cur) => (cur === row.id ? null : (row.id as string)))}
                      />
                    </div>
                  ),
                },
              ]}
              rows={keys.map((k) => ({
                id: k.id,
                label: k.label,
                groupId: k.groupId ?? "All vessels",
                createdBy: k.createdBy,
                createdAt: new Date(k.createdAt).toLocaleString(),
                lastUsedAt: k.lastUsedAt ? new Date(k.lastUsedAt).toLocaleString() : "Never",
                status: k.revokedAt ? "revoked" : "active",
              }))}
              searchPlaceholder="Search API keys"
              emptyMessage="No API keys yet."
            />
          </div>

          {logKeyId && selectedKey ? (
            <Card variant="outlined" style={{ flex: "1 1 27%", minWidth: 260, padding: 0, display: "flex", flexDirection: "column", maxHeight: 520 }}>
              <div style={{ padding: "14px 18px", borderBottom: "1px solid var(--color-outline-variant)", display: "flex", alignItems: "center", justifyContent: "space-between", gap: 8 }}>
                <div style={{ minWidth: 0 }}>
                  <div className="md-title-small" style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{selectedKey.label}</div>
                  <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)" }}>Activity log</div>
                </div>
                <IconButton icon="close" size="small" aria-label="Close" onClick={() => setLogKeyId(null)} />
              </div>
              <div style={{ padding: 18, overflowY: "auto", flex: 1 }}>
                {events === null ? (
                  <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
                    Loading…
                  </div>
                ) : events.length === 0 ? (
                  <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
                    No activity recorded yet.
                  </div>
                ) : (
                  events.map((e, i) => (
                    <div key={i} style={{ display: "flex", gap: 12, position: "relative", paddingBottom: 20 }}>
                      <div style={{ display: "flex", flexDirection: "column", alignItems: "center", flexShrink: 0 }}>
                        <span
                          style={{
                            width: 28, height: 28, borderRadius: "50%", background: "var(--color-secondary-container)", color: "var(--color-on-secondary-container)",
                            display: "flex", alignItems: "center", justifyContent: "center",
                          }}
                        >
                          <span className="material-symbols-rounded" style={{ fontSize: 15 }}>{EVENT_ICON[e.kind]}</span>
                        </span>
                        {i < events.length - 1 ? <span style={{ width: 1, flex: 1, background: "var(--color-outline-variant)", marginTop: 4 }} /> : null}
                      </div>
                      <div style={{ paddingTop: 3 }}>
                        <div className="md-body-medium">{EVENT_LABEL[e.kind]}</div>
                        <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)", fontFamily: "var(--font-mono)" }}>
                          {new Date(e.at).toLocaleString()}
                        </div>
                      </div>
                    </div>
                  ))
                )}
              </div>
            </Card>
          ) : null}
        </div>
      )}
    </div>
  );
}

function CreateApiKeyView({ onCancel, onDone }: { onCancel: () => void; onDone: () => void }) {
  const [label, setLabel] = useState("");
  const [groupId, setGroupId] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [created, setCreated] = useState<{ label: string; token: string } | null>(null);

  async function submit() {
    setBusy(true);
    setError(null);
    try {
      const resp = await api.createApiKey(label, groupId.trim() || null);
      setCreated({ label: resp.apiKey.label, token: resp.token });
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not create this API key.");
    } finally {
      setBusy(false);
    }
  }

  if (created) {
    return (
      <RevealSecretCard
        title="API key — shown once"
        warningTitle="Save this key now"
        warningMessage="This key cannot be shown again. It's a permanent bearer credential — anyone holding it can query the data API until it's revoked."
        fields={[
          { label: "Label", value: created.label },
          { label: "API key", value: created.token },
        ]}
        onDone={onDone}
      />
    );
  }

  return (
    <Card variant="outlined" style={{ padding: 20, maxWidth: 400, display: "flex", flexDirection: "column", gap: 16 }}>
      <div className="md-title-medium">New API key</div>
      {error ? <AlertBanner level="warning" title="Couldn't create API key" message={error} onDismiss={() => setError(null)} /> : null}
      <TextField label="Label (who is this for?)" value={label} onChange={setLabel} style={{ width: "100%" }} />
      <TextField
        label="Vessel group (optional)"
        value={groupId}
        onChange={setGroupId}
        supportingText="Leave blank to scope this key to every vessel."
        style={{ width: "100%" }}
      />
      <div style={{ display: "flex", gap: 8, justifyContent: "flex-end" }}>
        <Button variant="text" onClick={onCancel} disabled={busy}>
          Cancel
        </Button>
        <Button variant="filled" onClick={() => void submit()} disabled={busy || !label.trim()}>
          {busy ? "Creating…" : "Create"}
        </Button>
      </div>
    </Card>
  );
}
