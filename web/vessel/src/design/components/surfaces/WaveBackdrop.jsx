// SPDX-License-Identifier: AGPL-3.0-only

import React from "react";

// Encodes one repeatable wave-line tile as an inline SVG data URI.
// stroke="currentColor" so the wave picks up whatever CSS `color` the host
// div is given — lets each layer use a different theme color (and stay
// theme-reactive across light/dark) without baking a hex value into the
// data URI itself.
function waveTile(strokeWidth) {
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="200" height="60" viewBox="0 0 200 60"><path d="M0,30 C25,10 75,10 100,30 C125,50 175,50 200,30" fill="none" stroke="currentColor" stroke-width="${strokeWidth}"/></svg>`;
  return `url("data:image/svg+xml,${encodeURIComponent(svg)}")`;
}

// Three depths: slower/fainter further back, each a different brand hue at
// low opacity so the wave reads as texture, not a competing UI element.
const LAYERS = [
  { color: "var(--color-primary)", opacity: 0.14, top: "16%", duration: "48s", reverse: false, strokeWidth: 2 },
  { color: "var(--color-secondary)", opacity: 0.12, top: "42%", duration: "60s", reverse: true, strokeWidth: 2 },
  { color: "var(--color-tertiary)", opacity: 0.1, top: "68%", duration: "36s", reverse: false, strokeWidth: 1.5 },
];

/**
 * WaveBackdrop — full-bleed decorative background: layered wave-line bands
 * drifting slowly at different depths/speeds, evoking the sea without being
 * a literal illustration. Pure CSS (tiled background-position scroll, see
 * tokens/motion.css's wave-drift keyframe) — no canvas/WebGL/JS animation
 * loop — and respects prefers-reduced-motion (static when set).
 *
 * Absolutely positioned to fill its nearest positioned ancestor; mount it
 * before the real content in DOM order so content stacks on top without
 * needing an explicit z-index.
 */
export function WaveBackdrop() {
  return (
    <div aria-hidden="true" style={{ position: "absolute", inset: 0, overflow: "hidden", pointerEvents: "none" }}>
      {LAYERS.map((layer, i) => (
        <div
          key={i}
          className="wave-backdrop-layer"
          style={{
            position: "absolute",
            left: 0,
            right: 0,
            top: layer.top,
            height: 60,
            color: layer.color,
            opacity: layer.opacity,
            backgroundImage: waveTile(layer.strokeWidth),
            backgroundRepeat: "repeat-x",
            backgroundSize: "200px 60px",
            animationDirection: layer.reverse ? "reverse" : "normal",
            ["--wave-duration"]: layer.duration,
          }}
        />
      ))}
    </div>
  );
}
