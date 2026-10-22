import './style.css'
import { initializeColorMode } from './lib/theme'
import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { PiniaColada } from '@pinia/colada'
import { createRouter, createWebHistory } from 'vue-router'
import ui from '@nuxt/ui/vue-plugin'
import App from './App.vue'

initializeColorMode()

const app = createApp(App)
app.use(createPinia())
app.use(PiniaColada)
app.use(createRouter({ history: createWebHistory(), routes: [{ path: '/', component: App }] }))
app.use(ui)
app.mount('#app')
