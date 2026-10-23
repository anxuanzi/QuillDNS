<script setup lang="ts">
import { computed } from 'vue'
import { useColorMode } from './lib/theme'
import { activePage, type Page } from './lib/navigation'
import OverviewPage from './features/overview/OverviewPage.vue'
import RecordsPage from './features/records/RecordsPage.vue'
import QueriesPage from './features/queries/QueriesPage.vue'
import SettingsPage from './features/settings/SettingsPage.vue'

const page = activePage
const navigation: { key: Page, label: string, icon: string }[] = [
  { key: 'overview', label: 'Overview', icon: 'i-lucide-layout-dashboard' },
  { key: 'records', label: 'DNS Records', icon: 'i-lucide-list-tree' },
  { key: 'queries', label: 'Query Logs', icon: 'i-lucide-chart-no-axes-combined' },
  { key: 'settings', label: 'Settings', icon: 'i-lucide-settings-2' }
]
const { mode, toggle } = useColorMode()
const copyrightYear = new Date().getFullYear()
const current = computed(() => navigation.find(item => item.key === page.value)!)
const views = { overview: OverviewPage, records: RecordsPage, queries: QueriesPage, settings: SettingsPage }
</script>

<template>
  <UApp>
    <div class="shell">
      <aside class="sidebar">
        <div class="brand">
          <img class="brand-mark" src="/brand/quilldns-mark.svg" width="42" height="42" alt="" aria-hidden="true" />
          <span class="brand-name">Quill<span class="brand-name-accent">DNS</span></span>
        </div>
        <div class="nav-heading">Workspace</div>
        <nav class="nav" aria-label="Main navigation">
          <button
            v-for="item in navigation"
            :key="item.key"
            :class="['nav-item', { active: page === item.key }]"
            :aria-current="page === item.key ? 'page' : undefined"
            @click="page = item.key"
          >
            <UIcon :name="item.icon" class="size-4" /> {{ item.label }}
          </button>
        </nav>
        <div class="nav-heading">Infrastructure</div>
        <div class="connection"><span class="dot" /> QuillDNS instance
          <div class="connection-sub">Self-hosted · Go powered</div>
        </div>
        <div class="sidebar-bottom">
          <a class="sidebar-project" href="https://github.com/anxuanzi/QuillDNS" target="_blank" rel="noopener noreferrer">
            <UIcon name="i-lucide-github" class="size-4" />
            <span>View on GitHub</span>
            <UIcon name="i-lucide-arrow-up-right" class="size-3" />
          </a>
          <div class="side-note">DNS, under your control.</div>
          <div class="side-sub">Open source · Apache-2.0</div>
        </div>
      </aside>

      <main class="main">
        <header class="top-header">
          <div>
            <div class="eyebrow">Your DNS workspace <span class="eyebrow-divider">/</span> {{ current.label }}</div>
            <h1 class="h1">{{ current.label }}</h1>
            <p class="subheading">Manage and understand your DNS infrastructure in one place.</p>
          </div>
          <div class="header-actions">
            <UBadge color="success" variant="subtle"><UIcon name="i-lucide-shield-check" class="size-4" /> Self-hosted</UBadge>
            <UButton
              color="neutral"
              variant="outline"
              :icon="mode === 'light' ? 'i-lucide-moon' : 'i-lucide-sun'"
              :aria-label="mode === 'light' ? 'Switch to dark theme' : 'Switch to light theme'"
              :title="mode === 'light' ? 'Switch to dark theme' : 'Switch to light theme'"
              :aria-pressed="mode === 'dark'"
              @click="toggle"
            />
          </div>
        </header>
        <component :is="views[page]" />
        <footer class="footer">
          <span>© {{ copyrightYear }} QuillDNS contributors · Apache-2.0</span>
          <a href="https://github.com/anxuanzi/QuillDNS" target="_blank" rel="noopener noreferrer"><UIcon name="i-lucide-github" class="size-4" /> GitHub / anxuanzi/QuillDNS <UIcon name="i-lucide-arrow-up-right" class="size-3" /></a>
        </footer>
      </main>
    </div>
  </UApp>
</template>
