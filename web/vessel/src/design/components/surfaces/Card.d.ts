import * as React from "react";

export interface CardProps {
  children: React.ReactNode;
  variant?: "elevated" | "filled" | "outlined";
  padding?: string;
  style?: React.CSSProperties;
  onClick?: () => void;
}

export declare function Card(props: CardProps): React.ReactElement;
