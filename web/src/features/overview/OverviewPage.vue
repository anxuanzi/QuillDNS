<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { useQuery } from '@pinia/colada'
import { api } from '../../lib/api'

const { data: metrics, refetch, error } = useQuery({ key: ['metrics'], query: api.metrics })
let interval: ReturnType<typeof setInterval> | undefined
onMounted(() => { interval = setInterval(() => { void refetch() }, 5000) })
onUnmounted(() => { if (interval) clearInterval(interval) })

const domain = ref('example.com')
const type = ref('A')
const lookupResult = ref<{ rcode: string; answers: string[] } | null>(null)
const lookupError = ref('')
const busy = ref(false)
async function lookup() {
  busy.value = true
  lookupError.value = ''
  lookupResult.value = null
  try { lookupResult.value = await api.lookup(domain.value, type.value) }
  catch (error) { lookupError.value = error instanceof Error ? error.message : 'Lookup failed' }
  finally { busy.value = false }
}
</script>

<template>
  <section class="kpis" aria-label="DNS performance">
    <div v-for="item in [
      { label: 'Total queries', value: metrics?.queries ?? 0, icon: 'i-lucide-radio-tower', detail: 'Since startup' },
      { label: 'Local answers', value: metrics?.local ?? 0, icon: 'i-lucide-server', detail: 'From your DNS records' },
      { label: 'Cache answers', value: metrics?.cache ?? 0, icon: 'i-lucide-database', detail: 'Without upstream round-trip' },
      { label: 'Upstream answers', value: metrics?.upstream ?? 0, icon: 'i-lucide-globe-2', detail: 'From public DNS' }
    ]" :key="item.label" class="panel kpi">
      <div class="kpi-head"><span class="kpi-label">{{ item.label }}</span><UIcon :name="item.icon" class="size-5 kpi-icon" /></div>
      <div class="kpi-value">{{ item.value.toLocaleString() }}</div>
      <div class="kpi-detail">{{ item.detail }}</div>
    </div>
  </section>

  <div v-if="error" class="notice-error" role="alert">Unable to fetch live statistics: {{ error.message }}</div>

  <section class="grid2">
    <div class="panel">
      <div class="section-title">Resolver activity</div>
      <p class="subheading" style="margin-top:5px">Real-time DNS response distribution</p>
      <div class="distribution">
        <div v-for="item in [
          { label: 'Local overrides', value: metrics?.local ?? 0 },
          { label: 'Response cache', value: metrics?.cache ?? 0 },
          { label: 'Public upstreams', value: metrics?.upstream ?? 0 }
        ]" :key="item.label" class="distribution-item">
          <div class="distribution-label"><span>{{ item.label }}</span><span>{{ (metrics?.queries ? item.value / metrics.queries * 100 : 0).toFixed(1) }}%</span></div>
          <div class="meter"><div class="meter-fill" :style="{ width: (metrics?.queries ? item.value / metrics.queries * 100 : 0) + '%' }" /></div>
        </div>
      </div>
      <div class="inline-metric"><span>Average response latency</span><strong>{{ (metrics?.average_latency_ms ?? 0).toFixed(2) }} ms</strong></div>
      <div class="inline-metric"><span>Errors / non-NOERROR responses</span><strong>{{ metrics?.errors ?? 0 }}</strong></div>
      <div class="inline-metric"><span>Dropped query-log events</span><strong>{{ metrics?.dropped_logs ?? 0 }}</strong></div>
    </div>

    <div class="panel">
      <div class="section-title">DNS lookup</div>
      <p class="subheading" style="margin-top:5px">Test your active resolver from the control plane</p>
      <form class="lookup-form" @submit.prevent="lookup">
        <label class="field-label" for="lookup-name">Domain or hostname</label>
        <UInput id="lookup-name" v-model="domain" placeholder="example.com" size="lg" class="w-full" required />
        <label class="field-label" for="lookup-type">Record type</label>
        <div class="lookup-row">
          <USelect v-model="type" :items="['A','AAAA','CNAME','TXT','MX','SRV','PTR']" class="w-36" />
          <UButton type="submit" icon="i-lucide-search" block :loading="busy">Resolve</UButton>
        </div>
      </form>
      <div v-if="lookupResult" class="lookup-result">
        <UBadge :color="lookupResult.rcode === 'NOERROR' ? 'success' : 'warning'" variant="subtle">{{ lookupResult.rcode }}</UBadge>
        <div v-for="answer in lookupResult.answers" :key="answer" class="mono result-answer">{{ answer }}</div>
        <div v-if="!lookupResult.answers.length" class="muted">No answer records returned.</div>
      </div>
      <div v-if="lookupError" class="notice-error" role="alert">{{ lookupError }}</div>
    </div>
  </section>
</template>
