// SPDX-License-Identifier: AGPL-3.0-only

import { useState } from "react";
import { TextField } from "../design/components/forms/TextField.jsx";
import { Button } from "../design/components/core/Button.jsx";
import { AlertBanner } from "../design/components/feedback/AlertBanner.jsx";

const MIN_LENGTH = 8;

// Office has no OIDC provider wired up yet (a deliberate deferral, see
// PROJECT.md's Phase 3 decisions log — never actually blocked on
// Veracity, which is unrelated and now dead anyway) and no
// enrollment-issued credential either — someone has to create the very
// first local Admin account directly, the same role first-run problem
// vessel/httpapi's handleSetupMaster solves on that side. Shown only
// when GET /api/setup/status reports hasAnyUser=false; office/httpapi's
// handleSetupAdmin rejects a second attempt once any user exists.
export function SetupAdmin({ onCreated }: { onCreated: (username: string, password: string) => Promise<void> }) {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const passwordsMatch = confirmPassword === "" || confirmPassword === password;
  const canSubmit = username.trim() !== "" && password.length >= MIN_LENGTH && password === confirmPassword;

  async function handleSubmit() {
    setError(null);
    setLoading(true);
    try {
      await onCreated(username, password);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Something went wrong.");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div style={{ minHeight: "100vh", display: "flex", alignItems: "center", justifyContent: "center", background: "var(--color-primary-container)" }}>
      <div style={{ width: 400, maxWidth: "100%", background: "var(--color-surface)", borderRadius: "var(--shape-extra-large)", padding: 32, boxShadow: "var(--elevation-2)", display: "flex", flexDirection: "column", gap: 20 }}>
        <div>
          <div className="md-display-small" style={{ fontFamily: "var(--font-brand)", color: "var(--color-primary)" }}>
            Tideline
          </div>
          <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
            ovl-office — first-time setup
          </div>
        </div>
        <div className="md-headline-small">Create the first Admin account</div>
        <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
          No account exists yet. Create the Admin account you'll sign in with — it can manage users,
          vessels, groups, and enrollment.
        </div>

        <TextField label="Username" value={username} onChange={setUsername} leadingIcon="badge" />
        <TextField label="Password" value={password} onChange={setPassword} type="password" leadingIcon="lock" />
        <TextField
          label="Confirm password"
          value={confirmPassword}
          onChange={setConfirmPassword}
          type="password"
          leadingIcon="lock"
          error={!passwordsMatch}
          supportingText={passwordsMatch ? undefined : "Passwords don't match"}
        />

        {error ? <AlertBanner level="warning" title="Couldn't create the account" message={error} /> : null}

        <Button
          variant="filled"
          size="large"
          onClick={() => void handleSubmit()}
          disabled={loading || !canSubmit}
          style={{ width: "100%" }}
        >
          {loading ? "Creating…" : "Create account"}
        </Button>
      </div>
    </div>
  );
}
