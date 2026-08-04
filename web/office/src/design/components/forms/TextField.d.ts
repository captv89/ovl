// FORKED COMPONENT: web/vessel and web/office each keep their own copy of this component and they are intentionally NOT identical (vessel wireframe rework, 2026-07-13). A change here likely needs mirroring to the other app's copy — do not assume they match. See docs/codebase-audit-2026-07-22.md §6.
import * as React from "react";

export interface TextFieldProps {
  label: string;
  value: string;
  onChange?: (value: string) => void;
  variant?: "filled" | "outlined";
  /**
   * Native input type. "password" renders a masked input with a
   * visibility toggle (suppressed if trailingIcon is also set); any other
   * value (e.g. "number", "date", "time", "datetime-local") passes
   * straight through to the underlying `<input>`.
   */
  type?: "text" | "password" | "number" | "date" | "time" | "datetime-local";
  supportingText?: string | null;
  error?: boolean;
  /** Live plausibility finding (severity "warning") on this field — amber border/label/supportingText, one rung below error. */
  warning?: boolean;
  /** Material Symbols ligature name. */
  leadingIcon?: string | null;
  trailingIcon?: string | null;
  /** When set, the trailing icon shows this text via Tooltip on hover/focus instead of being purely decorative. */
  trailingIconTooltip?: string | null;
  /** Overrides the trailing icon's color, e.g. an amber "overridden" state. */
  trailingIconColor?: string | null;
  /** Accessible name for a clickable trailing icon; falls back to trailingIconTooltip. */
  trailingIconLabel?: string | null;
  /** Makes the trailing icon a real, focusable, clickable button (e.g. "recalculate"). */
  onTrailingIconClick?: (() => void) | null;
  /** Plain-text unit shown after the input, e.g. "kn", "MT" — distinct from trailingIcon. */
  suffix?: string | null;
  disabled?: boolean;
  /** When set, hovering/focusing the field's own label shows this text via tooltip — for fields whose short label needs a bit more context. */
  labelTooltip?: string | null;
  /** Field-policy marker drawn as this field's own border: solid for mandatory/both, dashed for GHG-relevant. Loses to error/focus color. */
  policyOutline?: "mandatory" | "ghgRelevant" | "both" | null;
  style?: React.CSSProperties;
}

export declare function TextField(props: TextFieldProps): React.ReactElement;
