// SPDX-License-Identifier: AGPL-3.0-only

import React from "react";
import { FieldShell, fieldFrameStyle } from "./FieldShell.jsx";
import { Switch } from "./Switch.jsx";

/**
 * BooleanField — a Switch on FieldShell, matching every other field
 * primitive's label-above/bordered-frame anatomy instead of a bespoke
 * label-left/switch-right row. FieldRow.tsx's boolean branch used to
 * build that row by hand — border-color cascade unified onto
 * `fieldFrameStyle` back in the Phase 1 rework, but never actually
 * moved onto `FieldShell` itself, so it kept reading as a completely
 * different control next to every text/select/date field around it
 * (2026-07-13 manual-test feedback: "the radio button style sucks...
 * completely out of place... not in line with the text input field").
 */
export function BooleanField({
  label,
  checked,
  onChange,
  required = false,
  infoTip = null,
  error = false,
  warning = false,
  supportingText = null,
  disabled = false,
  policyOutline = null,
  style,
}) {
  const frame = fieldFrameStyle({ error, warning, policyOutline });
  return (
    <FieldShell
      label={label}
      required={required}
      infoTip={infoTip}
      frame={frame}
      disabled={disabled}
      supportingText={supportingText}
      error={error}
      warning={warning}
      style={style}
    >
      <Switch checked={checked} onChange={onChange} disabled={disabled} />
    </FieldShell>
  );
}
