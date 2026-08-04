import * as React from "react";

export interface BooleanFieldProps {
  label: React.ReactNode;
  checked: boolean;
  onChange?: (checked: boolean) => void;
  required?: boolean;
  /** Always-visible ⓘ tooltip content next to the label. */
  infoTip?: React.ReactNode | null;
  error?: boolean;
  /** Live plausibility finding (severity "warning") on this field — amber border/supportingText, one rung below error. */
  warning?: boolean;
  supportingText?: React.ReactNode | null;
  disabled?: boolean;
  /** Field-policy marker drawn as this field's own border: solid for mandatory/both, dashed for GHG-relevant. Loses to error/warning color. */
  policyOutline?: "mandatory" | "ghgRelevant" | "both" | null;
  style?: React.CSSProperties;
}

export declare function BooleanField(props: BooleanFieldProps): React.ReactElement;
