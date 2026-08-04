// SPDX-License-Identifier: AGPL-3.0-only

import { useCallback, useEffect, useState } from "react";
import { BrowserRouter, Navigate, Outlet, Route, Routes, useLocation, useNavigate, useParams, useSearchParams } from "react-router";
import { WizardShell } from "./screens/setup/WizardShell";
import { ModeStep } from "./screens/setup/ModeStep";
import { EnrollmentStep } from "./screens/setup/EnrollmentStep";
import { MasterStep } from "./screens/setup/MasterStep";
import { DoneStep } from "./screens/setup/DoneStep";
import { Login } from "./screens/Login";
import { AppShell, type Section } from "./screens/AppShell";
import { Home } from "./screens/home/Home";
import { ReportsScreen } from "./screens/reports/ReportsScreen";
import { UsersScreen } from "./screens/users/UsersScreen";
import { SettingsScreen } from "./screens/settings/SettingsScreen";
import { ReportForm } from "./screens/report-form/ReportForm";
import { ReportDetailScreen } from "./screens/report-detail/ReportDetailScreen";
import { ForcedPasswordChange } from "./screens/account/ForcedPasswordChange";
import { api, ApiError, type SetupStatus, type UserView } from "./api/client";

type View =
  | { kind: "loading" }
  | { kind: "error"; message: string }
  | { kind: "setup"; step: "mode" | "enrollment" | "master" | "done"; status: SetupStatus }
  | { kind: "login" }
  // architecture 9.2's forced password change: interposed between login
  // and the router below whenever the signed-in user's mustChangePassword
  // is still true (a Master just created the account or reset its
  // password) — a one-time interstitial, not itself a destination worth
  // a URL.
  | { kind: "changePassword"; user: UserView; enrolled: boolean }
  | { kind: "authed"; user: UserView; enrolled: boolean };

// Real URL-based routing (react-router), replacing the previous plain
// state-switching View union for every authenticated screen — refreshing
// mid-session used to always drop back to Home because nothing about
// which screen you were on lived in the URL. Login/setup/forced-password-
// change stay outside the router: they're one-time interstitials, not
// destinations worth deep-linking into, and resume() below still gates on
// them before the router ever mounts.
export default function App() {
  const [view, setView] = useState<View>({ kind: "loading" });

  const resume = useCallback(async () => {
    try {
      const status = await api.getSetupStatus();
      if (!status.configured) {
        setView({ kind: "setup", step: "mode", status });
        return;
      }
      if (!status.hasMaster) {
        setView({ kind: "setup", step: "enrollment", status });
        return;
      }
      try {
        const user = await api.me();
        const enrolled = status.enrollment.submitted;
        setView(user.mustChangePassword ? { kind: "changePassword", user, enrolled } : { kind: "authed", user, enrolled });
      } catch {
        setView({ kind: "login" });
      }
    } catch (err) {
      setView({ kind: "error", message: err instanceof ApiError ? err.message : "Could not reach ovl-vessel." });
    }
  }, []);

  useEffect(() => {
    void resume();
  }, [resume]);

  if (view.kind === "loading") {
    return <FullPageMessage text="Loading…" />;
  }
  if (view.kind === "error") {
    return <FullPageMessage text={view.message} />;
  }

  if (view.kind === "login") {
    // Re-resolve through resume() rather than assuming enrolled here —
    // a normal login (not just the one right after the wizard) needs
    // the real enrollment status, or the "Not enrolled" reminder would
    // wrongly disappear after every login. resume() also handles the
    // mustChangePassword branch above, so a temp-password login lands
    // on the forced-change screen the same way a fresh session does.
    return <Login onLoggedIn={() => void resume()} />;
  }

  if (view.kind === "changePassword") {
    return (
      <ForcedPasswordChange
        user={view.user}
        onDone={(user) => setView({ kind: "authed", user, enrolled: view.enrolled })}
      />
    );
  }

  if (view.kind === "authed") {
    return (
      <BrowserRouter>
        <AuthedApp initialUser={view.user} enrolled={view.enrolled} onLogout={() => void api.logout().finally(() => setView({ kind: "login" }))} />
      </BrowserRouter>
    );
  }

  // view.kind === "setup"
  const stepIndex = { mode: 0, enrollment: 1, master: 2, done: 3 }[view.step];
  return (
    <WizardShell activeIndex={stepIndex}>
      {view.step === "mode" && (
        <ModeStep
          defaultDataDir={view.status.defaultDataDir}
          onNext={async (mode, dataDir) => {
            const status = await api.setupMode(mode, dataDir);
            setView({ kind: "setup", step: "enrollment", status });
          }}
        />
      )}
      {view.step === "enrollment" && (
        <EnrollmentStep
          onSubmit={async (officeURL, code) => {
            const status = await api.setupEnrollment({ officeURL, code, skip: false });
            setView({ kind: "setup", step: "master", status });
          }}
          onSkip={async () => {
            const status = await api.setupEnrollment({ skip: true });
            setView({ kind: "setup", step: "master", status });
          }}
        />
      )}
      {view.step === "master" && (
        <MasterStep
          onNext={async (username, password) => {
            await api.setupMaster(username, password);
            setView({ kind: "setup", step: "done", status: view.status });
          }}
        />
      )}
      {view.step === "done" && (
        <DoneStep enrolled={view.status.enrollment.submitted} onStart={() => void resume()} />
      )}
    </WizardShell>
  );
}

