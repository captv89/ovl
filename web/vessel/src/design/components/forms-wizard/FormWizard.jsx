// SPDX-License-Identifier: AGPL-3.0-only

import React from "react";
import { Tooltip } from "../feedback/Tooltip.jsx";

// A section's fill-status indicator was previously 3 different SOLID dot
// colors (green/blue/gray) — same shape for every state, so at a glance it
// just read as "some blue marker" (2026-07-13 manual-test feedback: "not
// a circle with 3 states"). Shape now carries the state too, color-blind-
// safe on its own: empty is a hollow ring, partial is a half-filled ring,
// complete is a solid filled dot — plus the legend below so the coding is
// explained rather than assumed self-evident.
function sectionFillDotStyle(status) {
  const base = { width: 9, height: 9, borderRadius: "50%", flexShrink: 0, boxSizing: "border-box" };
  switch (status) {
    case "complete":
      return { ...base, background: "var(--color-status-underway)", border: "1.5px solid var(--color-status-underway)" };
    case "partial":
      return {
        ...base,
        background: "linear-gradient(90deg, var(--color-primary) 50%, transparent 50%)",
        border: "1.5px solid var(--color-primary)",
      };
    default:
      return { ...base, background: "transparent", border: "1.5px solid var(--color-outline)" };
  }
}

function SectionFillLegend() {
  const rowStyle = { display: "flex", alignItems: "center", gap: 8 };
  return (
    <div
      className="md-label-small"
      style={{
        display: "flex",
        flexDirection: "column",
        gap: 6,
        padding: "var(--space-3) var(--space-4)",
        borderTop: "1px solid var(--color-outline-variant)",
        color: "var(--color-on-surface-variant)",
      }}
    >
      <span style={rowStyle}>
        <span style={sectionFillDotStyle("empty")} /> Empty
      </span>
      <span style={rowStyle}>
        <span style={sectionFillDotStyle("partial")} /> Partial
      </span>
      <span style={rowStyle}>
        <span style={sectionFillDotStyle("complete")} /> Complete
      </span>
    </div>
  );
}

/**
 * FormWizard — full multi-section form/wizard shell: header (breadcrumb, title,
 * status chip), a left section-nav rail with per-section fill/error/warning/lock
 * indicators, a content pane for the active section (children), and a sticky
 * footer health-report bar (aggregate counts + expandable descriptive issues
 * with fix hints, scoped to the active section). Self-contained — the section
 * rail and health-report bar are implemented inline below, not as separate
 * components, so there's nothing to import if you need a different layout;
 * copy this file as a starting point instead.
 *
 * `status` renders arbitrary content (e.g. a status-badge component) next
 * to the title in place of the built-in `statusLabel` pill — pass one or
 * the other, not both; `status` wins if both are set. `statusLabel` stays
 * for the plain-text case.
 *
 * The footer panel's expanded state is controllable (`expanded`/
 * `onExpandedChange`) so a caller can open it programmatically — e.g. a
 * "Check report" action expanding the panel with richer content instead
 * of navigating to a separate screen. Omit both and the panel manages its
 * own open/closed state as before. `panel` replaces the default scoped
 * issue-list body with arbitrary content once expanded (still gated on
 * `hasIssues`/the same toggle button) — used for the full post-check
 * review (errors, warnings with acknowledge, regulatory readiness,
 * continuity impact, submit) without needing a second surface; leave it
 * unset for the plain live-findings issue list.
 */
