// SPDX-License-Identifier: AGPL-3.0-only

import i18n from "i18next";
import { initReactI18next } from "react-i18next";

// Scaffolding only (architecture 17: "String externalization from day
// one so i18n is a translation task later, not a refactor") — English
// is the only real language in v1, and this bundle intentionally
// covers one screen (Login.tsx) as proof the mechanism works end to
// end, not a full-app translation pass. Every other screen's strings
// stay hardcoded English until a real localization effort picks this
// up; when it does, the pattern here (one namespace-flat resources
// object per app, `useTranslation()` + `t()` at the call site) is what
// to extend, not redesign. web/office's own ../i18n/index.ts is the
// same shape — deliberately duplicated per app, not shared, matching
// this project's existing "vendored copy per app" convention for
// anything UI-facing (see reportPersistence.ts's own precedent).
const resources = {
  en: {
    translation: {
      login: {
        title: "OVL Vessel",
        subtitle: "Open Voyage Log",
        username: "Username",
        password: "Password",
        signIn: "Sign in",
        signingIn: "Signing in…",
        signInFailedTitle: "Sign-in failed",
        signInFailedMessage: "Incorrect username or password.",
        forgotPassword: "Forgot password? Ask the Master to reset it.",
      },
    },
  },
};

void i18n.use(initReactI18next).init({
  resources,
  lng: "en",
  fallbackLng: "en",
  interpolation: { escapeValue: false }, // React already escapes
});

export default i18n;
