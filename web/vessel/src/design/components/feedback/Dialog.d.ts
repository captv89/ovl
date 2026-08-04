import * as React from "react";

export interface DialogAction {
  label: string;
  onClick?: () => void;
}
export interface DialogProps {
  open: boolean;
  title: string;
  children: React.ReactNode;
  onClose?: () => void;
  actions?: DialogAction[];
}

export declare function Dialog(props: DialogProps): React.ReactElement;
