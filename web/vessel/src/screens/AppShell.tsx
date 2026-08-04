// SPDX-License-Identifier: AGPL-3.0-only

import { useEffect, useState, type ReactNode } from "react";
import { TopAppBar } from "../design/components/navigation/TopAppBar.jsx";
import { NavigationRail, type NavigationRailItem } from "../design/components/navigation/NavigationRail.jsx";
import { UserMenu } from "../design/components/navigation/UserMenu.jsx";
import type { UserView } from "../api/client";
import { currentTheme, setTheme as persistTheme, subscribeTheme } from "../theme";
import { ROLE_LABELS } from "./account/roleLabels";
import { ChangePasswordDialog } from "./account/ChangePasswordDialog";
import { NotificationBell } from "./NotificationBell";

// The vessel-side persistent chrome the user asked for (matching the
// Carbon prototype's structure — top bar + left nav rail — translated to
// Tideline components, not its visual language). Mirrors office's own
// AppShell.tsx pattern.
//
// Originally scoped to "app-level" destinations only (Home, Backup), with
// the report form/detail/health-check screens kept on their own dedicated
// full-bleed layout to avoid "a second, redundant layer of navigation
// chrome." The user flagged that split directly (2026-07-09, "the shell
// is not maintained across the app") after seeing the report form render
// with no top bar at all — no logout, no theme toggle, a differently
// styled nav rail — right after leaving Home. Reversed: every non-setup,
// non-login screen now mounts inside this shell (App.tsx), so logout/
// theme/nav are always in the same place. The immersive screens still
// keep their own internal layout (FormWizard's section rail, etc.) nested
// inside this outer chrome, the same way a detail pane nests inside a
// persistent app frame elsewhere (e.g. an email client's reading pane) —
// only the *outer* frame is now shared, not the whole page structure.
//
// Backup later folded into a full Settings screen (design handoff A10)
// rather than staying its own rail destination — see settings/
// SettingsScreen's own comment.
export type Section = "home" | "reports" | "users" | "settings";

// A single constant rather than a per-section title: the report form/
// detail screens (mounted with section="home" for nav-rail highlighting,
// since they're Home-initiated drill-ins, not their own destinations)
// already render their own breadcrumb/heading right below this bar
// (FormWizard's header, ReportDetailScreen's own <h1>) — a per-section
// "Home" label here would just contradict whatever the content pane
// says under it. One constant identity across every screen instead.
const APP_TITLE = "OVL Vessel";

const ALL_ITEMS: (NavigationRailItem & { key: Section })[] = [
  { key: "home", icon: "home", label: "Home" },
  { key: "reports", icon: "list_alt", label: "Reports" },
  { key: "users", icon: "group", label: "Users" },
  { key: "settings", icon: "settings", label: "Settings" },
];

export function AppShell({
  user,
  section,
  onSelectSection,
  onLogout,
  onUserChanged,
  children,
}: {
  user: UserView;
  section: Section;
  onSelectSection: (section: Section) => void;
  onLogout: () => void;
  /** The user record can change shape after a self-service password change (mustChangePassword clears). */
  onUserChanged: (user: UserView) => void;
  children: ReactNode;
}) {
  const [theme, setThemeState] = useState(currentTheme());
  const [changePasswordOpen, setChangePasswordOpen] = useState(false);

  // Settings' Appearance section can change the theme independently
  // (it's on the same page, inside this same shell) — stay in sync
  // regardless of which control last changed it.
  useEffect(() => subscribeTheme(() => setThemeState(currentTheme())), []);

  function toggleTheme() {
    persistTheme(theme === "light" ? "dark" : "light");
  }

  // Users (architecture 9.3's Master-only user administration) is the
  // same gating Home's old Backup button — now Settings' Backup &
  // restore section — used before this shell existed.
  const items = ALL_ITEMS.filter((it) => it.key !== "users" || user.role === "master");

  return (
    <div style={{ position: "relative", display: "flex", flexDirection: "column", height: "100vh", fontFamily: "var(--font-body)" }}>
      <TopAppBar
        title={APP_TITLE}
        leadingIcon={null}
        actions={[{ icon: theme === "light" ? "dark_mode" : "light_mode", onClick: toggleTheme }]}
        trailing={
          <div style={{ display: "flex", alignItems: "center", gap: 4 }}>
            <NotificationBell onNavigate={onSelectSection} />
            <UserMenu
              username={user.username}
              subtitle={ROLE_LABELS[user.role]}
              items={[
                { icon: "key", label: "Change password…", onClick: () => setChangePasswordOpen(true) },
                { icon: "logout", label: "Sign out", onClick: onLogout },
              ]}
            />
          </div>
        }
      />
      <div style={{ display: "flex", flex: 1, minHeight: 0 }}>
        <NavigationRail items={items} selected={section} onSelect={(key) => onSelectSection(key as Section)} />
        <div style={{ flex: 1, overflow: "auto", background: "var(--color-background)" }}>{children}</div>
      </div>
      <ChangePasswordDialog
        open={changePasswordOpen}
        onClose={() => setChangePasswordOpen(false)}
        onChanged={(updated) => {
          setChangePasswordOpen(false);
          onUserChanged(updated);
        }}
      />
    </div>
  );
}
