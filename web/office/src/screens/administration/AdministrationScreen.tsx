// SPDX-License-Identifier: AGPL-3.0-only

import { Navigate, Route, Routes, useNavigate, useParams } from "react-router";
import { Tabs } from "../../design/components/navigation/Tabs.jsx";
import { UsersTab } from "./UsersTab";
import { VesselGroupsTab } from "./VesselGroupsTab";
import { SystemTab } from "./SystemTab";
import { ApiAccessTab } from "./ApiAccessTab";

const TABS = ["Users", "Vessel groups", "API Access", "System"] as const;
type Tab = (typeof TABS)[number];

const TAB_SLUGS: Record<Tab, string> = { Users: "users", "Vessel groups": "vessel-groups", "API Access": "api-access", System: "system" };
const TAB_BY_SLUG: Record<string, Tab> = { users: "Users", "vessel-groups": "Vessel groups", "api-access": "API Access", system: "System" };

// Design handoff B10: Administration, Admin-only (gated at the nav
// level already — see AppShell.tsx's ALL_ITEMS filter, and App.tsx's own
// "administration/*" route guard for direct-URL access). "API Access"
// added 2026-07-16 (architecture 13.1) — replaces the dropped "Veracity"
// nav destination with admin-issued keys for the new external data API,
// rather than a standing top-level nav item of its own. Mounted at
// "administration/*" by App.tsx — the active tab is a real URL segment
// (react-router) so refreshing on e.g. "API Access" doesn't drop back to
// "Users" (or Dashboard).
export function AdministrationScreen() {
  return (
    <Routes>
      <Route index element={<Navigate to={TAB_SLUGS.Users} replace />} />
      <Route path=":tabSlug" element={<AdministrationTab />} />
    </Routes>
  );
}

function AdministrationTab() {
  const { tabSlug = "" } = useParams();
  const navigate = useNavigate();
  const tab = TAB_BY_SLUG[tabSlug] ?? "Users";

  return (
    <div style={{ padding: 24, display: "flex", flexDirection: "column", gap: 16 }}>
      <div className="md-headline-small">Administration</div>
      <Tabs items={[...TABS]} selected={tab} onSelect={(t) => navigate(`/administration/${TAB_SLUGS[t as Tab]}`)} />
      {tab === "Users" ? (
        <UsersTab />
      ) : tab === "Vessel groups" ? (
        <VesselGroupsTab />
      ) : tab === "API Access" ? (
        <ApiAccessTab />
      ) : (
        <SystemTab />
      )}
    </div>
  );
}
