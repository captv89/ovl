import * as React from "react";

export interface UserMenuItem {
  /** Material Symbols ligature name. */
  icon?: string;
  label: string;
  onClick?: () => void;
}

export interface UserMenuProps {
  username: string;
  /** Shown under the username in the popover, e.g. a role label. */
  subtitle?: string;
  items?: UserMenuItem[];
  style?: React.CSSProperties;
}

export declare function UserMenu(props: UserMenuProps): React.ReactElement;
