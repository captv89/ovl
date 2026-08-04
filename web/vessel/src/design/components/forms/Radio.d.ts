import * as React from "react";

export interface RadioProps {
  selected: boolean;
  onChange?: () => void;
  label?: string;
  disabled?: boolean;
}

export declare function Radio(props: RadioProps): React.ReactElement;
