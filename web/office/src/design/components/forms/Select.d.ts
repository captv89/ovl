// FORKED COMPONENT: web/vessel and web/office each keep their own copy of this component and they are intentionally NOT identical (vessel wireframe rework, 2026-07-13). A change here likely needs mirroring to the other app's copy — do not assume they match. See docs/codebase-audit-2026-07-22.md §6.
import * as React from "react";

export interface SelectProps {
  label: React.ReactNode;
  value: string;
  options: string[];
  onChange?: (value: string) => void;
  /** Shown when value is empty. Defaults to "Select…". */
  placeholder?: string;
  supportingText?: React.ReactNode | null;
  error?: boolean;
  warning?: boolean;
  disabled?: boolean;
  /** When set, hovering/focusing the field's own label shows this text via tooltip. */
  labelTooltip?: React.ReactNode | null;
  /** Field-policy marker drawn as this field's own border: solid for mandatory/both, dashed for GHG-relevant. Loses to error/warning/open color. */
  policyOutline?: "mandatory" | "ghgRelevant" | "both" | null;
  style?: React.CSSProperties;
}

export declare function Select(props: SelectProps): React.ReactElement;