export function FormWizard({
  breadcrumbs = [],
  title,
  statusLabel,
  status = null,
  statusVersion,
  sections,
  activeKey,
  onSelectSection,
  children,
  issues = [],
  onIssueClick = null,
  actions = null,
  expanded: expandedProp,
  onExpandedChange,
  panel = null,
}) {
  const [expandedState, setExpandedState] = React.useState(false);
  const expanded = expandedProp ?? expandedState;
  const setExpanded = onExpandedChange ?? setExpandedState;
  const [scope, setScope] = React.useState("active");

  const totalErrors = sections.reduce((n, s) => n + (s.errors || 0), 0);
  const totalWarnings = sections.reduce((n, s) => n + (s.warnings || 0), 0);
  const totalRemarks = sections.reduce((n, s) => n + (s.remarks || 0), 0);
  const hasIssues = totalErrors + totalWarnings > 0;
  const activeSection = sections.find((s) => s.key === activeKey);
  const activeLabel = activeSection && activeSection.label;

  const shown = scope === "active" && activeLabel
    ? issues.filter((i) => i.sectionLabel === activeLabel)
    : issues;

  return (
    <div style={{ display: "flex", flexDirection: "column", height: "100%", fontFamily: "var(--font-body)", background: "var(--color-surface)" }}>
      <div style={{ padding: "var(--space-5) var(--space-6) var(--space-4)", borderBottom: "1px solid var(--color-outline-variant)" }}>
        {breadcrumbs.length ? (
          <div className="md-body-medium" style={{ display: "flex", alignItems: "center", gap: 6, color: "var(--color-on-surface-variant)", marginBottom: "var(--space-2)" }}>
            {breadcrumbs.map((b, i) => (
              <React.Fragment key={i}>
                {i > 0 ? <span className="material-symbols-rounded" style={{ fontSize: 14 }}>chevron_right</span> : null}
                <span style={{ color: i === breadcrumbs.length - 1 ? "var(--color-on-surface-variant)" : "var(--color-primary)" }}>{b.label}</span>
              </React.Fragment>
            ))}
          </div>
        ) : null}
        <div style={{ display: "flex", alignItems: "center", gap: "var(--space-3)" }}>
          <h1 className="md-headline-medium" style={{ color: "var(--color-on-surface)" }}>{title}</h1>
          {status ? (
            status
          ) : statusLabel ? (
            <span className="md-label-large" style={{ display: "inline-flex", alignItems: "center", gap: 6, height: 26, padding: "0 var(--space-3)", borderRadius: "var(--shape-small)", background: "var(--color-surface-container-highest)", color: "var(--color-on-surface)" }}>
              {statusLabel}
            </span>
          ) : null}
          {statusVersion ? <span className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>{statusVersion}</span> : null}
        </div>
      </div>

      <div style={{ flex: 1, display: "flex", minHeight: 0 }}>
        <nav style={{ width: 240, flexShrink: 0, display: "flex", flexDirection: "column", borderRight: "1px solid var(--color-outline-variant)", minHeight: 0 }}>
          <div style={{ flex: 1, overflowY: "auto", padding: "var(--space-2) 0" }}>
          {sections.map((s) => {
            const active = s.key === activeKey;
            const presence = s.lock && !s.lock.self ? s.lock : null;
            return (
              // A wrapping div, not a second nested control: the lock/
              // presence indicator below can itself contain a real
              // "Force release" button (Master only), and a <button>
              // inside a <button> is invalid HTML — this row's own
              // selection control and that indicator have to be
              // siblings, not parent/child.
              <div
                key={s.key}
                style={{
                  position: "relative",
                  // Elevated only when this row owns a presence tooltip:
                  // every row here is position:relative (needed as the
                  // tooltip's anchor), so with z-index:auto everywhere,
                  // later DOM siblings would otherwise paint their own
                  // background over an earlier row's tooltip wherever it
                  // overlaps a neighboring row (its placement="bottom"
                  // case, or "top" against the row above).
                  zIndex: presence ? 1 : "auto",
                  borderLeft: active ? "3px solid var(--color-primary)" : "3px solid transparent",
                  background: active ? "var(--color-surface-container-high)" : "transparent",
                }}
              >
                <button
                  onClick={() => !s.locked && onSelectSection && onSelectSection(s.key)}
                  disabled={s.locked}
                  style={{
                    display: "flex", alignItems: "center", justifyContent: "space-between", gap: "var(--space-2)",
                    width: "100%", padding: "var(--space-3) var(--space-4)", border: "none", background: "transparent",
                    cursor: s.locked ? "not-allowed" : "pointer", textAlign: "left", font: "inherit",
                  }}
                >
                  <span style={{ display: "flex", alignItems: "center", gap: "var(--space-3)", minWidth: 0 }}>
                    <span style={sectionFillDotStyle(s.fillStatus)} />
                    <span className="md-body-large" style={{ color: s.locked ? "var(--color-on-surface-variant)" : "var(--color-on-surface)", whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>
                      {s.label}
                    </span>
                  </span>
                  <span style={{ display: "flex", alignItems: "center", gap: "var(--space-2)", flexShrink: 0 }}>
                    {s.errors ? (
                      <span className="md-label-medium" style={{ display: "flex", alignItems: "center", gap: 3, color: "var(--color-error)" }}>
                        <span style={{ width: 7, height: 7, borderRadius: "50%", background: "var(--color-error)" }} />
                        {s.errors}
                      </span>
                    ) : null}
                    {s.warnings ? (
                      <span className="md-label-medium" style={{ display: "flex", alignItems: "center", gap: 3, color: "var(--color-status-caution)" }}>
                        <span className="material-symbols-rounded" style={{ fontSize: 12 }}>warning</span>
                        {s.warnings}
                      </span>
                    ) : null}
                    {s.remarks ? (
                      <span className="md-label-medium" style={{ display: "flex", alignItems: "center", gap: 3, color: "var(--color-status-caution)" }}>
                        <span className="material-symbols-rounded" style={{ fontSize: 12 }}>chat</span>
                        {s.remarks}
                      </span>
                    ) : null}
                    {!presence && s.locked ? <span className="material-symbols-rounded" style={{ fontSize: 16, color: "var(--color-on-surface-variant)" }}>lock</span> : null}
                    {/* Reserves the presence indicator's width so the label above doesn't reflow when it's added — the indicator itself renders as an absolutely-positioned sibling below. */}
                    {presence ? <span style={{ width: 18, flexShrink: 0 }} /> : null}
                  </span>
                </button>
                {presence ? (
                  <span style={{ position: "absolute", right: "var(--space-4)", top: "50%", transform: "translateY(-50%)" }}>
                    <Tooltip
                      maxWidth={220}
                      placement="bottom"
                      label={<LockTooltipContent lock={presence} onForceRelease={s.onForceRelease ? () => s.onForceRelease(s.key) : null} />}
                    >
                      <PresenceAvatar holderLabel={presence.holderLabel} />
                    </Tooltip>
                  </span>
                ) : null}
              </div>
            );
          })}
          </div>
          <SectionFillLegend />
        </nav>

        <div style={{ flex: 1, minWidth: 0, overflowY: "auto", padding: "var(--space-6)" }}>
          {children}
        </div>
      </div>

      <div style={{ borderTop: "1px solid var(--color-outline-variant)", background: "var(--color-surface-container-low)" }}>
        {expanded && (hasIssues || panel) ? (
          <div style={{ padding: "var(--space-3) var(--space-5)", borderBottom: "1px solid var(--color-outline-variant)", display: "flex", flexDirection: "column", gap: "var(--space-2)", maxHeight: panel ? "min(60vh, 560px)" : 220, overflowY: "auto" }}>
            {panel ? (
              panel
            ) : (
              <>
                {activeLabel ? (
                  <div style={{ display: "flex", gap: "var(--space-2)" }}>
                    {["active", "all"].map((s) => (
                      <button
                        key={s}
                        onClick={() => setScope(s)}
                        className="md-label-large"
                        style={{
                          height: 28, padding: "0 var(--space-3)", borderRadius: "var(--shape-full)", border: "1px solid transparent",
                          cursor: "pointer",
                          background: scope === s ? "var(--color-inverse-surface)" : "transparent",
                          color: scope === s ? "var(--color-inverse-on-surface)" : "var(--color-on-surface-variant)",
                        }}
                      >
                        {s === "active" ? `This section (${activeLabel})` : "All sections"}
                      </button>
                    ))}
                  </div>
                ) : null}

                {shown.length === 0 ? (
                  <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)", padding: "var(--space-2) 0" }}>No issues in this section.</div>
                ) : (
                  shown.map((issue, i) => (
                    <IssueRow key={i} issue={issue} scope={scope} onIssueClick={onIssueClick} />
                  ))
                )}
              </>
            )}
          </div>
        ) : null}

        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "var(--space-3) var(--space-5)" }}>
          <button
            onClick={() => (hasIssues || panel) && setExpanded(!expanded)}
            className="md-label-large"
            style={{ display: "flex", alignItems: "center", gap: "var(--space-4)", background: "none", border: "none", cursor: hasIssues || panel ? "pointer" : "default", padding: 0, color: "var(--color-on-surface)" }}
          >
            <span style={{ display: "flex", alignItems: "center", gap: 6, color: totalErrors ? "var(--color-error)" : "var(--color-on-surface-variant)" }}>
              <span style={{ width: 9, height: 9, borderRadius: "50%", background: totalErrors ? "var(--color-error)" : "var(--color-outline)" }} />
              {totalErrors} error{totalErrors === 1 ? "" : "s"}
            </span>
            <span style={{ display: "flex", alignItems: "center", gap: 6, color: totalWarnings ? "var(--color-status-caution)" : "var(--color-on-surface-variant)" }}>
              <span className="material-symbols-rounded" style={{ fontSize: 14 }}>warning</span>
              {totalWarnings} warning{totalWarnings === 1 ? "" : "s"}
            </span>
            {totalRemarks ? (
              <span style={{ display: "flex", alignItems: "center", gap: 6, color: "var(--color-status-caution)" }}>
                <span className="material-symbols-rounded" style={{ fontSize: 14 }}>chat</span>
                {totalRemarks} remark{totalRemarks === 1 ? "" : "s"}
              </span>
            ) : null}
            {hasIssues || panel ? (
              <span className="material-symbols-rounded" style={{ fontSize: 18, color: "var(--color-on-surface-variant)" }}>{expanded ? "expand_more" : "expand_less"}</span>
            ) : null}
          </button>
          <div style={{ display: "flex", gap: "var(--space-3)" }}>{actions}</div>
        </div>
      </div>
    </div>
  );
}

