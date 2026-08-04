// FORKED COMPONENT: web/vessel and web/office each keep their own copy of this component and they are intentionally NOT identical (vessel wireframe rework, 2026-07-13). A change here likely needs mirroring to the other app's copy — do not assume they match. See docs/codebase-audit-2026-07-22.md §6.
import * as React from "react";

export interface TooltipProps {
  children: React.ReactNode;
  label: React.ReactNode;
  /** When set, wraps long content to this pixel width instead of one nowrap line. */
  maxWidth?: number;
  /** Hover-intent delay in ms before showing (0 = instant, the original behavior). Focus always opens immediately. */
  delay?: number;
}

export declare function Tooltip(props: TooltipProps): React.ReactElement;
