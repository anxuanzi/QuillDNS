<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useQuery } from '@pinia/colada'
import { api, type QueryEvent, type QueryLogFilter } from '../../lib/api'

const PAGE_SIZE = 100
const WINDOWS: Array<{ label: string, value: string, seconds: number | null }> = [
  { label: 'Last hour', value: '1h', seconds: 3600 },
  { label: 'Last 6 hours', value: '6h', seconds: 21600 },
  { label: 'Last 24 hours', value: '24h', seconds: 86400 },
  { label: 'Last 7 days', value: '7d', seconds: 604800 },
  { label: 'All time', value: 'all', seconds: null }
]

const search = ref('')
const debouncedSearch = ref('')
const sourceFilter = ref('all')
const rcodeFilter = ref('all')
const typeFilter = ref('all')
const windowFilter = ref('24h')
const newestFirst = ref(true)
const live = ref(true)

const rows = ref<QueryEvent[]>([])
const total = ref<number | null>(null)
const loading = ref(false)
const loadError = ref<string | null>(null)
const page = ref(1)
// Cursors[i] carries the keyset cursor that fetches page i+2.
const cursors = ref<number[]>([])

const { data: metrics } = useQuery({ key: ['metrics'], query: api.metrics })

const filter = computed<QueryLogFilter>(() => {
  const applied: QueryLogFilter = { sort: newestFirst.value ? 'desc' : 'asc', limit: PAGE_SIZE }
  if (debouncedSearch.value) applied.q = debouncedSearch.value
  if (sourceFilter.value !== 'all') applied.source = sourceFilter.value
  if (rcodeFilter.value !== 'all') applied.rcode = rcodeFilter.value
  if (typeFilter.value !== 'all') applied.type = typeFilter.value
  const window = WINDOWS.find(item => item.value === windowFilter.value)
  if (window?.seconds) applied.since = new Date(Date.now() - window.seconds * 1000).toISOString()
  if (page.value > 1) applied.cursor = cursors.value[page.value - 2]
  if (page.value === 1) applied.count = true
  return applied
})

const exportHref = computed(() => api.queriesExportPath({ ...filter.value, cursor: undefined, count: false }))

let controller: AbortController | undefined
let searchTimer: ReturnType<typeof setTimeout> | undefined
let interval: ReturnType<typeof setInterval> | undefined

async function load() {
  controller?.abort()
  const attempt = new AbortController()
  controller = attempt
  loading.value = true
  loadError.value = null
  try {
    const result = await api.queries({ ...filter.value })
    if (attempt !== controller) return
    rows.value = result.data
    if (result.total !== undefined) total.value = result.total
  } catch (error) {
    if (attempt !== controller) return
    loadError.value = (error as Error).message
  } finally {
    if (attempt === controller) loading.value = false
  }
}

function applyFilters() {
  page.value = 1
  cursors.value = []
  void load()
}

watch(search, value => {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => { debouncedSearch.value = value.trim() }, 300)
})
watch([debouncedSearch, sourceFilter, rcodeFilter, typeFilter, windowFilter, newestFirst], applyFilters)
watch(page, () => { void load() })

const atFirstPage = computed(() => page.value === 1)
const hasNextPage = computed(() => rows.value.length === PAGE_SIZE)
const countLabel = computed(() => {
  if (total.value === null) return ''
  return total.value.toLocaleString() + ' matching quer' + (total.value === 1 ? 'y' : 'ies')
})
const pageLabel = computed(() => 'Page ' + page.value.toLocaleString() + (total.value !== null && atFirstPage.value ? ' · ' + countLabel.value : ''))

const rcodeClass = (rcode: string): string => {
  if (rcode === 'NXDOMAIN' || rcode === 'FORMERR' || rcode === 'NOTIMP') return 'rcode-warn'
  if (rcode === 'SERVFAIL' || rcode === 'REFUSED') return 'rcode-fail'
  return ''
}

const ms = (number: number | undefined) => (number ?? 0).toFixed(2) + ' ms'
const latencyClass = (value: number): string => value > 100 ? 'slow' : value > 25 ? 'medium' : ''
const hostOf = (client: string) => client.split(':')[0] || client
const withoutDot = (name: string) => name.replace(/\.$/, '')

onMounted(() => {
  void load()
  interval = setInterval(() => { if (live.value && page.value === 1) void load() }, 5000)
})
onUnmounted(() => { if (interval) clearInterval(interval) })
</script>

