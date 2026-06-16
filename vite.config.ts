import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import path from 'path'

// https://vite.dev/config/
export default defineConfig({
  server: {
    host: '127.0.0.1',
    port: 5174,
    proxy: {
      '/api': 'http://127.0.0.1:18081',
      '/mcp': 'http://127.0.0.1:18081',
      '/jsonrpc': {
        target: 'http://127.0.0.1:18081',
        ws: true,
      },
    },
  },
  build: {
    sourcemap: false,
    outDir: 'internal/web/dist',
    emptyOutDir: true,
  },
  plugins: [
    vue(),
  ],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'), // ✅ 定义 @ = src
    },
  },
})
