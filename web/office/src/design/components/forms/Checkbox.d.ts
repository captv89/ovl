import * as React from "react";

export interface CheckboxProps {
  checked: boolean;
  onChange?: (checked: boolean) => void;
  label?: string;
  disabled?: boolean;
}

export declare function Checkbox(props: CheckboxProps): React.ReactElement;
