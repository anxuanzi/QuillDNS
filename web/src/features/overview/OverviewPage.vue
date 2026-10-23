<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useQuery } from '@pinia/colada'
import { api } from '../../lib/api'
import { openClientQueries } from '../../lib/navigation'
import { AreaChart, LineChart } from 'vue-chrts'
import { useColorMode } from '../../lib/theme'

const { data: metrics, refetch: refreshMetrics, error: metricsError } = useQuery({
  key: ['metrics'],
  query: api.metrics
})
const trendWindow = ref('1h')
const TREND_WINDOWS = [
  { label: 'Last hour', value: '1h' },
  { label: 'Last 24 hours', value: '24h' },
  { label: 'Last 7 days', value: '7d' }
]
const trendSpanLabel = computed(() => trendWindow.value === '1h' ? 'last hour' : trendWindow.value === '24h' ? 'last 24 hours' : 'last 7 days')
const trendBucketLabel = computed(() => {
  const seconds = trend.value?.bucket_seconds ?? 300
  return seconds >= 3600 ? (seconds / 3600) + '-hour buckets' : (seconds / 60) + '-minute intervals'
})
const { data: trend, refetch: refreshTrend, error: trendError } = useQuery({
  key: () => ['metrics', 'trend', trendWindow.value],
  query: () => api.trend(trendWindow.value)
})
const { data: tops, refetch: refreshTops, error: topsError } = useQuery({
  key: ['metrics', 'tops'],
  query: () => api.tops('1h', 8)
})
const { data: resolver, refetch: refreshResolver, error: resolverError } = useQuery({
  key: ['metrics', 'resolver'],
  query: api.resolver
})

