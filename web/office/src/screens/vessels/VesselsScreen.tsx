// SPDX-License-Identifier: AGPL-3.0-only

import { Suspense, lazy } from "react";
import { Navigate, Route, Routes, useNavigate, useParams } from "react-router-dom";
import { Button } from "../../design/components/core/Button.jsx";
import { api } from "../../api/client";
import type { UserView } from "../../api/client";
import { VesselList } from "./VesselList";
import { VesselDetailScreen } from "./VesselDetailScreen";
import { VesselProfileForm } from "./VesselProfileForm";
import { ReportDetailScreen } from "../reports/ReportDetailScreen";

// Leaflet (react-leaflet + leaflet) is the single largest dependency in the
// office bundle and is only needed on this one route — lazy-load it so the
// main bundle stays small and the map chunk loads on demand.
const VesselMapView = lazy(() => import("./VesselMapView").then((m) => ({ default: m.VesselMapView })));

// Design handoff B2's Vessels section: mounted at "vessels/*" by App.tsx,
// so this screen owns its own list/create/detail/report sub-routes,
// relative to /vessels — real URLs (react-router), not the plain state
// switching this file used before: refreshing mid-drill-in used to always
// drop back to the vessels list (or further, to Dashboard). "report"
// lets the vessel-scoped Reports tab drill into a specific report
// without duplicating ReportDetailScreen (shared with the fleet-wide
// Reports section).
export function VesselsScreen({ user, groupFilter }: { user: UserView; groupFilter: string | null }) {
  const isAdmin = user.roles.includes("admin");
  const isReviewer = user.roles.includes("reviewer");
  const navigate = useNavigate();

  return (
    <Routes>
      <Route
        index
        element={
          <VesselList
            canCreate={isAdmin}
            globalGroup={groupFilter}
            onOpenVessel={(id) => navigate(`/vessels/${id}`)}
            onCreateVessel={() => navigate("/vessels/new")}
            onOpenMap={() => navigate("/vessels/map")}
          />
        }
      />
      <Route path="new" element={<CreateVesselRoute />} />
      <Route
        path="map"
        element={
          <Suspense fallback={null}>
            <VesselMapView globalGroup={groupFilter} onOpenVessel={(id) => navigate(`/vessels/${id}`)} onBackToList={() => navigate("/vessels")} />
          </Suspense>
        }
      />
      <Route path=":vesselId" element={<VesselDetailRoute isAdmin={isAdmin} />} />
      <Route path=":vesselId/reports/:reportId" element={<VesselReportRoute isReviewer={isReviewer} />} />
      <Route path="*" element={<Navigate to="/vessels" replace />} />
    </Routes>
  );
}

function CreateVesselRoute() {
  const navigate = useNavigate();
  return (
    <div style={{ padding: 24 }}>
      <Button variant="text" icon="arrow_back" onClick={() => navigate("/vessels")}>
        Back to vessels
      </Button>
      <div className="md-headline-small" style={{ margin: "8px 0 16px" }}>
        Add vessel
      </div>
      <VesselProfileForm
        submitLabel="Create vessel"
        onCancel={() => navigate("/vessels")}
        onSubmit={async ({ imo, name, type, groups }) => {
          const created = await api.createVessel(imo ?? "", name, type, groups);
          navigate(`/vessels/${created.id}`);
        }}
      />
    </div>
  );
}

function VesselDetailRoute({ isAdmin }: { isAdmin: boolean }) {
  const { vesselId = "" } = useParams();
  const navigate = useNavigate();
  return (
    <VesselDetailScreen
      vesselId={vesselId}
      isAdmin={isAdmin}
      onBack={() => navigate("/vessels")}
      onOpenReport={(reportId) => navigate(`/vessels/${vesselId}/reports/${reportId}`)}
    />
  );
}

function VesselReportRoute({ isReviewer }: { isReviewer: boolean }) {
  const { vesselId = "", reportId = "" } = useParams();
  const navigate = useNavigate();
  return <ReportDetailScreen vesselId={vesselId} reportId={reportId} isReviewer={isReviewer} onBack={() => navigate(`/vessels/${vesselId}`)} />;
}
