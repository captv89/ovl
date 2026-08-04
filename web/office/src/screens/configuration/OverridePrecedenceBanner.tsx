// SPDX-License-Identifier: AGPL-3.0-only

import { AlertBanner } from "../../design/components/feedback/AlertBanner.jsx";

// Design handoff B7's test feedback #2: "every B7 panel explains the
// vessel > group > fleet override precedence up front." Shared across
// all four B7 panels — Regulatory profiles, Cadence rules, Rule
// severities, and Bundle assignment (which gained fleet-wide scope
// itself as test feedback #3's other half, correcting architecture 6.5
// — see office/configbundle/assignment.go's own doc comment).
export function OverridePrecedenceBanner() {
  return (
    <AlertBanner
      level="ok"
      title="How overriding works"
      message="Vessel and group settings override the fleet default. Most specific wins: vessel > group > fleet. Unset scopes inherit the level above."
    />
  );
}