const topDomains = computed(() => tops.value?.data.domains ?? [])
const topClients = computed(() => tops.value?.data.clients ?? [])
const topTypes = computed(() => tops.value?.data.types ?? [])
const topRcodes = computed(() => tops.value?.data.rcodes ?? [])
const trimDot = (value: string) => value.replace(/\.$/, '')
const hostOf = (client: string) => client.split(':')[0] || client
const rcodeColor = (rcode: string) => (rcode === 'NOERROR' ? 'success' : rcode === 'NXDOMAIN' ? 'neutral' : 'error')
const topShare = (count: number) => {
  const biggest = topDomains.value[0]?.count ?? 0
  return (biggest ? 100 * count / biggest : 0).toFixed(1)
}
const liveCacheHitRate = computed(() => {
  const cache = resolver.value?.cache
  if (!cache || cache.hits + cache.misses === 0) return '—'
  return (100 * cache.hits / (cache.hits + cache.misses)).toFixed(1) + '%'
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

// The dashboard polls every five seconds; the Live toggle and a hidden tab
// both pause the polling so background dashboards never load the server.
const live = ref(true)
let interval: ReturnType<typeof setInterval> | undefined
onMounted(() => {
  interval = setInterval(() => {
    if (!live.value || document.hidden) return
    void refreshMetrics()
    void refreshTrend()
    void refreshTops()
    void refreshResolver()
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
      { label: 'P95 latency', value: formatMS(metrics?.p95_latency_ms), icon: 'i-lucide-gauge', detail: 'Latest in-memory query window' },
      { label: 'Served from cache', value: cacheHitRate + '%', icon: 'i-lucide-database', detail: 'Share of responses from the local cache' }
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
          <p class="chart-note">Query volume · {{ trendSpanLabel }} · {{ trendBucketLabel }}</p>
        </div>
        <span class="chart-legend">
          <label class="live-toggle" title="Refresh the dashboard every five seconds"><input v-model="live" type="checkbox" aria-label="Auto-refresh dashboard"> Live</label>
          <USelect v-model="trendWindow" :items="TREND_WINDOWS" class="w-36" aria-label="Trend window" />
          <span class="chart-dot" /> Queries
        </span>
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
          <p class="chart-note">Mean DNS processing time · {{ trendSpanLabel }}</p>
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
      <div class="inline-metric" style="margin-top:17px"><span>P99 latency · latest in-memory window</span><strong>{{ formatMS(metrics?.p99_latency_ms) }}</strong></div>
      <div class="inline-metric"><span>Hard errors (SERVFAIL / REFUSED)</span><strong>{{ metrics?.failures ?? 0 }}</strong></div>
      <div class="inline-metric"><span>Name not found (NXDOMAIN)</span><strong>{{ metrics?.nxdomain ?? 0 }}</strong></div>
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

  <section class="grid2" style="margin-top:16px" aria-label="Query analytics and resolver health">
    <div class="panel">
      <h2 class="section-title">Top activity · last hour</h2>
      <p class="subheading" style="margin-top:5px">Most requested domains, busiest clients and response codes</p>
      <div v-if="topsError" class="notice-error" role="alert">Unable to load top activity: {{ topsError.message }}</div>
      <div v-if="!topDomains.length" class="trend-empty" style="padding:26px 0">
        <span>No queries logged in the last hour.</span>
      </div>
      <div v-if="topDomains.length" class="distribution" style="margin:18px 0 8px">
        <div v-for="entry in topDomains" :key="entry.value" class="distribution-item">
          <div class="distribution-label">
            <span class="mono">{{ trimDot(entry.value) }}</span>
            <span>{{ entry.count.toLocaleString() }}</span>
          </div>
          <div class="meter"><div class="meter-fill" :style="{ width: topShare(entry.count) + '%' }" /></div>
        </div>
      </div>
      <div v-if="topClients.length || topTypes.length || topRcodes.length" class="metric-mini-grid" aria-label="Busiest clients, record types and response codes">
        <div class="metric-mini">
          <div class="metric-mini-label">Busiest clients</div>
          <div v-for="entry in topClients" :key="entry.value" class="top-row">
            <button type="button" class="linklike mono" :aria-label="'Show query history for client ' + hostOf(entry.value)" @click="openClientQueries(hostOf(entry.value))">{{ hostOf(entry.value) }}</button>
            <strong>{{ entry.count.toLocaleString() }}</strong>
          </div>
          <div v-if="!topClients.length" class="top-row muted">—</div>
        </div>
        <div class="metric-mini">
          <div class="metric-mini-label">Record types</div>
          <div v-for="entry in topTypes" :key="entry.value" class="top-row">
            <span><UBadge color="neutral" variant="subtle">{{ entry.value }}</UBadge></span>
            <strong>{{ entry.count.toLocaleString() }}</strong>
          </div>
          <div v-if="!topTypes.length" class="top-row muted">—</div>
        </div>
        <div class="metric-mini">
          <div class="metric-mini-label">Response codes</div>
          <div v-for="entry in topRcodes" :key="entry.value" class="top-row">
            <span><UBadge :color="rcodeColor(entry.value)" variant="subtle">{{ entry.value }}</UBadge></span>
            <strong>{{ entry.count.toLocaleString() }}</strong>
          </div>
          <div v-if="!topRcodes.length" class="top-row muted">—</div>
        </div>
      </div>
    </div>

    <div class="panel">
      <h2 class="section-title">Cache &amp; upstream health</h2>
      <p class="subheading" style="margin-top:5px">Live resolver state since the last restart</p>
      <div v-if="resolverError" class="notice-error" role="alert">Unable to load resolver health: {{ resolverError.message }}</div>
      <div class="inline-metric" style="margin-top:17px"><span>Cache hit ratio (hits / lookups)</span><strong>{{ liveCacheHitRate }}</strong></div>
      <div class="inline-metric"><span>Cache entries</span><strong>{{ resolver?.cache.entries ?? 0 }} / {{ resolver?.cache.max ?? 0 }}</strong></div>
      <div class="inline-metric"><span>Cache hits</span><strong>{{ (resolver?.cache.hits ?? 0).toLocaleString() }}</strong></div>
      <div class="inline-metric"><span>Cache misses</span><strong>{{ (resolver?.cache.misses ?? 0).toLocaleString() }}</strong></div>
      <div class="inline-metric"><span>Cache entries expired or evicted</span><strong>{{ (resolver?.cache.evictions ?? 0).toLocaleString() }}</strong></div>
      <div class="upstream-list" aria-label="Upstream resolver health">
        <div v-for="upstream in resolver?.upstreams ?? []" :key="upstream.address" class="inline-metric">
          <span class="upstream-label">
            <span class="dot" :class="upstream.status === 'healthy' ? '' : 'dot-warn'" aria-hidden="true" />
            <span class="mono">{{ upstream.address }}</span>
            <UBadge v-if="upstream.status !== 'healthy'" :color="upstream.status === 'cooling' ? 'error' : 'warning'" variant="subtle">{{ upstream.status }}</UBadge>
          </span>
          <strong>
            {{ upstream.requests.toLocaleString() }} req
            · {{ upstream.errors.toLocaleString() }} err
            · {{ upstream.avg_latency_ms.toFixed(1) }} ms
          </strong>
        </div>
        <div v-if="!resolver?.upstreams.length" class="inline-metric"><span>No upstream resolvers configured</span><strong>—</strong></div>
      </div>
    </div>
  </section>
</template>
