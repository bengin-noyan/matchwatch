/// <reference types="vitest/config" />
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'

// Geliştirmede Go API'si :8080'de çalışır ve proxy'lenir; böylece uygulama ile API aynı adresi paylaşır
// (SameSite=Strict çerez).
export default defineConfig({
  plugins: [vue(), tailwindcss()],
  server: { port: 5173, proxy: { '/api': 'http://127.0.0.1:8080' } },
  test: { environment: 'jsdom' },
})
