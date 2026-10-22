<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useQuery } from '@pinia/colada'
import { api } from '../../lib/api'
const search = ref('')
const { data, error, refetch } = useQuery({ key: ['queries'], query: api.queries })
const events = computed(() => data.value?.data ?? [])
const filtered = computed(() => events.value.filter(item =>
  (item.name + item.type + item.source + item.client + item.rcode).toLowerCase().includes(search.value.toLowerCase())
))
let interval: ReturnType<typeof setInterval> | undefined
onMounted(() => { interval = setInterval(() => { void refetch() }, 5000) })
onUnmounted(() => { if (interval) clearInterval(interval) })
</script>

<template>
  <section class="toolbar">
    <div><div class="section-title">Recent query history</div><p class="subheading">Inspect the latest DNS traffic and resolution sources</p></div>
    <UInput v-model="search" icon="i-lucide-search" placeholder="Filter by domain, type, client…" />
  </section>
  <div v-if="error" role="alert" class="notice-error">Unable to load query history: {{ error.message }}</div>
  <div class="panel records-panel">
    <table class="table" aria-label="DNS query history">
      <thead><tr><th>Timestamp</th><th>Hostname</th><th>Type</th><th>Source</th><th>Response</th><th>Latency</th></tr></thead>
      <tbody>
        <tr v-for="(item, index) in filtered" :key="item.time + item.name + index">
          <td class="muted">{{ new Date(item.time).toLocaleString() }}</td>
          <td class="hostname">{{ item.name }}</td>
          <td><UBadge color="neutral" variant="subtle">{{ item.type }}</UBadge></td>
          <td><UBadge :color="item.source === 'local' ? 'success' : item.source === 'cache' ? 'info' : 'neutral'" variant="subtle">{{ item.source }}</UBadge></td>
          <td class="mono">{{ item.rcode }}</td>
          <td>{{ item.duration_ms.toFixed(2) }} ms</td>
        </tr>
        <tr v-if="!filtered.length"><td colspan="6" class="empty-table">No matching queries in your recent history.</td></tr>
      </tbody>
    </table>
  </div>
  <p class="footnote">Logs are stored locally, are never sent to an external telemetry service, and are automatically retained within configured storage limits.</p>
</template>
