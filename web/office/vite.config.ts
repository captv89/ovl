// SPDX-License-Identifier: AGPL-3.0-only

import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  build: {
    // office/main.go embeds this directory directly via go:embed, which
    // cannot reference a parent directory (../web/office/dist) — same
    // reasoning as web/vessel/vite.config.ts.
    outDir: '../../office/webdist',
    emptyOutDir: true,
  },
})
