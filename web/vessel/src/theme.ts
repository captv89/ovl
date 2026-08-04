// SPDX-License-Identifier: AGPL-3.0-only

// Single owner of document.documentElement's data-theme attribute.
// Previously two things wrote it: main.tsx applied the OS preference
// once and kept listening for OS changes forever, while AppShell held
// its own in-memory theme state that only ever wrote the attribute, so
// a manual toggle would silently get overwritten the next time the OS
// preference changed (e.g. sunset/sunrise auto dark mode on the host).
// Also gave the choice nowhere to persist — a reload always fell back
// to the OS preference. This module fixes both: a stored override in
// localStorage wins once set, and the OS listener only applies while no
// override exists yet.

export type Theme = "light" | "dark";

const STORAGE_KEY = "ovl-theme";

// AppShell's top-bar toggle and Settings' Appearance section can both be
// on screen at once (Settings mounts inside AppShell) and both need to
// reflect whichever one last changed the theme — a plain module-level
// function has no way to notify the other, so changes are broadcast
// here and each caller subscribes.
const CHANGE_EVENT = "ovl:theme-change";

function osPreference(): Theme {
  return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

function apply(theme: Theme): void {
  document.documentElement.setAttribute("data-theme", theme);
  window.dispatchEvent(new Event(CHANGE_EVENT));
}

/** Reads the currently-applied theme off the document. */
export function currentTheme(): Theme {
  const attr = document.documentElement.getAttribute("data-theme");
  return attr === "dark" ? "dark" : "light";
}

/** Sets the theme and persists it as the user's explicit override. */
export function setTheme(theme: Theme): void {
  localStorage.setItem(STORAGE_KEY, theme);
  apply(theme);
}

/** Clears the stored override, falling back to following the OS preference again. */
export function clearThemeOverride(): void {
  localStorage.removeItem(STORAGE_KEY);
  apply(osPreference());
}

export function hasThemeOverride(): boolean {
  return localStorage.getItem(STORAGE_KEY) !== null;
}

/** Notifies cb whenever the applied theme (or override state) changes, from any source. Returns an unsubscribe function. */
export function subscribeTheme(cb: () => void): () => void {
  window.addEventListener(CHANGE_EVENT, cb);
  return () => window.removeEventListener(CHANGE_EVENT, cb);
}

// Applies the stored override (if any) or the OS preference, and starts
// following OS changes for as long as no override has been chosen. Call
// once at startup (main.tsx).
export function initTheme(): void {
  const stored = localStorage.getItem(STORAGE_KEY);
  apply(stored === "dark" || stored === "light" ? stored : osPreference());

  window.matchMedia("(prefers-color-scheme: dark)").addEventListener("change", (e) => {
    if (localStorage.getItem(STORAGE_KEY) !== null) return; // an explicit choice always wins
    apply(e.matches ? "dark" : "light");
  });
}
