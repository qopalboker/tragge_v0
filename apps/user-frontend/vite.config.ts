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
      '@user': path.resolve(__dirname, 'src/modules/user'),
      '@trade': path.resolve(__dirname, 'src/modules/trade'),
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api/user': { target: 'http://localhost:8081', changeOrigin: true },
      '/api/trade': { target: 'http://localhost:8082', changeOrigin: true },
      '/ws/trade': { target: 'ws://localhost:8082', ws: true },
      '/ws/tournaments': { target: 'ws://localhost:8082', ws: true },
      '/api/payments': { target: 'http://localhost:8091', changeOrigin: true },
      '/api/wallet': { target: 'http://localhost:8091', changeOrigin: true },
      '/api/tournaments': { target: 'http://localhost:8081', changeOrigin: true },
    },
  },
  build: {
    rollupOptions: {
      output: {
        manualChunks: {
          'vendor-vue': ['vue', 'vue-router', 'pinia'],
          'vendor-charts': ['lightweight-charts'],
        },
      },
    },
  },
})
