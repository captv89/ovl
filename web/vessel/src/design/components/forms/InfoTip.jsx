// SPDX-License-Identifier: AGPL-3.0-only

import React from "react";
import { Tooltip } from "../feedback/Tooltip.jsx";

/**
 * InfoTip — the always-visible ⓘ next to a field label (the OVL Vessel
 * Wireframes' `.fi` element), showing a plain-language definition on
 * hover/focus. Distinct from the floating-label tooltip it replaces
 * across the field primitives: this is reachable regardless of whether
 * the field has a value or is focused — the old pattern only worked once
 * a label had floated, which made an empty, unfocused field's own
 * description unreachable, so that pattern was dropped.
 */
export function InfoTip({ label, maxWidth = 220 }) {
  return (
    <Tooltip label={label} maxWidth={maxWidth} delay={150}>
      <span
        // Not in the Tab sequence (2026-07-14 manual-test feedback:
        // pressing Tab through a long form landed on each field's info
        // icon instead of advancing to the next field). Still reachable
        // by mouse hover; the field's own description text is exposed to
        // screen readers separately, not solely through this tooltip.
        tabIndex={-1}
        role="img"
        aria-label="Field info"
        style={{
          display: "inline-flex",
          alignItems: "center",
          justifyContent: "center",
          width: 13,
          height: 13,
          flexShrink: 0,
          fontSize: 9,
          fontWeight: 700,
          fontStyle: "italic",
          borderRadius: "50%",
          border: "1px solid var(--color-outline)",
          color: "var(--color-on-surface-variant)",
          cursor: "help",
        }}
      >
        i
      </span>
    </Tooltip>
  );
}
