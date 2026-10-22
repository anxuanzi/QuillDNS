<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useQuery } from '@pinia/colada'
import { api } from '../../lib/api'
import { AreaChart, LineChart } from 'vue-chrts'
import { useColorMode } from '../../lib/theme'

const { data: metrics, refetch: refreshMetrics, error: metricsError } = useQuery({
  key: ['metrics'],
  query: api.metrics
})
const { data: trend, refetch: refreshTrend, error: trendError } = useQuery({
  key: ['metrics', 'trend'],
  query: api.trend
})

const cacheHitRate = computed(() => metrics.value?.queries
  ? (100 * (metrics.value.cache ?? 0) / metrics.value.queries).toFixed(1)
  : '0.0')
const chartData = computed(() => (trend.value?.data ?? []).map(point => ({
  time: new Date(point.time).toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit', hour12: false }),
  queries: point.queries,
  latency: Number(point.average_latency_ms.toFixed(3))
})))
const { mode } = useColorMode()
const volumeCategories = computed(() => ({
  queries: { name: 'DNS queries', color: mode.value === 'dark' ? '#65d6e6' : '#098ca3' }
}))
const latencyCategories = computed(() => ({
  latency: { name: 'Mean processing time (ms)', color: mode.value === 'dark' ? '#a9a7ff' : '#686fe0' }
}))
const hasActivity = computed(() => chartData.value.some(item => item.queries > 0))
const trafficDomain = computed<[number, number]>(() => [
  0, Math.max(1, Math.ceil(Math.max(0, ...chartData.value.map(point => point.queries)) * 1.18))
])
const latencyDomain = computed<[number, number]>(() => [
  0, Math.max(0.01, Math.max(0, ...chartData.value.map(point => point.latency)) * 1.18)
])
const timeTick = (tick: number | Date): string => chartData.value[Math.floor(Number(tick))]?.time ?? ''
const formatMS = (value: number | undefined) => (value ?? 0).toFixed(2) + ' ms'

let interval: ReturnType<typeof setInterval> | undefined
onMounted(() => {
  interval = setInterval(() => {
    void refreshMetrics()
    void refreshTrend()
  }, 5000)
})
onUnmounted(() => { if (interval) clearInterval(interval) })

