// SPDX-License-Identifier: AGPL-3.0-only
// FORKED COMPONENT: web/vessel and web/office each keep their own copy of this component and they are intentionally NOT identical (vessel wireframe rework, 2026-07-13). A change here likely needs mirroring to the other app's copy — do not assume they match. See docs/codebase-audit-2026-07-22.md §6.

import React from "react";
import { createPortal } from "react-dom";

/**
 * Tooltip — hover/focus label for a wrapped trigger. Gained keyboard
 * support (focus/blur, not just mouse) and an optional `maxWidth` for
 * longer content: without it, long help text (e.g. a field's full
 * mandatory-note explanation) would render as one unreadable nowrap line
 * off the edge of the screen instead of wrapping. `delay` (ms) defers
 * showing on hover — a hover-intent guard so a cursor merely passing
 * over the trigger doesn't flash a tooltip; 0 (default) keeps the
 * original instant behavior for existing call sites. Focus always opens
 * immediately regardless of `delay`.
 *
 * The bubble renders through a portal into `document.body` at
 * `position: fixed`, anchored to the trigger's own `getBoundingClientRect`
 * — not `position: absolute` inside the trigger's own stacking context,
 * which clips against any scrollable ancestor no matter its z-index.
 * Forward-ported from the vessel app's same fix (2026-07-13 manual-test
 * feedback: tooltips clipped by scroll containers and rendering in the
 * M3 "inverse surface" pair instead of this app's normal floating-surface
 * tokens). Closes on scroll/resize instead of continuously repositioning
 * — a hover-triggered bubble whose anchor just moved out from under the
 * cursor has already lost its reason to stay open.
 */
export function Tooltip({ children, label, maxWidth, delay = 0 }) {
  const [show, setShow] = React.useState(false);
  const [coords, setCoords] = React.useState(null);
  const timerRef = React.useRef(null);
  const anchorRef = React.useRef(null);

  function clearTimer() {
    if (timerRef.current !== null) {
      window.clearTimeout(timerRef.current);
      timerRef.current = null;
    }
  }
  function openOnHover() {
    if (delay > 0) {
      timerRef.current = window.setTimeout(() => setShow(true), delay);
    } else {
      setShow(true);
    }
  }
  function close() {
    clearTimer();
    setShow(false);
  }

  React.useEffect(() => clearTimer, []);

  React.useLayoutEffect(() => {
    if (!show || !anchorRef.current) return undefined;
    const rect = anchorRef.current.getBoundingClientRect();
    setCoords({ left: rect.left + rect.width / 2, top: rect.top });
    window.addEventListener("scroll", close, true);
    window.addEventListener("resize", close);
    return () => {
      window.removeEventListener("scroll", close, true);
      window.removeEventListener("resize", close);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [show]);

  const openProps = {
    onMouseEnter: openOnHover,
    onMouseLeave: close,
    onFocus: () => setShow(true),
    onBlur: close,
  };
  return (
    <span ref={anchorRef} style={{ position: "relative", display: "inline-block" }} {...openProps}>
      {children}
      {show && coords
        ? createPortal(
            <>
              <style>{"@keyframes ovl-tooltip-in { from { opacity: 0; transform: translate(-50%, 3px); } to { opacity: 1; transform: translate(-50%, 0); } }"}</style>
              <span
                role="tooltip"
                style={{
                  position: "fixed",
                  top: coords.top,
                  left: coords.left,
                  transform: "translate(-50%, calc(-100% - 8px))",
                  background: "var(--color-surface-container-highest)",
                  color: "var(--color-on-surface)",
                  border: "1px solid var(--color-outline-variant)",
                  fontSize: 12,
                  lineHeight: 1.4,
                  padding: "6px 10px",
                  borderRadius: "var(--shape-extra-small)",
                  boxShadow: "var(--elevation-2)",
                  whiteSpace: maxWidth ? "pre-line" : "nowrap",
                  width: maxWidth ? maxWidth : undefined,
                  textAlign: maxWidth ? "left" : "center",
                  zIndex: 1000,
                  animation: "ovl-tooltip-in 120ms ease",
                  animationFillMode: "forwards",
                }}
              >
                {label}
              </span>
            </>,
            document.body,
          )
        : null}
    </span>
  );
}
