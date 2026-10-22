<script setup lang="ts">
import { computed, ref } from 'vue'
import OverviewPage from './features/overview/OverviewPage.vue'
import RecordsPage from './features/records/RecordsPage.vue'
import QueriesPage from './features/queries/QueriesPage.vue'
import SettingsPage from './features/settings/SettingsPage.vue'

type Page = 'overview' | 'records' | 'queries' | 'settings'
const page = ref<Page>('overview')
const navigation: { key: Page, label: string, icon: string }[] = [
  { key: 'overview', label: 'Overview', icon: 'i-lucide-layout-dashboard' },
  { key: 'records', label: 'DNS Records', icon: 'i-lucide-list-tree' },
  { key: 'queries', label: 'Query Logs', icon: 'i-lucide-chart-no-axes-combined' },
  { key: 'settings', label: 'Settings', icon: 'i-lucide-settings-2' }
]
const current = computed(() => navigation.find(item => item.key === page.value)!)
const views = { overview: OverviewPage, records: RecordsPage, queries: QueriesPage, settings: SettingsPage }
</script>

<template>
  <UApp>
    <div class="shell">
      <aside class="sidebar">
        <div class="brand">
          <span class="brand-mark" aria-hidden="true">✒</span>
          <span>QuillDNS<span style="color:#52dab3">.</span></span>
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
          <div class="side-note">DNS, under your control.</div>
          <div class="side-sub">QuillDNS Open Source</div>
        </div>
      </aside>

      <main class="main">
        <header class="top-header">
          <div>
            <div class="eyebrow">Your DNS workspace <span class="eyebrow-divider">/</span> {{ current.label }}</div>
            <h1 class="h1">{{ current.label }}</h1>
            <p class="subheading">Manage and understand your DNS infrastructure in one place.</p>
          </div>
          <UBadge color="success" variant="subtle"><UIcon name="i-lucide-shield-check" class="size-4" /> Self-hosted</UBadge>
        </header>
        <component :is="views[page]" />
        <footer class="footer">
          <span>QuillDNS · Open-source infrastructure</span>
          <span>Built for clarity and control</span>
        </footer>
      </main>
    </div>
  </UApp>
</template>
