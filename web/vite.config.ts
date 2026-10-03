import { fileURLToPath, URL } from 'node:url'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { defineConfig } from 'vitest/config'

const backend = new URL(
  `http://${(process.env.SUBLANE_ADDR || '127.0.0.1:8080').replace(/^:/, '127.0.0.1:')}`,
)
// Wildcard listen addresses must be reached through loopback by the local proxy.
if (backend.hostname === '0.0.0.0') backend.hostname = '127.0.0.1'
if (backend.hostname === '[::]') backend.hostname = '[::1]'
const target = backend.origin

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) } },
  server: {
    port: 5173,
    strictPort: true,
    proxy: {
      // Keep the browser Host so authentication Origin checks work through the dev proxy.
      '/api': { target, changeOrigin: false },
      '/v1': { target, changeOrigin: false, ws: true },
      '/healthz': target,
      '/readyz': target,
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    restoreMocks: true,
    unstubGlobals: true,
  },
})