// Which nav-rail item highlights for a given location. report-form and
// report-detail are drill-ins reachable from either Home or Reports (see
// each's own onOpenReportForm/onResumeReport/onViewReport below) — which
// rail item they highlight, and where their Back button returns to, is
// carried in the `from` query param so it survives a refresh too.
function sectionForLocation(pathname: string, search: string): Section {
  if (pathname.startsWith("/users")) return "users";
  if (pathname.startsWith("/settings")) return "settings";
  if (pathname.startsWith("/reports") || pathname.startsWith("/report-form")) {
    return new URLSearchParams(search).get("from") === "home" ? "home" : "reports";
  }
  return "home";
}

function ShellLayout({
  user,
  onLogout,
  onUserChanged,
}: {
  user: UserView;
  onLogout: () => void;
  onUserChanged: (user: UserView) => void;
}) {
  const location = useLocation();
  const navigate = useNavigate();
  const section = sectionForLocation(location.pathname, location.search);
  return (
    <AppShell
      user={user}
      section={section}
      onSelectSection={(s) => navigate(s === "home" ? "/" : `/${s}`)}
      onLogout={onLogout}
      onUserChanged={onUserChanged}
    >
      <Outlet />
    </AppShell>
  );
}

function ReportFormRoute({ user }: { user: UserView }) {
  const { schemaName = "", reportId } = useParams();
  const [search] = useSearchParams();
  const navigate = useNavigate();
  const from = search.get("from") === "home" ? "home" : "reports";
  const eventType = search.get("eventType") ?? undefined;
  return (
    <ReportForm
      key={`${schemaName}|${reportId ?? ""}|${eventType ?? ""}`}
      schemaName={schemaName}
      initialEventType={eventType}
      existingReportId={reportId}
      user={user}
      onBack={() => navigate(from === "home" ? "/" : "/reports")}
    />
  );
}

function ReportDetailRoute() {
  const { schemaName = "", reportId = "" } = useParams();
  const [search] = useSearchParams();
  const navigate = useNavigate();
  const from = search.get("from") === "home" ? "home" : "reports";
  return (
    <ReportDetailScreen
      schemaName={schemaName}
      reportId={reportId}
      onBack={() => navigate(from === "home" ? "/" : "/reports")}
      onStartCorrection={(schemaName, reportId) => navigate(`/report-form/${schemaName}/${reportId}${from === "home" ? "?from=home" : ""}`)}
    />
  );
}

function AuthedApp({
  initialUser,
  enrolled,
  onLogout,
}: {
  initialUser: UserView;
  enrolled: boolean;
  onLogout: () => void;
}) {
  const [user, setUser] = useState(initialUser);
  const navigate = useNavigate();

  return (
    <Routes>
      <Route element={<ShellLayout user={user} onLogout={onLogout} onUserChanged={setUser} />}>
        <Route
          index
          element={
            <Home
              enrolled={enrolled}
              onOpenReportForm={(schemaName, eventType) =>
                navigate(`/report-form/${schemaName}?from=home${eventType ? `&eventType=${encodeURIComponent(eventType)}` : ""}`)
              }
              onResumeReport={(schemaName, reportId) => navigate(`/report-form/${schemaName}/${reportId}?from=home`)}
              onViewReport={(schemaName, reportId) => navigate(`/reports/${schemaName}/${reportId}?from=home`)}
            />
          }
        />
        <Route
          path="reports"
          element={
            <ReportsScreen
              onOpenReportForm={(schemaName, eventType) => navigate(`/report-form/${schemaName}${eventType ? `?eventType=${encodeURIComponent(eventType)}` : ""}`)}
              onResumeReport={(schemaName, reportId) => navigate(`/report-form/${schemaName}/${reportId}`)}
              onViewReport={(schemaName, reportId) => navigate(`/reports/${schemaName}/${reportId}`)}
            />
          }
        />
        <Route path="reports/:schemaName/:reportId" element={<ReportDetailRoute />} />
        <Route path="report-form/:schemaName" element={<ReportFormRoute user={user} />} />
        <Route path="report-form/:schemaName/:reportId" element={<ReportFormRoute user={user} />} />
        <Route path="users" element={user.role === "master" ? <UsersScreen user={user} /> : <Navigate to="/" replace />} />
        <Route path="settings" element={<SettingsScreen user={user} />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  );
}

function FullPageMessage({ text }: { text: string }) {
  return (
    <div style={{ minHeight: "100vh", display: "flex", alignItems: "center", justifyContent: "center", background: "var(--color-background)" }}>
      <span className="md-body-large" style={{ color: "var(--color-on-surface-variant)" }}>
        {text}
      </span>
    </div>
  );
}
