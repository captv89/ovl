// SPDX-License-Identifier: AGPL-3.0-only

import * as React from "react";

export interface MultiSelectMenuProps {
  options: string[];
  /** Selected values. The empty array is a meaningful state, rendered as allLabel — not a null/placeholder state. */
  value: string[];
  onChange?: (value: string[]) => void;
  /** Label for the empty selection, both on the trigger and as the popover's clear-all row. Defaults to "All". */
  allLabel?: string;
  /** When set, renders the 56px outlined Select/TextField shell with this label inside the box, for sitting alongside real Selects in a toolbar. Omit for the compact 36px grid-cell pill. */
  label?: string | null;
  /** Renders a static pill with a lock icon instead of an openable menu. */
  locked?: boolean;
  style?: React.CSSProperties;
}

export declare function MultiSelectMenu(props: MultiSelectMenuProps): React.ReactElement;
