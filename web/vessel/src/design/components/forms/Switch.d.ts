import * as React from "react";

export interface SwitchProps {
  checked: boolean;
  onChange?: (checked: boolean) => void;
  disabled?: boolean;
}

export declare function Switch(props: SwitchProps): React.ReactElement;
