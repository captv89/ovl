// SPDX-License-Identifier: AGPL-3.0-only

import { Route, Routes, useNavigate, useParams } from "react-router";
import { ReportList } from "./ReportList";
import { ReportDetailScreen } from "./ReportDetailScreen";
import type { UserView } from "../../api/client";

// Design handoff B3/B4's Reports section: mounted at "reports/*" by
// App.tsx, so this screen owns its own list/detail sub-route, relative
// to /reports — real URLs (react-router), not the plain state switching
// this file used before: refreshing while viewing a report's detail used
// to drop all the way back to Dashboard.
export function ReportsScreen({ user, groupFilter }: { user: UserView; groupFilter: string | null }) {
  const isReviewer = user.roles.includes("reviewer");
  const navigate = useNavigate();

  return (
    <Routes>
      <Route
        index
        element={<ReportList isReviewer={isReviewer} globalGroup={groupFilter} onOpenReport={(vesselId, reportId) => navigate(`/reports/${vesselId}/${reportId}`)} />}
      />
      <Route path=":vesselId/:reportId" element={<ReportDetailRoute isReviewer={isReviewer} />} />
    </Routes>
  );
}

function ReportDetailRoute({ isReviewer }: { isReviewer: boolean }) {
  const { vesselId = "", reportId = "" } = useParams();
  const navigate = useNavigate();
  return <ReportDetailScreen vesselId={vesselId} reportId={reportId} isReviewer={isReviewer} onBack={() => navigate("/reports")} />;
}