const domain = ref('example.com')
const type = ref('A')
const lookupResult = ref<{ rcode: string; answers: string[]; duration_ms: number } | null>(null)
const lookupError = ref('')
const busy = ref(false)
async function lookup() {
  if (busy.value) return
  busy.value = true
  lookupError.value = ''
  lookupResult.value = null
  try {
    lookupResult.value = await api.lookup(domain.value, type.value)
    // A lookup is a real resolver operation; refresh the dashboard so metrics stay current.
    void refreshMetrics()
  } catch (error) {
    lookupError.value = error instanceof Error ? error.message : 'Lookup failed'
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <section class="kpis" aria-label="DNS performance">
    <div v-for="item in [
      { label: 'Total queries', value: (metrics?.queries ?? 0).toLocaleString(), icon: 'i-lucide-radio-tower', detail: 'Since the last restart' },
      { label: 'Average response time', value: formatMS(metrics?.average_latency_ms), icon: 'i-lucide-timer', detail: 'Overall DNS processing' },
      { label: 'P95 latency', value: formatMS(metrics?.p95_latency_ms), icon: 'i-lucide-gauge', detail: 'Rolling recent query window' },
      { label: 'Cache hit rate', value: cacheHitRate + '%', icon: 'i-lucide-database', detail: 'Responses from the local cache' }
    ]" :key="item.label" class="panel kpi">
      <div class="kpi-head">
        <span class="kpi-label">{{ item.label }}</span>
        <UIcon :name="item.icon" class="size-5 kpi-icon" />
      </div>
      <div class="kpi-value">{{ item.value }}</div>
      <div class="kpi-detail">{{ item.detail }}</div>
    </div>
  </section>
  <div v-if="metricsError" class="notice-error" role="alert">Unable to fetch live statistics: {{ metricsError.message }}</div>

  <section class="grid2" aria-label="DNS performance charts">
    <div class="panel">
      <div class="chart-head">
        <div>
          <h2 class="section-title">DNS traffic</h2>
          <p class="chart-note">Query volume · last 60 minutes · 5-minute intervals</p>
        </div>
        <span class="chart-legend"><span class="chart-dot" /> Queries</span>
      </div>
      <div class="chart-frame">
        <AreaChart
          v-if="hasActivity"
          :data="chartData"
          :categories="volumeCategories"
          :y-domain="trafficDomain"
          :height="192"
          :x-formatter="timeTick"
          :x-num-ticks="4"
          :y-grid-line="true"
          :hide-legend="true"
          :duration="0"
          aria-label="DNS query volume by five-minute interval"
        />
        <div v-else class="trend-empty">
          <UIcon name="i-lucide-chart-no-axes-column" class="size-6" />
          <span>No DNS traffic yet</span>
          <small>Your first queries will appear here automatically.</small>
        </div>
      </div>
    </div>
    <div class="panel">
      <div class="chart-head">
        <div>
          <h2 class="section-title">Response latency</h2>
          <p class="chart-note">Mean DNS processing time · last 60 minutes</p>
        </div>
        <span class="chart-legend"><span class="chart-dot" /> Milliseconds</span>
      </div>
      <div class="chart-frame">
        <LineChart
          v-if="hasActivity"
          :data="chartData"
          :categories="latencyCategories"
          :y-domain="latencyDomain"
          :height="192"
          :x-formatter="timeTick"
          :x-num-ticks="4"
          :y-grid-line="true"
          :hide-legend="true"
          :duration="0"
          aria-label="Average DNS processing latency by five-minute interval"
        />
        <div v-else class="trend-empty">
          <UIcon name="i-lucide-timer" class="size-6" />
          <span>No latency samples yet</span>
          <small>Processing time is measured from real DNS requests.</small>
        </div>
      </div>
    </div>
  </section>
  <div v-if="trendError" role="alert" class="notice-error">Unable to load historical metrics: {{ trendError.message }}</div>

  <section class="grid2" style="margin-top:16px">
    <div class="panel">
      <h2 class="section-title">Resolver activity</h2>
      <p class="subheading" style="margin-top:5px">Where your answers come from</p>
      <div class="distribution">
        <div v-for="item in [
          { label: 'Local overrides', value: metrics?.local ?? 0 },
          { label: 'Response cache', value: metrics?.cache ?? 0 },
          { label: 'Public upstreams', value: metrics?.upstream ?? 0 }
        ]" :key="item.label" class="distribution-item">
          <div class="distribution-label">
            <span>{{ item.label }}</span>
            <span>{{ (metrics?.queries ? item.value / metrics.queries * 100 : 0).toFixed(1) }}%</span>
          </div>
          <div class="meter">
            <div class="meter-fill" :style="{ width: (metrics?.queries ? item.value / metrics.queries * 100 : 0) + '%' }" />
          </div>
        </div>
      </div>
      <div class="metric-mini-grid" aria-label="Average processing time by resolution source">
        <div v-for="item in [
          { label: 'Local', value: metrics?.local_avg_latency_ms },
          { label: 'Cache', value: metrics?.cache_avg_latency_ms },
          { label: 'Upstream', value: metrics?.upstream_avg_latency_ms }
        ]" :key="item.label" class="metric-mini">
          <div class="metric-mini-label">{{ item.label }}</div>
          <strong class="metric-mini-value">{{ formatMS(item.value) }}</strong>
        </div>
      </div>
      <div class="inline-metric" style="margin-top:17px"><span>P99 response latency · rolling</span><strong>{{ formatMS(metrics?.p99_latency_ms) }}</strong></div>
      <div class="inline-metric"><span>Errors / non-success responses</span><strong>{{ metrics?.errors ?? 0 }}</strong></div>
      <div class="inline-metric"><span>Dropped query-log events</span><strong>{{ metrics?.dropped_logs ?? 0 }}</strong></div>
    </div>

    <div class="panel">
      <h2 class="section-title">DNS lookup</h2>
      <p class="subheading" style="margin-top:5px">Test how your active DNS resolver responds</p>
      <form class="lookup-form" @submit.prevent="lookup">
        <label class="field-label" for="lookup-name">Domain or hostname</label>
        <UInput id="lookup-name" v-model="domain" placeholder="example.com" size="lg" class="w-full" required />
        <label class="field-label" for="lookup-type">Record type</label>
        <div class="lookup-row">
          <USelect v-model="type" :items="['A','AAAA','CNAME','TXT','MX','SRV','PTR','CAA']" class="w-36" />
          <UButton type="submit" icon="i-lucide-search" block :loading="busy" :disabled="busy">Resolve</UButton>
        </div>
      </form>
      <div v-if="lookupResult" class="lookup-result" aria-live="polite">
        <div class="lookup-result-meta">
          <UBadge :color="lookupResult.rcode === 'NOERROR' ? 'success' : 'warning'" variant="subtle">{{ lookupResult.rcode }}</UBadge>
          <span class="lookup-latency"><UIcon name="i-lucide-timer" class="size-3" /> {{ formatMS(lookupResult.duration_ms) }} processing time</span>
        </div>
        <div v-for="answer in lookupResult.answers" :key="answer" class="mono result-answer">{{ answer }}</div>
        <div v-if="!lookupResult.answers.length" class="muted">No answer records returned.</div>
      </div>
      <div v-if="lookupError" class="notice-error" role="alert">{{ lookupError }}</div>
      <p class="footnote">Lookup timings include proxy processing and any upstream wait; they are not network round-trip measurements from your browser.</p>
    </div>
  </section>
</template>
