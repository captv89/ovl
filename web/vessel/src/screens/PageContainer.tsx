// SPDX-License-Identifier: AGPL-3.0-only

import type { ReactNode } from "react";

// The shared page convention for every screen mounted inside AppShell's
// content pane (Home, Reports, Users, Settings): one max-width, one
// spacing scale, one heading semantic. Before this, screens each picked
// their own max-width (640/800/960), their own background token
// (--color-background vs --color-surface), raw pixels vs var(--space-*),
// and a mix of real <h1>s and styled <div>s for the page title — the
// "shell not maintained across the app" complaint wasn't just the outer
// chrome, it was every screen underneath it too. AppShell's content pane
// owns the background now (see its own comment), so this only owns
// layout: padding, max-width, and the title row.
//
// Immersive drill-in screens (ReportForm/FormWizard, whose health-check
// review now lives in FormWizard's own footer panel rather than a
// separate screen) deliberately don't use this — they own their own
// full-bleed layout with a section-nav rail, which a centered max-width
// column would only get in the way of.
export function PageContainer({
  title,
  width = "default",
  leading,
  actions,
  children,
}: {
  title: string;
  width?: "default" | "narrow";
  /** Rendered before the title — a drill-in screen's Back button, e.g. ReportDetailScreen. */
  leading?: ReactNode;
  actions?: ReactNode;
  children: ReactNode;
}) {
  return (
    <div style={{ position: "relative", padding: "var(--space-6)", minHeight: "100%" }}>
      <div style={{ maxWidth: width === "narrow" ? 640 : 960, margin: "0 auto", display: "flex", flexDirection: "column", gap: "var(--space-6)" }}>
        <div style={{ display: "flex", alignItems: "center", gap: "var(--space-3)" }}>
          {leading}
          <h1 className="md-headline-medium" style={{ color: "var(--color-on-surface)", flex: 1 }}>{title}</h1>
          {actions}
        </div>
        {children}
      </div>
    </div>
  );
}
