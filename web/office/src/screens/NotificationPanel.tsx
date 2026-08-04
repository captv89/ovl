// SPDX-License-Identifier: AGPL-3.0-only

import { useEffect, useState } from "react";
import { api, type NotificationView } from "../api/client";
import type { Section } from "./AppShell";

const CATEGORY_ICON: Record<NotificationView["category"], string> = {
  overdue: "warning",
  remark: "forum",
  sync: "cloud_done",
};

const CATEGORY_COLOR: Record<NotificationView["category"], string> = {
  overdue: "var(--color-error)",
  remark: "var(--color-status-caution)",
  sync: "var(--color-status-underway)",
};

const FILTERS = ["All", "Unread", "Overdue", "Remarks", "Sync"] as const;
type Filter = (typeof FILTERS)[number];

function matchesFilter(n: NotificationView, filter: Filter): boolean {
  switch (filter) {
    case "All":
      return true;
    case "Unread":
      return !n.read;
    case "Overdue":
      return n.category === "overdue";
    case "Remarks":
      return n.category === "remark";
    case "Sync":
      return n.category === "sync";
  }
}

// Design handoff B1·N's notification panel — a bell in the top bar with
// an unread count, opening a filterable recency feed. Deliberately
// scoped down from the wireframe in one respect: rows deep-link to the
// relevant *section* (Vessels/Reports) via the same onNavigate callback
// Dashboard's "View all" links use, not to the specific vessel/report —
// opening one specific record from outside that screen's own internal
// navigation state would need a real router or a larger lift of
// navigation state into App.tsx, out of scope for this pass (see
// PROJECT.md's Office UI rework, Phase O6). There is also no separate
// "View all notifications" full-history screen — this panel's own list
// (capped server-side at 50, most-recent-first) is the whole surface
// for now.
export function NotificationPanel({ onNavigate }: { onNavigate: (section: Section) => void }) {
  const [open, setOpen] = useState(false);
  const [notifications, setNotifications] = useState<NotificationView[] | null>(null);
  const [filter, setFilter] = useState<Filter>("All");

  const reload = () => {
    api
      .listNotifications()
      .then(setNotifications)
      .catch(() => {
        // Best-effort — the bell just shows nothing rather than an error banner.
      });
  };

  useEffect(reload, []);
  // Light polling while the panel is closed keeps the unread badge fresh
  // without needing a websocket/SSE channel for a v1 notification feed.
  useEffect(() => {
    const id = window.setInterval(reload, 60_000);
    return () => window.clearInterval(id);
  }, []);

  const unreadCount = notifications?.filter((n) => !n.read).length ?? 0;
  const filtered = (notifications ?? []).filter((n) => matchesFilter(n, filter));

  async function markAllRead() {
    const unreadIds = (notifications ?? []).filter((n) => !n.read).map((n) => n.id);
    if (unreadIds.length === 0) return;
    await api.markNotificationsRead(unreadIds);
    reload();
  }

  async function handleRowClick(n: NotificationView) {
    if (!n.read) await api.markNotificationsRead([n.id]);
    setOpen(false);
    if (n.link) onNavigate(n.link.section);
    reload();
  }

  return (
    <div style={{ position: "relative" }}>
      <button
        onClick={() => setOpen((v) => !v)}
        aria-label="Notifications"
        style={{
          position: "relative", width: 40, height: 40, border: "none", background: "transparent",
          borderRadius: "var(--shape-full)", cursor: "pointer", display: "flex", alignItems: "center", justifyContent: "center",
          color: "var(--color-on-surface-variant)",
        }}
      >
        <span className="material-symbols-rounded">notifications</span>
        {unreadCount > 0 ? (
          <span
            className="md-label-small"
            style={{
              position: "absolute", top: 2, right: 2, minWidth: 15, height: 15, padding: "0 3px",
              borderRadius: "var(--shape-full)", background: "var(--color-error)", color: "var(--color-on-error)",
              fontSize: 9, fontWeight: 700, display: "flex", alignItems: "center", justifyContent: "center",
            }}
          >
            {unreadCount > 9 ? "9+" : unreadCount}
          </span>
        ) : null}
      </button>

      {open ? (
        <>
          <div onClick={() => setOpen(false)} style={{ position: "fixed", inset: 0, zIndex: 20 }} />
          <div
            style={{
              position: "absolute", top: "100%", right: 0, marginTop: 4, zIndex: 21,
              width: 400, maxHeight: 480, display: "flex", flexDirection: "column", overflow: "hidden",
              background: "var(--color-surface-container-high)", borderRadius: "var(--shape-medium)", boxShadow: "var(--elevation-2)",
            }}
          >
            <div style={{ display: "flex", alignItems: "center", gap: 8, padding: "13px 16px", borderBottom: "1px solid var(--color-outline-variant)" }}>
              <span className="md-title-medium">Notifications</span>
              {unreadCount > 0 ? (
                <span className="md-label-small" style={{ background: "var(--color-secondary-container)", color: "var(--color-on-secondary-container)", borderRadius: "var(--shape-full)", padding: "1px 7px" }}>
                  {unreadCount} new
                </span>
              ) : null}
              <span style={{ flex: 1 }} />
              <button
                onClick={() => void markAllRead()}
                disabled={unreadCount === 0}
                className="md-body-small"
                style={{ border: "none", background: "transparent", color: unreadCount === 0 ? "var(--color-on-surface-variant)" : "var(--color-primary)", cursor: unreadCount === 0 ? "default" : "pointer" }}
              >
                Mark all read
              </button>
            </div>

            <div style={{ display: "flex", gap: 6, padding: "10px 16px 6px", flexWrap: "wrap" }}>
              {FILTERS.map((f) => (
                <button
                  key={f}
                  onClick={() => setFilter(f)}
                  className="md-label-medium"
                  style={{
                    height: 26, padding: "0 12px", borderRadius: "var(--shape-full)", cursor: "pointer",
                    border: `1px solid ${filter === f ? "var(--color-secondary)" : "var(--color-outline)"}`,
                    background: filter === f ? "var(--color-secondary-container)" : "transparent",
                    color: filter === f ? "var(--color-on-secondary-container)" : "var(--color-on-surface)",
                  }}
                >
                  {f}
                </button>
              ))}
            </div>

            <div style={{ overflowY: "auto", flex: 1 }}>
              {filtered.length === 0 ? (
                <div className="md-body-medium" style={{ padding: 24, textAlign: "center", color: "var(--color-on-surface-variant)" }}>
                  {notifications === null ? "Loading…" : "Nothing here."}
                </div>
              ) : (
                filtered.map((n) => (
                  <button
                    key={n.id}
                    onClick={() => void handleRowClick(n)}
                    style={{
                      display: "flex", gap: 11, width: "100%", padding: "11px 16px", border: "none",
                      borderTop: "1px solid var(--color-outline-variant)", background: "transparent",
                      cursor: "pointer", textAlign: "left", font: "inherit",
                    }}
                  >
                    <span className="material-symbols-rounded" style={{ fontSize: 20, color: CATEGORY_COLOR[n.category], flexShrink: 0, marginTop: 1 }}>
                      {CATEGORY_ICON[n.category]}
                    </span>
                    <div style={{ flex: 1, minWidth: 0 }}>
                      <div className="md-body-medium" style={{ color: "var(--color-on-surface)" }}>{n.title}</div>
                      <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)" }}>{n.message}</div>
                      <div className="md-label-small" style={{ color: "var(--color-on-surface-variant)", marginTop: 2 }}>
                        {new Date(n.at).toLocaleString()}
                      </div>
                    </div>
                    {!n.read ? (
                      <span style={{ width: 8, height: 8, borderRadius: "50%", background: "var(--color-primary)", flexShrink: 0, marginTop: 6 }} />
                    ) : null}
                  </button>
                ))
              )}
            </div>
          </div>
        </>
      ) : null}
    </div>
  );
}
