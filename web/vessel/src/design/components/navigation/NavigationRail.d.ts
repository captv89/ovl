import * as React from "react";

export interface NavigationRailItem {
  key: string;
  icon: string;
  label: string;
}

export interface NavigationRailProps {
  items: NavigationRailItem[];
  selected: string;
  onSelect?: (key: string) => void;
  style?: React.CSSProperties;
}

export declare function NavigationRail(props: NavigationRailProps): React.ReactElement;
