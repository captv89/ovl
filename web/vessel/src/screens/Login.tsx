// SPDX-License-Identifier: AGPL-3.0-only

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { TextField } from "../design/components/forms/TextField.jsx";
import { Button } from "../design/components/core/Button.jsx";
import { AlertBanner } from "../design/components/feedback/AlertBanner.jsx";
import { WaveBackdrop } from "../design/components/surfaces/WaveBackdrop.jsx";
import { api, type UserView } from "../api/client";

// Design handoff A2. Vessel name/IMO aren't shown (no vessel-profile data
// model exists yet — that arrives with enrollment/config bundles,
// Phase 3/4) — the wordmark stands in for now.
//
// This screen is i18n scaffolding's own proof (architecture 17,
// ../i18n/index.ts) — every visible string routes through useTranslation
// rather than being hardcoded, even though English is still the only
// real language shipped. Every other screen in this app is intentionally
// still plain hardcoded strings; extend this pattern to them when real
// localization work starts, don't redesign it.
export function Login({ onLoggedIn }: { onLoggedIn: (user: UserView) => void }) {
  const { t } = useTranslation();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  async function handleSubmit() {
    setError(null);
    setLoading(true);
    try {
      const user = await api.login(username, password);
      onLoggedIn(user);
    } catch {
      // Deliberately generic and identical regardless of cause (unknown
      // username vs. wrong password) — matches the server's own
      // response and A2's "without lockout drama" messaging.
      setError(t("login.signInFailedMessage"));
    } finally {
      setLoading(false);
    }
  }

  return (
    <div style={{ position: "relative", minHeight: "100vh", display: "flex", alignItems: "center", justifyContent: "center", background: "var(--color-primary-container)", overflow: "hidden" }}>
      <WaveBackdrop />
      <div style={{ position: "relative", width: 360, maxWidth: "100%", background: "var(--color-surface)", borderRadius: "var(--shape-extra-large)", padding: 32, boxShadow: "var(--elevation-2)", display: "flex", flexDirection: "column", gap: 20 }}>
        <div>
          <div className="md-display-small" style={{ fontFamily: "var(--font-brand)", color: "var(--color-primary)" }}>
            {t("login.title")}
          </div>
          <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
            {t("login.subtitle")}
          </div>
        </div>
        <TextField label={t("login.username")} value={username} onChange={setUsername} leadingIcon="badge" style={{ width: "100%" }} />
        <TextField label={t("login.password")} value={password} onChange={setPassword} type="password" leadingIcon="lock" style={{ width: "100%" }} />
        {error ? <AlertBanner level="warning" title={t("login.signInFailedTitle")} message={error} /> : null}
        <Button
          variant="filled"
          size="large"
          onClick={() => void handleSubmit()}
          disabled={loading || username.trim() === "" || password === ""}
          style={{ width: "100%" }}
        >
          {loading ? t("login.signingIn") : t("login.signIn")}
        </Button>
        <div className="md-body-small" style={{ color: "var(--color-on-surface-variant)", textAlign: "center" }}>
          {t("login.forgotPassword")}
        </div>
      </div>
    </div>
  );
}
