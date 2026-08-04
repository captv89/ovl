// SPDX-License-Identifier: AGPL-3.0-only

import { useEffect, useState } from "react";
import { Navigate, Route, Routes, useNavigate, useParams } from "react-router-dom";
import { DataTable } from "../../design/components/data/DataTable.jsx";
import { AlertBanner } from "../../design/components/feedback/AlertBanner.jsx";
import { Button } from "../../design/components/core/Button.jsx";
import { Tabs } from "../../design/components/navigation/Tabs.jsx";
import { api, type ReportListItemView, type ReportState, type VesselView } from "../../api/client";
import { CommercialReportForm } from "./CommercialReportForm";
import { ReportDetailScreen, StatePill } from "../reports/ReportDetailScreen";

const SCHEMA_BY_TAB = {
  "Commercial periods": "commercial-period",
  "Cargo nominations": "cargo-nomination",
} as const;

type Tab = keyof typeof SCHEMA_BY_TAB;

const TAB_SLUGS: Record<Tab, string> = { "Commercial periods": "commercial-periods", "Cargo nominations": "cargo-nominations" };
const TAB_BY_SLUG: Record<string, Tab> = { "commercial-periods": "Commercial periods", "cargo-nominations": "Cargo nominations" };

// Design handoff B8: office-authored commercial data, "same list anatomy
// as B3" but scoped to just the two office-authored schemas and with a
// "New" action instead of anything vessel-submitted. Mounted at
// "commercial/*" by App.tsx — the active tab, and the create/detail
// drill-in beneath it, are real URL segments (react-router) now, not the
// plain state switching this file used before: refreshing on "Cargo
// nominations" or mid-detail used to drop back to "Commercial periods"
// (or Dashboard).
export function CommercialScreen({ canEdit, isReviewer }: { canEdit: boolean; isReviewer: boolean }) {
  return (
    <Routes>
      <Route index element={<Navigate to={TAB_SLUGS["Commercial periods"]} replace />} />
      <Route path=":tabSlug/*" element={<CommercialTab canEdit={canEdit} isReviewer={isReviewer} />} />
    </Routes>
  );
}

function CommercialTab({ canEdit, isReviewer }: { canEdit: boolean; isReviewer: boolean }) {
  const { tabSlug = "" } = useParams();
  const navigate = useNavigate();
  const tab = TAB_BY_SLUG[tabSlug] ?? "Commercial periods";
  const [reports, setReports] = useState<ReportListItemView[] | null>(null);
  const [vessels, setVessels] = useState<VesselView[]>([]);
  const [error, setError] = useState<string | null>(null);

  const schemaName = SCHEMA_BY_TAB[tab];
  const itemLabel = tab === "Commercial periods" ? "commercial period" : "cargo nomination";
  const tabPath = `/commercial/${TAB_SLUGS[tab]}`;

  function reload() {
    setError(null);
    api
      .listReports({ schema: schemaName })
      .then(setReports)
      .catch((err) => setError(err instanceof Error ? err.message : "Could not load commercial data."));
  }

  useEffect(() => {
    setReports(null);
    reload();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [schemaName]);

  useEffect(() => {
    api.listVessels().then(setVessels).catch(() => undefined);
  }, []);

  return (
    <Routes>
      <Route
        path="new"
        element={
          <div style={{ padding: 24 }}>
            <Button variant="text" icon="arrow_back" onClick={() => navigate(tabPath)}>
              Back to commercial
            </Button>
            <div className="md-headline-small" style={{ margin: "8px 0 16px" }}>
              New {itemLabel}
            </div>
            <CommercialReportForm
              schemaName={schemaName}
              vessels={vessels}
              onCancel={() => navigate(tabPath)}
              onCreated={() => {
                navigate(tabPath);
                reload();
              }}
            />
          </div>
        }
      />
      <Route
        path=":vesselId/:reportId"
        element={<CommercialDetailRoute isReviewer={isReviewer} tabPath={tabPath} onBack={reload} />}
      />
      <Route
        index
        element={
          <div style={{ padding: 24, display: "flex", flexDirection: "column", gap: 16 }}>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
              <div className="md-headline-small">Commercial</div>
              {canEdit ? (
                <Button variant="filled" onClick={() => navigate(`${tabPath}/new`)}>
                  New {itemLabel}
                </Button>
              ) : null}
            </div>

            <Tabs items={Object.keys(SCHEMA_BY_TAB)} selected={tab} onSelect={(t) => navigate(`/commercial/${TAB_SLUGS[t as Tab]}`)} />

            {error ? <AlertBanner level="warning" title="Couldn't load commercial data" message={error} /> : null}

            {!reports ? (
              <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
                Loading…
              </div>
            ) : (
              <DataTable
                columns={[
                  { key: "vessel", label: "Vessel", type: "iconText", sortable: true },
                  { key: "eventType", label: "Type", type: "text", filterable: true },
                  {
                    key: "state",
                    label: "State",
                    type: "badge",
                    filterable: true,
                    render: (row) => <StatePill state={row.state as ReportState} resubmitted={(row.versionNo as number) > 1} />,
                  },
                  { key: "eventTime", label: "Submitted", type: "text", sortable: true },
                ]}
                rows={reports.map((r) => ({
                  id: `${r.vesselId}/${r.reportId}`,
                  vesselId: r.vesselId,
                  reportId: r.reportId,
                  vessel: { icon: "directions_boat", text: r.vesselName, subtext: r.vesselImo },
                  eventType: r.eventType,
                  state: r.state,
                  versionNo: r.versionNo,
                  eventTime: new Date(r.eventTime).toLocaleString(),
                }))}
                onRowAction={(row) => navigate(`${tabPath}/${row.vesselId}/${row.reportId}`)}
                searchPlaceholder="Search"
                emptyMessage={`No ${tab.toLowerCase()} yet.`}
              />
            )}
          </div>
        }
      />
    </Routes>
  );
}

function CommercialDetailRoute({ isReviewer, tabPath, onBack }: { isReviewer: boolean; tabPath: string; onBack: () => void }) {
  const { vesselId = "", reportId = "" } = useParams();
  const navigate = useNavigate();
  return (
    <ReportDetailScreen
      vesselId={vesselId}
      reportId={reportId}
      isReviewer={isReviewer}
      onBack={() => {
        navigate(tabPath);
        onBack();
      }}
    />
  );
}