// A jumpable issue (one with a `field` reference and a handler) is a real
// interactive control, not static text — needs its own hover state (this
// codebase's convention is useState + onMouseEnter/Leave, e.g. Button.jsx,
// not a CSS hover class) and keyboard activation.
function IssueRow({ issue, scope, onIssueClick }) {
  const [hover, setHover] = React.useState(false);
  const jumpable = Boolean(issue.field && onIssueClick);
  return (
    <div
      role={jumpable ? "button" : undefined}
      tabIndex={jumpable ? 0 : undefined}
      onClick={jumpable ? () => onIssueClick(issue) : undefined}
      onKeyDown={jumpable ? (e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); onIssueClick(issue); } } : undefined}
      onMouseEnter={() => jumpable && setHover(true)}
      onMouseLeave={() => setHover(false)}
      style={{
        display: "flex", gap: "var(--space-3)", borderRadius: "var(--shape-small)",
        padding: "var(--space-1) var(--space-2)", margin: "calc(var(--space-1) * -1) calc(var(--space-2) * -1)",
        cursor: jumpable ? "pointer" : "default",
        background: jumpable && hover ? "var(--color-surface-container-high)" : "transparent",
      }}
    >
      <span className="material-symbols-rounded" style={{ fontSize: 18, marginTop: 2, color: issue.severity === "error" ? "var(--color-error)" : "var(--color-status-caution)" }}>
        {issue.severity === "error" ? "error" : "warning"}
      </span>
      <div>
        <div className="md-body-medium" style={{ color: "var(--color-on-surface)" }}>
          {issue.message}
          {scope === "all" && issue.sectionLabel ? (
            <span className="md-label-medium" style={{ color: "var(--color-on-surface-variant)", marginLeft: 8 }}>· {issue.sectionLabel}</span>
          ) : null}
          {jumpable ? (
            <span className="material-symbols-rounded" aria-hidden="true" style={{ fontSize: 14, marginLeft: 6, verticalAlign: "text-bottom", color: "var(--color-on-surface-variant)" }}>arrow_forward</span>
          ) : null}
        </div>
        {issue.hint ? <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)", marginTop: 2 }}>Fix: {issue.hint}</div> : null}
      </div>
    </div>
  );
}

