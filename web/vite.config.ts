import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import ui from '@nuxt/ui/vite'

export default defineConfig({
  plugins: [vue(), ui()],
  build: { outDir: '../webassets/dist', emptyOutDir: true },
  server: { proxy: { '/api': 'http://127.0.0.1:8080' } }
})
