// SPDX-License-Identifier: AGPL-3.0-only

import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './design/styles.css'
import './i18n'
import App from './App.tsx'
import { initTheme } from './theme'

// Tideline ships both a light and a "Night Bridge" dark scheme (bridges
// are kept dark at night for outward visibility). initTheme applies a
// stored manual choice (set via AppShell's theme toggle) or falls back
// to the OS preference, and keeps following OS changes only until the
// user picks one explicitly — see theme.ts.
initTheme()

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
