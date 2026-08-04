import * as React from "react";

export interface PositionFieldProps {
  axis: "lat" | "lon";
  label: React.ReactNode;
  degree?: string | number;
  minutes?: string | number;
  hemisphere?: string;
  onChangeDegree?: (value: string) => void;
  onChangeMinutes?: (value: string) => void;
  onChangeHemisphere?: (value: string) => void;
  error?: boolean;
  /** Live plausibility finding (severity "warning") on this field — amber border/supportingText, one rung below error. */
  warning?: boolean;
  /** Message shown below the field, tinted by error/warning severity — e.g. a live plausibility finding's text. */
  supportingText?: React.ReactNode | null;
  /** Always-visible ⓘ tooltip content next to the label. */
  infoTip?: React.ReactNode | null;
  disabled?: boolean;
  required?: boolean;
  /** Field-policy marker drawn as this field's own border: solid for mandatory/both, dashed for GHG-relevant. Loses to error/warning/focus color. */
  policyOutline?: "mandatory" | "ghgRelevant" | "both" | null;
  style?: React.CSSProperties;
}

export declare function PositionField(props: PositionFieldProps): React.ReactElement;
