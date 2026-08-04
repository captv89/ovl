import * as React from "react";

export interface TabsProps {
  items: string[];
  selected: string;
  onSelect?: (item: string) => void;
  style?: React.CSSProperties;
}

export declare function Tabs(props: TabsProps): React.ReactElement;
