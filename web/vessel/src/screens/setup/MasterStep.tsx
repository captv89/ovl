// SPDX-License-Identifier: AGPL-3.0-only

import { useMemo, useState } from "react";
import { TextField } from "../../design/components/forms/TextField.jsx";
import { Button } from "../../design/components/core/Button.jsx";
import { AlertBanner } from "../../design/components/feedback/AlertBanner.jsx";
import { ProgressIndicator } from "../../design/components/feedback/ProgressIndicator.jsx";
import { MIN_PASSWORD_LENGTH, passwordStrength } from "../account/passwordStrength";

export function MasterStep({
  onNext,
}: {
  onNext: (username: string, password: string) => Promise<void>;
}) {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const meter = useMemo(() => passwordStrength(password), [password]);
  const passwordsMatch = confirmPassword === "" || confirmPassword === password;
  const canSubmit =
    username.trim() !== "" && password.length >= MIN_PASSWORD_LENGTH && password === confirmPassword;

  async function handleContinue() {
    setError(null);
    setLoading(true);
    try {
      await onNext(username, password);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Something went wrong.");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
      <div>
        <div className="md-headline-small">Create the Master account</div>
        <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)", marginTop: 4 }}>
          Master is this vessel's super-admin: manages users, resets passwords, and can always submit
          reports. Choose credentials only you will use.
        </div>
      </div>

      <TextField label="Username" value={username} onChange={setUsername} leadingIcon="badge" />

      <div>
        <TextField label="Password" value={password} onChange={setPassword} type="password" leadingIcon="lock" />
        {password.length > 0 ? (
          <div style={{ marginTop: 8, display: "flex", flexDirection: "column", gap: 4 }}>
            <ProgressIndicator variant="linear" value={(meter.score / 4) * 100} />
            <span className="md-label-small" style={{ color: meter.color }}>
              {meter.label}
            </span>
          </div>
        ) : null}
      </div>

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

      <Button variant="filled" size="large" onClick={() => void handleContinue()} disabled={loading || !canSubmit} style={{ alignSelf: "flex-end" }}>
        {loading ? "Creating…" : "Create account"}
      </Button>
    </div>
  );
}
