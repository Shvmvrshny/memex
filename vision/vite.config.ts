import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    proxy: {
      '/memories': 'http://localhost:8765',
      '/trace': 'http://localhost:8765',
      '/facts': 'http://localhost:8765',
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
  },
})
