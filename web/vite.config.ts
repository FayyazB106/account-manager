import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  // In production the Go binary embeds and serves this build (single artifact),
  // so emit it where the Go embed lives instead of ./dist.
  build: {
    outDir: '../internal/webui/dist',
    emptyOutDir: true,
  },
  // Dev only: proxy the API to the Go server so the frontend uses a relative
  // `/api` base with no CORS and no hardcoded host.
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
})
