// SPDX-License-Identifier: AGPL-3.0-only

import { useCallback, useEffect, useState } from "react";
import { BrowserRouter, Navigate, Outlet, Route, Routes, useLocation, useNavigate } from "react-router";
import { Login } from "./screens/Login";
import { SetupAdmin } from "./screens/SetupAdmin";
import { AppShell, type Section } from "./screens/AppShell";
import { Dashboard } from "./screens/dashboard/Dashboard";
import { VesselsScreen } from "./screens/vessels/VesselsScreen";
import { ReportsScreen } from "./screens/reports/ReportsScreen";
import { ConfigurationScreen } from "./screens/configuration/ConfigurationScreen";
import { CommercialScreen } from "./screens/commercial/CommercialScreen";
import { AdministrationScreen } from "./screens/administration/AdministrationScreen";
import { api, ApiError, type UserView } from "./api/client";

type View =
  | { kind: "loading" }
  | { kind: "error"; message: string }
  | { kind: "setupAdmin" }
  | { kind: "login" }
  | { kind: "authed"; user: UserView };

// Real URL-based routing (react-router), replacing the previous plain
// state-switching View union: office's screens (and several of their own
// sub-navigation, e.g. Reports' list/detail drill-in, Configuration's/
// Administration's tabs) used to keep "which screen am I on" purely in
// memory, so a refresh always dropped back to Dashboard. setupAdmin/login
// stay outside the router — one-time interstitials, not destinations
// worth deep-linking into.
export default function App() {
  const [view, setView] = useState<View>({ kind: "loading" });

  const resume = useCallback(async () => {
    try {
      const status = await api.getSetupStatus();
      if (!status.hasAnyUser) {
        setView({ kind: "setupAdmin" });
        return;
      }
      try {
        const user = await api.me();
        setView({ kind: "authed", user });
      } catch {
        setView({ kind: "login" });
      }
    } catch (err) {
      setView({ kind: "error", message: err instanceof ApiError ? err.message : "Could not reach ovl-office." });
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

  if (view.kind === "setupAdmin") {
    return (
      <SetupAdmin
        onCreated={async (username, password) => {
          await api.setupAdmin(username, password);
          await resume();
        }}
      />
    );
  }

  if (view.kind === "login") {
    return <Login onLoggedIn={(user) => setView({ kind: "authed", user })} />;
  }

  return (
    <BrowserRouter>
      <AuthedApp initialUser={view.user} onLogout={() => void api.logout().finally(() => setView({ kind: "login" }))} />
    </BrowserRouter>
  );
}

function sectionForPath(pathname: string): Section {
  const top = pathname.split("/")[1];
  if (top === "vessels" || top === "reports" || top === "commercial" || top === "configuration" || top === "administration") {
    return top;
  }
  return "dashboard";
}

function ShellLayout({
  user,
  onLogout,
  onUserChanged,
  groups,
  groupFilter,
  onGroupFilterChange,
}: {
  user: UserView;
  onLogout: () => void;
  onUserChanged: (user: UserView) => void;
  groups: string[];
  groupFilter: string | null;
  onGroupFilterChange: (group: string | null) => void;
}) {
  const location = useLocation();
  const navigate = useNavigate();
  const section = sectionForPath(location.pathname);
  return (
    <AppShell
      user={user}
      section={section}
      onSelectSection={(s) => navigate(s === "dashboard" ? "/" : `/${s}`)}
      onLogout={onLogout}
      onUserChanged={onUserChanged}
      groups={groups}
      groupFilter={groupFilter}
      onGroupFilterChange={onGroupFilterChange}
    >
      <Outlet />
    </AppShell>
  );
}

function AuthedApp({ initialUser, onLogout }: { initialUser: UserView; onLogout: () => void }) {
  const [user, setUser] = useState(initialUser);
  const [groups, setGroups] = useState<string[]>([]);
  const [groupFilter, setGroupFilter] = useState<string | null>(null);
  const navigate = useNavigate();
  const isAdmin = user.roles.includes("admin");

  // Global group filter's option list (design handoff Part B's top-bar
  // filter "narrows every screen"): every group tag seen across the
  // fleet. Fetched once on mount — cheap (one extra listVessels() call)
  // and avoids coupling this app-level control to a specific screen's
  // fetch lifecycle.
  useEffect(() => {
    let cancelled = false;
    api
      .listVessels()
      .then((vessels) => {
        if (cancelled) return;
        const set = new Set<string>();
        for (const v of vessels) for (const g of v.groups) set.add(g);
        setGroups([...set].sort());
      })
      .catch(() => {
        // Best-effort — the global filter simply shows no options if this fails.
      });
    return () => {
      cancelled = true;
    };
  }, []);

  return (
    <Routes>
      <Route
        element={
          <ShellLayout user={user} onLogout={onLogout} onUserChanged={setUser} groups={groups} groupFilter={groupFilter} onGroupFilterChange={setGroupFilter} />
        }
      >
        <Route
          index
          element={<Dashboard user={user} groupFilter={groupFilter} onNavigate={(section) => navigate(section === "dashboard" ? "/" : `/${section}`)} />}
        />
        <Route path="vessels/*" element={<VesselsScreen user={user} groupFilter={groupFilter} />} />
        <Route path="reports/*" element={<ReportsScreen user={user} groupFilter={groupFilter} />} />
        <Route path="configuration/*" element={<ConfigurationScreen user={user} />} />
        <Route
          path="commercial/*"
          element={<CommercialScreen canEdit={user.roles.includes("commercialEditor")} isReviewer={user.roles.includes("reviewer")} />}
        />
        <Route path="administration/*" element={isAdmin ? <AdministrationScreen /> : <Navigate to="/" replace />} />
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
