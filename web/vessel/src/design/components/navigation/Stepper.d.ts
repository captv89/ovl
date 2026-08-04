import * as React from "react";

export interface StepperStep {
  label: string;
}

export interface StepperProps {
  steps: StepperStep[];
  /** 0-based index of the current step; earlier steps render as done. */
  activeIndex: number;
}

export declare function Stepper(props: StepperProps): React.ReactElement;