<template>
  <section class="metric-mini-grid" style="margin-top:28px" aria-label="Query latency statistics">
    <div v-for="entry in [
      { label: 'Average processing time', value: metrics?.average_latency_ms },
      { label: 'P95 recent latency', value: metrics?.p95_latency_ms },
      { label: 'P99 recent latency', value: metrics?.p99_latency_ms }
    ]" :key="entry.label" class="metric-mini">
      <div class="metric-mini-label">{{ entry.label }}</div>
      <strong class="metric-mini-value">{{ ms(entry.value) }}</strong>
    </div>
  </section>
  <section class="toolbar">
    <div>
      <h2 class="section-title">Query history</h2>
      <p class="subheading">Server-side search over the full retained log — page through millions of rows without loading them</p>
    </div>
    <div class="toolbar-controls">
      <UInput v-model="search" icon="i-lucide-search" placeholder="Search domains, clients, IPs…" aria-label="Search DNS queries" :disabled="loading" />
      <USelect v-model="windowFilter" :items="WINDOWS.map(({ label, value }) => ({ label, value }))" class="w-40" aria-label="Time window" />
      <USelect v-model="sourceFilter" :items="['all', 'local', 'cache', 'upstream']" aria-label="Filter by resolution source" class="w-32" />
      <USelect v-model="rcodeFilter" :items="['all', 'NOERROR', 'NXDOMAIN', 'SERVFAIL', 'REFUSED', 'FORMERR', 'NOTIMP']" aria-label="Filter by response code" class="w-36" />
      <USelect v-model="typeFilter" :items="['all', 'A', 'AAAA', 'CNAME', 'TXT', 'MX', 'SRV', 'PTR', 'CAA']" aria-label="Filter by record type" class="w-28" />
      <UButton icon="i-lucide-arrow-down-up" color="neutral" variant="outline" :aria-label="newestFirst ? 'Newest first. Switch to oldest first.' : 'Oldest first. Switch to newest first'" @click="newestFirst = !newestFirst">
        {{ newestFirst ? 'Newest' : 'Oldest' }}
      </UButton>
      <UButton icon="i-lucide-download" color="neutral" variant="outline" :to="exportHref" aria-label="Export filtered query history as CSV">CSV</UButton>
      <label class="live-toggle"><input v-model="live" type="checkbox"> Live</label>
    </div>
  </section>
  <div v-if="loadError" role="alert" class="notice-error">Unable to load query history: {{ loadError }}</div>
  <div class="panel records-panel">
    <table class="table" aria-label="DNS query history">
      <thead><tr><th>Timestamp</th><th>Hostname</th><th>Type</th><th>Client</th><th>Source</th><th>Response</th><th>Processing time</th></tr></thead>
      <tbody>
        <tr v-for="(item, index) in rows" :key="(item.id ?? item.time) + item.name + index">
          <td class="muted">{{ new Date(item.time).toLocaleString() }}</td>
          <td class="hostname" :title="item.name">{{ withoutDot(item.name) }}</td>
          <td><UBadge color="neutral" variant="subtle">{{ item.type }}</UBadge></td>
          <td class="mono">{{ hostOf(item.client) }}</td>
          <td><UBadge :color="item.source === 'local' ? 'success' : item.source === 'cache' ? 'info' : 'neutral'" variant="subtle">{{ item.source }}</UBadge></td>
          <td><span class="mono" :class="rcodeClass(item.rcode)">{{ item.rcode }}</span></td>
          <td><span :class="['latency-value', latencyClass(item.duration_ms)]">{{ ms(item.duration_ms) }}</span></td>
        </tr>
        <tr v-if="!rows.length"><td colspan="7" class="empty-table">{{ loading ? 'Loading query history…' : 'No matching queries in the selected window.' }}</td></tr>
      </tbody>
    </table>
  </div>
  <div class="pagination-row">
    <UButton icon="i-lucide-chevron-left" color="neutral" variant="outline" :disabled="atFirstPage || loading" @click="page = page - 1">Previous</UButton>
    <span class="muted" aria-live="polite">{{ loading ? 'Loading…' : pageLabel }}</span>
    <UButton trailing-icon="i-lucide-chevron-right" color="neutral" variant="outline" :disabled="!hasNextPage || loading" @click="page = page + 1">Next</UButton>
  </div>
  <p class="footnote">Times measure the DNS proxy's handling of each request, including any upstream wait; they exclude client-to-server network latency. History is paginated server-side, so paging stays fast no matter how much has been logged. The count shown with the first page is refreshed with the filters. CSV downloads are capped at 50,000 rows — narrow the filters for larger exports.</p>
</template>
