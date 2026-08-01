import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'
import path from 'path'

export default defineConfig({
  plugins: [vue(), tailwindcss()],
  test: {
    exclude: ['e2e/**', 'node_modules/**'],
    passWithNoTests: true,
  },
  resolve: {
    alias: {
      '@': path.resolve(__dirname, 'src'),
      '@admin': path.resolve(__dirname, 'src/modules/admin'),
    },
  },
  server: {
    port: 5174,
    proxy: {
      // admin-frontend only talks to admin-bff. Cross-panel API calls
      // are a leak indicator; the gateway will return 404 for them in
      // production (Step 8).
      '/api/admin': { target: 'http://localhost:8083', changeOrigin: true },
    },
  },
  build: {
    rollupOptions: {
      output: {
        manualChunks: {
          'vendor-vue': ['vue', 'vue-router', 'pinia'],
        },
      },
    },
  },
})
