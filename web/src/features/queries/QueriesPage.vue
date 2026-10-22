<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useQuery } from '@pinia/colada'
import { api } from '../../lib/api'

const search = ref('')
const sourceFilter = ref('all')
const { data, error, refetch } = useQuery({ key: ['queries'], query: api.queries })
const { data: metrics } = useQuery({ key: ['metrics'], query: api.metrics })
const events = computed(() => data.value?.data ?? [])
const filtered = computed(() => events.value.filter(item =>
  (sourceFilter.value === 'all' || item.source === sourceFilter.value)
  && (item.name + item.type + item.source + item.client + item.rcode).toLowerCase().includes(search.value.toLowerCase())
))
const ms = (number: number | undefined) => (number ?? 0).toFixed(2) + ' ms'
const latencyClass = (value: number): string => value > 100 ? 'slow' : value > 25 ? 'medium' : ''
let interval: ReturnType<typeof setInterval> | undefined
onMounted(() => { interval = setInterval(() => { void refetch() }, 5000) })
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
      <h2 class="section-title">Recent query history</h2>
      <p class="subheading">Search names, clients, resolution sources and response timing</p>
    </div>
    <div class="toolbar-controls">
      <UInput v-model="search" icon="i-lucide-search" placeholder="Filter by domain, type, client…" aria-label="Filter DNS queries" />
      <USelect v-model="sourceFilter" :items="['all', 'local', 'cache', 'upstream']" aria-label="Filter by resolution source" class="w-36" />
    </div>
  </section>
  <div v-if="error" role="alert" class="notice-error">Unable to load query history: {{ error.message }}</div>
  <div class="panel records-panel">
    <table class="table" aria-label="DNS query history">
      <thead><tr><th>Timestamp</th><th>Hostname</th><th>Type</th><th>Source</th><th>Response</th><th>Processing time</th></tr></thead>
      <tbody>
        <tr v-for="(item, index) in filtered" :key="item.time + item.name + index">
          <td class="muted">{{ new Date(item.time).toLocaleString() }}</td>
          <td class="hostname">{{ item.name }}</td>
          <td><UBadge color="neutral" variant="subtle">{{ item.type }}</UBadge></td>
          <td><UBadge :color="item.source === 'local' ? 'success' : item.source === 'cache' ? 'info' : 'neutral'" variant="subtle">{{ item.source }}</UBadge></td>
          <td class="mono">{{ item.rcode }}</td>
          <td><span :class="['latency-value', latencyClass(item.duration_ms)]">{{ ms(item.duration_ms) }}</span></td>
        </tr>
        <tr v-if="!filtered.length"><td colspan="6" class="empty-table">No matching queries in your recent history.</td></tr>
      </tbody>
    </table>
  </div>
  <p class="footnote">Times measure the DNS proxy's handling of each request, including any upstream wait. They exclude client-to-server network latency. P95 and P99 refer to the latest in-memory query window.</p>
</template>
