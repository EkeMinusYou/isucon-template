import tailwindcss from '@tailwindcss/vite'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

const dashboardApiTarget = process.env.DASHBOARD_API_TARGET ?? 'http://127.0.0.1:8091'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    proxy: {
      '/api': {
        target: dashboardApiTarget,
        changeOrigin: true,
      },
    },
  },
})
