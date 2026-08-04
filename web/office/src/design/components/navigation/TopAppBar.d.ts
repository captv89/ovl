// FORKED COMPONENT: web/vessel and web/office each keep their own copy of this component and they are intentionally NOT identical (vessel wireframe rework, 2026-07-13). A change here likely needs mirroring to the other app's copy — do not assume they match. See docs/codebase-audit-2026-07-22.md §6.
import * as React from "react";

export interface TopAppBarAction {
  icon: string;
  onClick?: () => void;
}
export interface TopAppBarProps {
  title: string;
  leadingIcon?: string | null;
  onLeadingClick?: () => void;
  actions?: TopAppBarAction[];
  /** Rendered between the title and the icon actions — a single contextual control that narrows the whole app, e.g. a global group/scope filter pill. */
  filterSlot?: React.ReactNode;
  /** Rendered after the icon actions — for content that isn't a simple icon button, e.g. an account/user menu. */
  trailing?: React.ReactNode;
  style?: React.CSSProperties;
}

export declare function TopAppBar(props: TopAppBarProps): React.ReactElement;
