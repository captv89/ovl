// SPDX-License-Identifier: AGPL-3.0-only

import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  build: {
    // vessel/main.go embeds this directory directly via go:embed, which
    // cannot reference a parent directory (../web/vessel/dist).
    outDir: '../../vessel/webdist',
    emptyOutDir: true,
  },
})
