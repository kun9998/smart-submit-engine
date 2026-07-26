import path from 'node:path'
import { defineConfig, loadEnv } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  const adminPort = env.VITE_ADMIN_PORT || env.ADMIN_PORT || '8090'
  const adminTarget = `http://127.0.0.1:${adminPort}`

  return {
    plugins: [vue(), tailwindcss()],
    resolve: {
      alias: {
        '@': path.resolve(__dirname, './src'),
      },
    },
    server: {
      port: 5173,
      proxy: {
        '/api/logs/stream': {
          target: adminTarget,
          changeOrigin: true,
          timeout: 0,
          proxyTimeout: 0,
        },
        '/api': {
          target: adminTarget,
          changeOrigin: true,
        },
      },
    },
    build: {
      // 源码在 web/，构建产物在 web/dist；发布 ZIP 时将 dist 内容放到与二进制同级的 web/
      outDir: 'dist',
      emptyOutDir: true,
    },
  }
})