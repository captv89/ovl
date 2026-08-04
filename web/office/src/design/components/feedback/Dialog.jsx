// SPDX-License-Identifier: AGPL-3.0-only
// FORKED COMPONENT: web/vessel and web/office each keep their own copy of this component. They are currently identical and a change here should be mirrored to the other app's copy — but do not assume they still match. See docs/codebase-audit-2026-07-22.md §6.

import React from "react";
import { createPortal } from "react-dom";

/**
 * Dialog — modal with scrim, title, body, actions.
 *
 * Renders through a portal at `position: fixed`. It used to be
 * `position: absolute; inset: 0`, which resolves against the nearest
 * positioned ancestor rather than the viewport: inside a tall scrolling
 * panel that centred the dialog in the *panel's* box, so it opened
 * somewhere down the page — invisible until you scrolled, and the scrim
 * covered only that ancestor instead of the app. Reported from live use on
 * the Bundles tab (2026-08-01), where "Publish this bundle?" appeared
 * mid-list and read as if the button had done nothing.
 *
 * Modal obligations that came with making it actually modal: Escape and
 * scrim-click dismiss, focus moved to the confirming action on open and
 * restored to the trigger on close, Tab trapped inside, and background
 * scroll locked.
 *
 * `actions` are ordered [dismissive…, confirming]; the last one is treated
 * as the primary and rendered filled, matching every call site's
 * [Cancel, Do-the-thing] shape. A lone action is its own primary.
 */
export function Dialog({ open, title, children, onClose, actions = [] }) {
  const surfaceRef = React.useRef(null);
  const restoreFocusRef = React.useRef(null);
  const titleId = React.useId();

  React.useEffect(() => {
    if (!open) return undefined;
    restoreFocusRef.current = document.activeElement;
    const { overflow } = document.body.style;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = overflow;
      // Only pull focus back if it is still loose inside the (now unmounted)
      // dialog, so a call site that deliberately moves focus on close wins.
      const el = restoreFocusRef.current;
      if (el && typeof el.focus === "function" && (document.activeElement === document.body || document.activeElement === null)) el.focus();
    };
  }, [open]);

  React.useEffect(() => {
    if (!open) return;
    // The confirming action, not the dismissive one — this is a confirm
    // dialog, and landing on Cancel would make Enter a silent no-op.
    const buttons = surfaceRef.current?.querySelectorAll("button");
    if (buttons?.length) buttons[buttons.length - 1].focus();
  }, [open]);

  function onKeyDown(e) {
    if (e.key === "Escape") {
      e.stopPropagation();
      if (onClose) onClose();
      return;
    }
    if (e.key !== "Tab") return;
    const focusable = surfaceRef.current?.querySelectorAll(
      'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])',
    );
    if (!focusable?.length) return;
    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    if (e.shiftKey && document.activeElement === first) {
      e.preventDefault();
      last.focus();
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault();
      first.focus();
    }
  }

  if (!open) return null;

  return createPortal(
    <div
      onMouseDown={(e) => {
        // Only a press that both starts and ends on the scrim dismisses —
        // otherwise a drag that happens to release outside the surface
        // (selecting the bundle label, say) would throw the dialog away.
        if (e.target === e.currentTarget && onClose) onClose();
      }}
      onKeyDown={onKeyDown}
      style={{
        position: "fixed",
        inset: 0,
        background: "color-mix(in oklab, var(--color-scrim) 55%, transparent)",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        padding: 16,
        zIndex: 1200,
      }}
    >
      <div
        ref={surfaceRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        style={{
          width: "min(100%, 420px)",
          maxHeight: "calc(100vh - 64px)",
          display: "flex",
          flexDirection: "column",
          background: "var(--color-surface-container-high)",
          borderRadius: "var(--shape-extra-large)",
          padding: 24,
          boxShadow: "var(--elevation-3)",
        }}
      >
        <div id={titleId} className="md-headline-small" style={{ fontFamily: "var(--font-brand)", marginBottom: 12 }}>{title}</div>
        <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)", marginBottom: 24, overflowY: "auto", flex: "0 1 auto" }}>{children}</div>
        <div style={{ display: "flex", justifyContent: "flex-end", gap: 8, flexShrink: 0 }}>
          {actions.map((a, i) => {
            const primary = i === actions.length - 1;
            return (
              <button
                key={i}
                type="button"
                onClick={a.onClick || onClose}
                style={{
                  border: "none",
                  background: primary ? "var(--color-primary)" : "transparent",
                  color: primary ? "var(--color-on-primary)" : "var(--color-on-surface-variant)",
                  fontWeight: 600,
                  fontFamily: "var(--font-body)",
                  padding: "10px 20px",
                  borderRadius: "var(--shape-full)",
                  cursor: "pointer",
                }}
              >
                {a.label}
              </button>
            );
          })}
        </div>
      </div>
    </div>,
    document.body,
  );
}
