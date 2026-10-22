import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import ui from '@nuxt/ui/vite'

export default defineConfig({
  plugins: [
    vue(),
    // QuillDNS owns its light-first preference. Disable Nuxt UI's second color-mode
    // controller so it cannot overwrite the saved class after app initialization.
    ui({ colorMode: false })
  ],
  build: { outDir: '../webassets/dist', emptyOutDir: true },
  server: { proxy: { '/api': 'http://127.0.0.1:8080' } }
})
