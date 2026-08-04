// SPDX-License-Identifier: AGPL-3.0-only

import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './design/styles.css'
import './i18n'
import App from './App.tsx'

// Same OS-preference-follows theme bootstrap as web/vessel's main.tsx —
// no manual toggle exists at the document level (AppShell's own toggle
// only takes effect once a session exists; this covers Login/SetupAdmin
// too, which render before AppShell mounts).
const prefersDark = window.matchMedia('(prefers-color-scheme: dark)')
const applyTheme = () => {
  document.documentElement.setAttribute('data-theme', prefersDark.matches ? 'dark' : 'light')
}
applyTheme()
prefersDark.addEventListener('change', applyTheme)

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
