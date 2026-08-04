// SPDX-License-Identifier: AGPL-3.0-only

import { useEffect, useState, type ReactNode } from "react";
import { TopAppBar } from "../design/components/navigation/TopAppBar.jsx";
import { NavigationRail, type NavigationRailItem } from "../design/components/navigation/NavigationRail.jsx";
import { UserMenu } from "../design/components/navigation/UserMenu.jsx";
import { GroupFilterMenu } from "./GroupFilterMenu";
import { NotificationPanel } from "./NotificationPanel";
import { ChangePasswordDialog } from "./account/ChangePasswordDialog";
import type { UserView } from "../api/client";
import { ROLE_LABELS } from "../roles";

// Design handoff Part B: "Global layout: left navigation (Dashboard,
// Vessels, Reports, Commercial, Configuration, Administration), top bar
// with global vessel-group filter and user menu." The original handoff
// also listed a "Veracity" destination for the planned DNV push
// integration — dropped entirely (not renamed) once DNV declined API
// access; external data access is now pull-based (API keys + GraphQL,
// see Administration's own "API Access" tab), which needs no standing
// nav destination of its own. The global group filter is not built this
// pass (nothing yet consumes it — every section below Dashboard is
// still a placeholder); Administration is hidden entirely for non-Admin
// users rather than shown disabled, matching design-handoff-B10's
// "Roles: Admin" and the nav-hierarchy guidance that a destination the
// user categorically cannot use shouldn't clutter the rail (unlike a
// destination that's simply not built yet, which Section below explains
// with a message instead of hiding).
export type Section = "dashboard" | "vessels" | "reports" | "commercial" | "configuration" | "administration";

const SECTION_TITLES: Record<Section, string> = {
  dashboard: "Dashboard",
  vessels: "Vessels",
  reports: "Reports",
  commercial: "Commercial",
  configuration: "Configuration",
  administration: "Administration",
};

const ALL_ITEMS: (NavigationRailItem & { key: Section })[] = [
  { key: "dashboard", icon: "space_dashboard", label: "Dashboard" },
  { key: "vessels", icon: "directions_boat", label: "Vessels" },
  { key: "reports", icon: "description", label: "Reports" },
  { key: "commercial", icon: "handshake", label: "Commercial" },
  { key: "configuration", icon: "tune", label: "Configuration" },
  { key: "administration", icon: "admin_panel_settings", label: "Administration" },
];

export function AppShell({
  user,
  section,
  onSelectSection,
  onLogout,
  onUserChanged,
  groups,
  groupFilter,
  onGroupFilterChange,
  children,
}: {
  user: UserView;
  section: Section;
  onSelectSection: (section: Section) => void;
  onLogout: () => void;
  /** The user record can change shape after a self-service password change (mustChangePassword clears). */
  onUserChanged: (user: UserView) => void;
  /** Every group tag seen across the fleet, for the global filter's option list. */
  groups: string[];
  groupFilter: string | null;
  onGroupFilterChange: (group: string | null) => void;
  children: ReactNode;
}) {
  const [theme, setTheme] = useState<"light" | "dark">(
    window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light",
  );
  const [changePasswordOpen, setChangePasswordOpen] = useState(false);
  useEffect(() => {
    document.documentElement.setAttribute("data-theme", theme);
  }, [theme]);

  const items = ALL_ITEMS.filter((it) => it.key !== "administration" || user.roles.includes("admin"));

  return (
    <div style={{ display: "flex", flexDirection: "column", height: "100vh", fontFamily: "var(--font-body)" }}>
      <TopAppBar
        title={SECTION_TITLES[section]}
        leadingIcon={null}
        filterSlot={<GroupFilterMenu groups={groups} selected={groupFilter} onChange={onGroupFilterChange} />}
        actions={[
          { icon: theme === "light" ? "dark_mode" : "light_mode", onClick: () => setTheme(theme === "light" ? "dark" : "light") },
        ]}
        trailing={
          <div style={{ display: "flex", alignItems: "center", gap: 4 }}>
            <NotificationPanel onNavigate={onSelectSection} />
            <UserMenu
              username={user.username}
              subtitle={user.roles.map((r) => ROLE_LABELS[r]).join(", ")}
              items={[
                { icon: "key", label: "Change password…", onClick: () => setChangePasswordOpen(true) },
                { icon: "logout", label: "Sign out", onClick: onLogout },
              ]}
              style={{ marginLeft: 4 }}
            />
          </div>
        }
      />
      <div style={{ display: "flex", flex: 1, minHeight: 0 }}>
        <NavigationRail items={items} selected={section} onSelect={(key) => onSelectSection(key as Section)} />
        <div style={{ flex: 1, overflow: "auto" }}>{children}</div>
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
