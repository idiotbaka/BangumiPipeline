import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig(({ mode }) => {
  const target = mode === 'tv' ? 'tv' : 'mobile'

  return {
    plugins: [vue()],
    define: {
      'import.meta.env.VITE_APP_TARGET': JSON.stringify(target),
    },
    server: {
      host: '0.0.0.0',
      port: 1420,
      strictPort: true,
    },
    build: {
      outDir: target === 'tv' ? 'dist-tv' : 'dist',
      emptyOutDir: true,
    },
  }
})
