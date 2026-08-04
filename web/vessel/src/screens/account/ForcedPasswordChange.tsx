// SPDX-License-Identifier: AGPL-3.0-only

import { useMemo, useState } from "react";
import { TextField } from "../../design/components/forms/TextField.jsx";
import { Button } from "../../design/components/core/Button.jsx";
import { AlertBanner } from "../../design/components/feedback/AlertBanner.jsx";
import { ProgressIndicator } from "../../design/components/feedback/ProgressIndicator.jsx";
import { WaveBackdrop } from "../../design/components/surfaces/WaveBackdrop.jsx";
import { api, ApiError, type UserView } from "../../api/client";
import { MIN_PASSWORD_LENGTH, passwordStrength } from "./passwordStrength";

// Architecture 9.2's "forced password change on first login": shown
// full-page, with no way to skip or cancel, whenever the signed-in
// user's mustChangePassword is still true (a Master just created the
// account or reset its password — see vessel/httpapi/users.go). Same
// visual language as Login/WizardShell (WaveBackdrop, centered card),
// not a Dialog, since there is nowhere else for the user to go until
// this is done.
export function ForcedPasswordChange({
  user,
  onDone,
}: {
  user: UserView;
  onDone: (user: UserView) => void;
}) {
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const meter = useMemo(() => passwordStrength(newPassword), [newPassword]);
  const passwordsMatch = confirmPassword === "" || confirmPassword === newPassword;
  const canSubmit =
    currentPassword !== "" && newPassword.length >= MIN_PASSWORD_LENGTH && newPassword === confirmPassword;

  async function handleSubmit() {
    setError(null);
    setLoading(true);
    try {
      const updated = await api.changePassword(currentPassword, newPassword);
      onDone(updated);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not change the password.");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div style={{ position: "relative", minHeight: "100vh", display: "flex", alignItems: "center", justifyContent: "center", background: "var(--color-primary-container)", overflow: "hidden" }}>
      <WaveBackdrop />
      <div style={{ position: "relative", width: 360, maxWidth: "100%", background: "var(--color-surface)", borderRadius: "var(--shape-extra-large)", padding: 32, boxShadow: "var(--elevation-2)", display: "flex", flexDirection: "column", gap: 20 }}>
        <div>
          <div className="md-headline-small">Choose a new password</div>
          <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)", marginTop: 4 }}>
            A temporary password was set for {user.username} — choose your own to continue.
          </div>
        </div>

        <TextField label="Temporary password" value={currentPassword} onChange={setCurrentPassword} type="password" leadingIcon="lock" style={{ width: "100%" }} />

        <div>
          <TextField label="New password" value={newPassword} onChange={setNewPassword} type="password" leadingIcon="lock" style={{ width: "100%" }} />
          {newPassword.length > 0 ? (
            <div style={{ marginTop: 8, display: "flex", flexDirection: "column", gap: 4 }}>
              <ProgressIndicator variant="linear" value={(meter.score / 4) * 100} />
              <span className="md-label-small" style={{ color: meter.color }}>{meter.label}</span>
            </div>
          ) : null}
        </div>

        <TextField
          label="Confirm new password"
          value={confirmPassword}
          onChange={setConfirmPassword}
          type="password"
          leadingIcon="lock"
          error={!passwordsMatch}
          supportingText={passwordsMatch ? undefined : "Passwords don't match"}
          style={{ width: "100%" }}
        />

        {error ? <AlertBanner level="warning" title="Couldn't change password" message={error} /> : null}

        <Button variant="filled" size="large" onClick={() => void handleSubmit()} disabled={loading || !canSubmit} style={{ width: "100%" }}>
          {loading ? "Changing…" : "Continue"}
        </Button>
      </div>
    </div>
  );
}
