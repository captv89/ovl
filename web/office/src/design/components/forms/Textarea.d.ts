// FORKED COMPONENT: web/vessel and web/office each keep their own copy of this component and they are intentionally NOT identical (vessel wireframe rework, 2026-07-13). A change here likely needs mirroring to the other app's copy — do not assume they match. See docs/codebase-audit-2026-07-22.md §6.
import * as React from "react";

export interface TextareaProps {
  label: string;
  value: string;
  onChange?: (value: string) => void;
  variant?: "filled" | "outlined";
  supportingText?: string | null;
  error?: boolean;
  /** Live plausibility finding (severity "warning") on this field — amber label/supportingText, one rung below error. */
  warning?: boolean;
  /** When set, hovering/focusing the field's own label shows this text via tooltip. */
  labelTooltip?: string | null;
  /** Visible row count; the box remains vertically resizable beyond it. */
  rows?: number;
  /** Shows a live "n / max" character counter under the field when set. */
  maxLength?: number | null;
  disabled?: boolean;
  /** Field-policy marker drawn as this field's own border: solid for mandatory/both, dashed for GHG-relevant. Loses to error/focus color. */
  policyOutline?: "mandatory" | "ghgRelevant" | "both" | null;
  style?: React.CSSProperties;
}

export declare function Textarea(props: TextareaProps): React.ReactElement;
