// SPDX-License-Identifier: AGPL-3.0-only

import React from "react";

/**
 * Stepper — linear, numbered progress indicator for multi-step flows
 * (first-run wizards, onboarding). Material 3 has no official stepper
 * component; this follows the same primitives (filled circles, M3
 * color roles, Roboto labels) used throughout the rest of Tideline.
 */
export function Stepper({ steps, activeIndex }) {
  return (
    <div style={{ display: "flex", alignItems: "flex-start", width: "100%" }}>
      {steps.map((step, i) => {
        const done = i < activeIndex;
        const active = i === activeIndex;
        const circleBg = done || active ? "var(--color-primary)" : "var(--color-surface-container-highest)";
        const circleFg = done || active ? "var(--color-on-primary)" : "var(--color-on-surface-variant)";
        return (
          <React.Fragment key={step.label}>
            <div style={{ display: "flex", flexDirection: "column", alignItems: "center", gap: 8, minWidth: 72 }}>
              <div
                style={{
                  width: 32,
                  height: 32,
                  borderRadius: "50%",
                  background: circleBg,
                  color: circleFg,
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "center",
                  transition: "background var(--motion-duration-medium) var(--motion-easing-standard)",
                }}
              >
                {done ? (
                  <span className="material-symbols-rounded" style={{ fontSize: 18 }}>check</span>
                ) : (
                  <span className="md-label-large" style={{ color: circleFg }}>{i + 1}</span>
                )}
              </div>
              <span
                className="md-label-medium"
                style={{ color: active ? "var(--color-on-surface)" : "var(--color-on-surface-variant)", textAlign: "center" }}
              >
                {step.label}
              </span>
            </div>
            {i < steps.length - 1 ? (
              <div
                style={{
                  flex: 1,
                  height: 2,
                  marginTop: 15,
                  background: done ? "var(--color-primary)" : "var(--color-outline-variant)",
                  transition: "background var(--motion-duration-medium) var(--motion-easing-standard)",
                }}
              />
            ) : null}
          </React.Fragment>
        );
      })}
    </div>
  );
}
