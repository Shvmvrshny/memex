import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  base: '/ui/',
  server: {
    proxy: {
      '/trace': 'http://localhost:8765',
      '/projects': 'http://localhost:8765',
      '/checkpoint': 'http://localhost:8765',
      '/memories': 'http://localhost:8765',
    }
  }
})
