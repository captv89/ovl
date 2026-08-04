// SPDX-License-Identifier: AGPL-3.0-only

import React from "react";

function initialsFor(username) {
  const words = username.trim().split(/\s+/).filter(Boolean);
  if (words.length >= 2) return (words[0][0] + words[1][0]).toUpperCase();
  return (username.slice(0, 2) || "?").toUpperCase();
}

/**
 * UserMenu — a circular-avatar trigger (initials from `username`) that
 * opens an anchored popover: an identity block (username + subtitle,
 * e.g. a role label) above a list of action items. Generic account/
 * identity affordance — not tied to any one app's user model beyond
 * "has a username and a list of things you can do."
 */
export function UserMenu({ username, subtitle, items = [], style }) {
  const [open, setOpen] = React.useState(false);
  const containerRef = React.useRef(null);

  React.useEffect(() => {
    if (!open) return undefined;
    function onDocMouseDown(e) {
      if (containerRef.current && !containerRef.current.contains(e.target)) setOpen(false);
    }
    function onKeyDown(e) {
      if (e.key === "Escape") setOpen(false);
    }
    document.addEventListener("mousedown", onDocMouseDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("mousedown", onDocMouseDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [open]);

  return (
    <div ref={containerRef} style={{ position: "relative", ...style }}>
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-label={`Account menu for ${username}`}
        style={{
          width: 36,
          height: 36,
          borderRadius: "var(--shape-full)",
          border: "none",
          cursor: "pointer",
          background: "var(--color-primary-container)",
          color: "var(--color-on-primary-container)",
          fontFamily: "var(--font-body)",
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
        }}
      >
        <span className="md-label-large">{initialsFor(username)}</span>
      </button>
      {open ? (
        <div
          role="menu"
          style={{
            position: "absolute",
            top: "calc(100% + 4px)",
            right: 0,
            minWidth: 220,
            zIndex: 30,
            background: "var(--color-surface-container-high)",
            borderRadius: "var(--shape-medium)",
            boxShadow: "var(--elevation-2)",
            overflow: "hidden",
          }}
        >
          <div style={{ padding: "var(--space-4)" }}>
            <div className="md-title-small" style={{ color: "var(--color-on-surface)", whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>{username}</div>
            {subtitle ? (
              <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)" }}>{subtitle}</div>
            ) : null}
          </div>
          <div style={{ borderTop: "1px solid var(--color-outline-variant)" }} />
          <div style={{ padding: "var(--space-2) 0" }}>
            {items.map((item, i) => (
              <button
                key={i}
                type="button"
                role="menuitem"
                onClick={() => {
                  setOpen(false);
                  item.onClick && item.onClick();
                }}
                style={{
                  width: "100%",
                  display: "flex",
                  alignItems: "center",
                  gap: "var(--space-3)",
                  padding: "var(--space-2) var(--space-4)",
                  border: "none",
                  background: "none",
                  cursor: "pointer",
                  textAlign: "left",
                  font: "inherit",
                  color: "var(--color-on-surface)",
                }}
              >
                {item.icon ? <span className="material-symbols-rounded" style={{ fontSize: 20, color: "var(--color-on-surface-variant)" }}>{item.icon}</span> : null}
                <span className="md-body-large">{item.label}</span>
              </button>
            ))}
          </div>
        </div>
      ) : null}
    </div>
  );
}
