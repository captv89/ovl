// SPDX-License-Identifier: AGPL-3.0-only

import { useEffect, useState } from "react";
import { Tabs } from "../../design/components/navigation/Tabs.jsx";
import { api, type UserView, type VesselView } from "../../api/client";
import { RegulatoryProfilesPanel } from "./RegulatoryProfilesPanel";
import { CadenceRulesPanel } from "./CadenceRulesPanel";
import { RuleSeveritiesPanel } from "./RuleSeveritiesPanel";
import { BundleComposerPanel } from "./BundleComposerPanel";
import { VesselConfigsScreen } from "./VesselConfigsScreen";

const TABS = ["Regulatory profiles", "Cadence rules", "Rule severities", "Bundles", "Vessel configs"] as const;
type Tab = (typeof TABS)[number];

// Design handoff B7: "Configuration: profiles, cadence and bundles."
// Sub-tabbed the same way B6/B5 share the "Field policy" tab — B7 packs
// five distinct sub-screens (profiles, cadence, rule severities, bundle
// composer/history/assignment, and — 2026-07-25 redesign — the
// fleet-wide vessel configs table) that all need the same vessel/group
// scope picker, so vessels are fetched once here and passed down.
// Vessel configs is read-only and fetches its own data, but stays in
// this same tab group since it's still about bundle-application status.
export function ProfilesAndBundlesScreen({ user }: { user: UserView }) {
  const [tab, setTab] = useState<Tab>("Regulatory profiles");
  const [vessels, setVessels] = useState<VesselView[]>([]);
  const canEdit = user.roles.includes("configManager");

  useEffect(() => {
    api.listVessels().then(setVessels).catch(() => setVessels([]));
  }, []);

  return (
    <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
      <div style={{ padding: "16px 24px 0" }}>
        <Tabs items={[...TABS]} selected={tab} onSelect={(t) => setTab(t as Tab)} />
      </div>
      <div style={{ flex: 1, minHeight: 0, overflow: "auto" }}>
        {tab === "Regulatory profiles" ? <RegulatoryProfilesPanel vessels={vessels} canEdit={canEdit} /> : null}
        {tab === "Cadence rules" ? <CadenceRulesPanel vessels={vessels} canEdit={canEdit} /> : null}
        {tab === "Rule severities" ? <RuleSeveritiesPanel vessels={vessels} canEdit={canEdit} /> : null}
        {tab === "Bundles" ? <BundleComposerPanel vessels={vessels} canEdit={canEdit} /> : null}
        {tab === "Vessel configs" ? <VesselConfigsScreen /> : null}
      </div>
    </div>
  );
}
