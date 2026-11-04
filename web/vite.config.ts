/// <reference types="vitest/config" />
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'

// Dev: the Go API runs on :8080 and is proxied, so the app and API share one origin (SameSite=Strict cookie).
export default defineConfig({
  plugins: [vue(), tailwindcss()],
  server: { port: 5173, proxy: { '/api': 'http://127.0.0.1:8080' } },
  test: { environment: 'jsdom' },
})