// Same initials-from-a-name technique as navigation/UserMenu.jsx's
// initialsFor, applied to a pre-resolved holderLabel like "C. Nguyen
// (Chief Engineer)" — the role suffix in parens is stripped first so it
// doesn't get treated as part of the name.
function presenceInitials(holderLabel) {
  const name = holderLabel.replace(/\s*\([^)]*\)\s*$/, "").trim();
  const words = name.split(/\s+/).filter(Boolean);
  if (words.length >= 2) return (words[0][0] + words[1][0]).toUpperCase();
  return (name.slice(0, 2) || "?").toUpperCase();
}

// A small presence dot (18px, vs. UserMenu's 36px account-menu trigger —
// this one is a passive indicator inside a nav row, not a clickable
// trigger of its own).
function PresenceAvatar({ holderLabel }) {
  return (
    <span
      style={{
        width: 18, height: 18, borderRadius: "50%", flexShrink: 0,
        background: "var(--color-primary-container)", color: "var(--color-on-primary-container)",
        display: "flex", alignItems: "center", justifyContent: "center",
        fontSize: 9, fontWeight: 600,
      }}
    >
      {presenceInitials(holderLabel)}
    </span>
  );
}

// "3 min ago" / "1 hr ago" — computed at render time from an ISO
// timestamp, not a ticking clock; the tooltip only needs to be roughly
// current whenever it's actually opened.
function relativeTime(iso) {
  const minutes = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 60000));
  if (minutes < 1) return "just now";
  if (minutes === 1) return "1 min ago";
  if (minutes < 60) return `${minutes} min ago`;
  const hours = Math.round(minutes / 60);
  return hours === 1 ? "1 hr ago" : `${hours} hr ago`;
}

// The lock-icon tooltip's content: who holds the section, since when,
// and — Master only, via onForceRelease — a real button to release it
// (design handoff A5: "Master-only force release").
function LockTooltipContent({ lock, onForceRelease }) {
  return (
    <span style={{ display: "flex", flexDirection: "column", gap: 6 }}>
      <span>{`Locked by ${lock.holderLabel}, ${relativeTime(lock.since)}`}</span>
      {onForceRelease ? (
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            onForceRelease();
          }}
          style={{
            alignSelf: "flex-start", background: "none", border: "none", padding: 0,
            cursor: "pointer", color: "inherit", textDecoration: "underline", font: "inherit",
          }}
        >
          Force release
        </button>
      ) : null}
    </span>
  );
}
