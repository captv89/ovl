// SPDX-License-Identifier: AGPL-3.0-only

import { Navigate, Route, Routes, useNavigate, useParams } from "react-router";
import { Tabs } from "../../design/components/navigation/Tabs.jsx";
import { FieldPolicyScreen } from "./FieldPolicyScreen";
import { ProfilesAndBundlesScreen } from "./ProfilesAndBundlesScreen";
import type { UserView } from "../../api/client";

const TABS = ["Field policy", "Profiles & bundles"] as const;
type Tab = (typeof TABS)[number];

const TAB_SLUGS: Record<Tab, string> = { "Field policy": "field-policy", "Profiles & bundles": "profiles-bundles" };
const TAB_BY_SLUG: Record<string, Tab> = { "field-policy": "Field policy", "profiles-bundles": "Profiles & bundles" };

// Design handoff Part B's "Configuration" nav section covers B5
// (schema versions, folded into the field policy screen's own upload
// flow), B6 (field policy editor), and B7 (profiles/cadence/severities/
// bundle composer). Mounted at "configuration/*" by App.tsx — the active
// tab is a real URL segment (react-router) so refreshing on "Profiles &
// bundles" doesn't drop back to "Field policy" (or Dashboard).
export function ConfigurationScreen({ user }: { user: UserView }) {
  return (
    <Routes>
      <Route index element={<Navigate to={TAB_SLUGS["Field policy"]} replace />} />
      <Route path=":tabSlug" element={<ConfigurationTab user={user} />} />
    </Routes>
  );
}

function ConfigurationTab({ user }: { user: UserView }) {
  const { tabSlug = "" } = useParams();
  const navigate = useNavigate();
  const tab = TAB_BY_SLUG[tabSlug] ?? "Field policy";
  return (
    <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
      <div style={{ padding: "16px 24px 0" }}>
        <Tabs items={[...TABS]} selected={tab} onSelect={(t) => navigate(`/configuration/${TAB_SLUGS[t as Tab]}`)} />
      </div>
      <div style={{ flex: 1, minHeight: 0 }}>
        {tab === "Field policy" ? <FieldPolicyScreen user={user} /> : <ProfilesAndBundlesScreen user={user} />}
      </div>
    </div>
  );
}
