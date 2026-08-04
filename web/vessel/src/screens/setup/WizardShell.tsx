// SPDX-License-Identifier: AGPL-3.0-only

import type { ReactNode } from "react";
import { Stepper } from "../../design/components/navigation/Stepper.jsx";

const STEP_LABELS = [{ label: "Mode" }, { label: "Enrollment" }, { label: "Master account" }, { label: "Done" }];

export function WizardShell({ activeIndex, children }: { activeIndex: number; children: ReactNode }) {
  return (
    <div
      style={{
        minHeight: "100vh",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        background: "var(--color-primary-container)",
        padding: 24,
      }}
    >
      <div
        style={{
          width: 560,
          maxWidth: "100%",
          background: "var(--color-surface)",
          borderRadius: "var(--shape-extra-large)",
          padding: 40,
          boxShadow: "var(--elevation-2)",
          display: "flex",
          flexDirection: "column",
          gap: 32,
        }}
      >
        <div>
          <div className="md-display-small" style={{ fontFamily: "var(--font-brand)", color: "var(--color-primary)" }}>
            OVL Vessel
          </div>
          <div className="md-body-medium" style={{ color: "var(--color-on-surface-variant)" }}>
            First-run setup
          </div>
        </div>
        <Stepper steps={STEP_LABELS} activeIndex={activeIndex} />
        {children}
      </div>
    </div>
  );
}
