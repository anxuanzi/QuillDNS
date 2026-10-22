<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useQuery } from '@pinia/colada'
import { api } from '../../lib/api'

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
  time: new Date(point.time).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
  queries: point.queries,
  latency: Number(point.average_latency_ms.toFixed(3))
})))
const volumeSeries = [{ key: 'queries', label: 'DNS queries' }]
const latencySeries = [{ key: 'latency', label: 'Avg response time (ms)' }]
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
        <UChart
          v-if="chartData.length"
          type="area"
          :data="chartData"
          index="time"
          :categories="volumeSeries"
          :height="186"
          class="w-full"
          aria-label="Real DNS queries over the last hour"
        />
        <p v-else class="trend-empty">No traffic history collected yet.</p>
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
        <UChart
          v-if="chartData.length"
          type="line"
          :data="chartData"
          index="time"
          :categories="latencySeries"
          :height="186"
          class="w-full"
          aria-label="Average DNS response latency over the last hour"
        />
        <p v-else class="trend-empty">No latency history collected yet.</p>
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
