// FORKED COMPONENT: web/vessel and web/office each keep their own copy of this component and they are intentionally NOT identical (vessel wireframe rework, 2026-07-13). A change here likely needs mirroring to the other app's copy — do not assume they match. See docs/codebase-audit-2026-07-22.md §6.
import * as React from "react";

export interface TextFieldProps {
  label: React.ReactNode;
  value: string;
  onChange?: (value: string) => void;
  /**
   * Native input type. "password" renders a masked input with a
   * visibility toggle in the suffix slot; any other value (e.g.
   * "number", "date", "time", "datetime-local") passes straight through
   * to the underlying `<input>`.
   */
  type?: "text" | "password" | "number" | "date" | "time" | "datetime-local";
  /** Ghost prefill hint ("— last: 2.4 m") rendered as the native input placeholder. */
  placeholder?: string | null;
  supportingText?: React.ReactNode | null;
  error?: boolean;
  /** Live plausibility finding (severity "warning") on this field — amber border/supportingText, one rung below error. */
  warning?: boolean;
  disabled?: boolean;
  required?: boolean;
  /** Always-visible ⓘ tooltip content next to the label. */
  infoTip?: React.ReactNode | null;
  /** Rendered after the info icon in the label row — used for the computed-field "calculated" chip. */
  chip?: React.ReactNode | null;
  /** Material Symbols ligature name shown before the input (e.g. a username or URL icon). */
  leadingIcon?: string | null;
  /** Rendered after the input (ignored for type="password", which owns its own reveal-toggle suffix). */
  suffix?: React.ReactNode | null;
  /** Tinted input background — used for computed fields. */
  filledTint?: boolean;
  /** Field-policy marker drawn as this field's own border: solid for mandatory/both, dashed for GHG-relevant. Loses to error/warning/focus color. */
  policyOutline?: "mandatory" | "ghgRelevant" | "both" | null;
  style?: React.CSSProperties;
}

export declare function TextField(props: TextFieldProps): React.ReactElement;
