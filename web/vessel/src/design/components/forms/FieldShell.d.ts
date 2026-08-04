import * as React from "react";

export interface FieldFrameStyleInput {
  error?: boolean;
  focused?: boolean;
  warning?: boolean;
  policyOutline?: "mandatory" | "ghgRelevant" | "both" | null;
}

export interface FieldFrame {
  borderColor: string;
  borderStyle: "solid" | "dashed";
  borderWidth: string;
}

export declare function fieldFrameStyle(input?: FieldFrameStyleInput): FieldFrame;

export interface FieldShellProps {
  label: React.ReactNode;
  required?: boolean;
  /** Always-visible ⓘ tooltip content next to the label. */
  infoTip?: React.ReactNode | null;
  /** Arbitrary node rendered after the info icon (e.g. the computed-field "calculated" chip). */
  chip?: React.ReactNode | null;
  /** Result of `fieldFrameStyle` — the input frame's border. */
  frame: FieldFrame;
  /** Tinted input background, used for computed fields. */
  filledTint?: boolean;
  disabled?: boolean;
  /** Material Symbols ligature name shown before the input (e.g. login/setup fields' username/URL icons). */
  leadingIcon?: string | null;
  /** Rendered after the input, e.g. a plain unit span ("kn", "MT") or a clickable restore button. Caller styles its own content. */
  suffix?: React.ReactNode | null;
  supportingText?: React.ReactNode | null;
  error?: boolean;
  warning?: boolean;
  /** Frame height, px. Defaults to 38 (every single-line primitive); Textarea passes a taller value. */
  minHeight?: number;
  /** Frame vertical alignment. Defaults to "center"; Textarea passes "flex-start" for its multiline content. */
  alignItems?: "center" | "flex-start";
  children: React.ReactNode;
  style?: React.CSSProperties;
}

export declare function FieldShell(props: FieldShellProps): React.ReactElement;
