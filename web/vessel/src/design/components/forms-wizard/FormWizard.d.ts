import * as React from "react";

export type WizardFillStatus = "complete" | "partial" | "empty";

/** Presence/lock detail for a section's tooltip (architecture 9.5, design handoff A5). */
export interface WizardSectionLock {
  /** Pre-resolved display text, e.g. "C. Nguyen (Chief Engineer)" — FormWizard itself stays free of app-specific role vocabulary. */
  holderLabel: string;
  /** ISO timestamp the lock was acquired — FormWizard renders this as a relative "3 min ago" at render time. */
  since: string;
  /** True when the viewer themselves holds this section — suppresses the lock icon/tooltip entirely. */
  self: boolean;
}

export interface WizardSection {
  key: string;
  label: string;
  /** Data-completeness of the section (drives the nav dot color) — independent of error/warning counts. */
  fillStatus: WizardFillStatus;
  errors?: number;
  warnings?: number;
  /** Count of unresolved office-reviewer remarks (Phase 5) targeting fields in this section. A distinct concept from validation errors/warnings — an office Reviewer's comment, not a rule violation — so it renders as its own chat-icon badge rather than folding into warnings. */
  remarks?: number;
  /** Section is gated (e.g. sequence- or permission-locked) and cannot be selected. */
  locked?: boolean;
  /** Presence/lock detail for the tooltip — omit or leave undefined when nobody holds this section. */
  lock?: WizardSectionLock;
  /** Master-only: when set, renders a "Force release" affordance inside the lock tooltip. Omit for non-Master viewers. */
  onForceRelease?: (sectionKey: string) => void;
}

export interface HealthReportIssue {
  sectionLabel?: string;
  message: string;
  /** Short, actionable guidance shown under the message ("Fix: …"). */
  hint?: string;
  severity: "error" | "warning";
  /** Schema field name this issue is about, if any. Whole-report rules (e.g. timestamp uniqueness) have none. */
  field?: string;
}

export interface FormWizardBreadcrumb {
  label: string;
}

export interface FormWizardProps {
  breadcrumbs?: FormWizardBreadcrumb[];
  title: string;
  /** Small pill next to the title, e.g. "Draft". */
  statusLabel?: string;
  /** Arbitrary content next to the title in place of the statusLabel pill (e.g. a status-badge component). Wins over statusLabel if both are set. */
  status?: React.ReactNode;
  /** Plain text next to statusLabel, e.g. "v1". */
  statusVersion?: string;
  sections: WizardSection[];
  activeKey: string;
  onSelectSection?: (key: string) => void;
  /** The active section's field content. */
  children?: React.ReactNode;
  /** Full issue list across the form; the footer panel defaults to filtering by the active section. */
  issues?: HealthReportIssue[];
  /** Called when a jumpable issue (one with `field` set) is activated by click or Enter/Space. */
  onIssueClick?: (issue: HealthReportIssue) => void;
  /** Primary/secondary buttons rendered at the right of the footer bar. */
  actions?: React.ReactNode;
  /** Controls the footer panel's open/closed state. Omit (with onExpandedChange) to let the panel manage its own state. */
  expanded?: boolean;
  /** Called when the footer panel's expand toggle is clicked, with the new value. Required alongside `expanded` for controlled use. */
  onExpandedChange?: (expanded: boolean) => void;
  /** Replaces the default scoped issue-list body with arbitrary content once expanded (e.g. a full post-check review). Also makes the panel expandable even when there are zero errors/warnings. Omit for the default live-findings issue list. */
  panel?: React.ReactNode;
}

export declare function FormWizard(props: FormWizardProps): React.ReactElement;
