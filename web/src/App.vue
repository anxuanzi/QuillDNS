<script setup lang="ts">
import { computed, ref } from 'vue'
import { useQuery, useMutation, useQueryCache } from '@pinia/colada'
import { api, type DNSRecord } from './lib/api'

type Page = 'overview' | 'records' | 'queries' | 'settings'
const page = ref<Page>('overview')
const tabs: { key: Page, label: string, icon: string }[] = [
  { key: 'overview', label: 'Overview', icon: 'i-lucide-layout-dashboard' },
  { key: 'records', label: 'DNS Records', icon: 'i-lucide-list-tree' },
  { key: 'queries', label: 'Query Logs', icon: 'i-lucide-activity' },
  { key: 'settings', label: 'Settings', icon: 'i-lucide-settings-2' }
]
const queryCache = useQueryCache()
const { data: recordsResponse, status: recordsStatus, error: recordsError } = useQuery({ key: ['records'], query: api.records })
const { data: metrics } = useQuery({ key: ['metrics'], query: api.metrics, refetchInterval: 5000 })
const { data: queryResponse } = useQuery({ key: ['queries'], query: api.queries, refetchInterval: 5000 })
const records = computed(() => recordsResponse.value?.data ?? [])
const queries = computed(() => queryResponse.value?.data ?? [])
const search = ref('')
const filtered = computed(() => records.value.filter(r => (r.name + r.value + r.type).toLowerCase().includes(search.value.toLowerCase())))
const showCreate = ref(false)
const feedback = ref('')
const form = ref({ name: '', type: 'A' as DNSRecord['type'], value: '', ttl: 300, enabled: true, priority: 10, weight: 0, port: 443 })
const createMutation = useMutation({
  mutation: (payload: Omit<DNSRecord, 'id'>) => api.createRecord(payload),
  onSuccess: async () => { showCreate.value = false; feedback.value = 'Record created successfully'; await queryCache.invalidateQueries({ key: ['records'] }) },
  onError: (error) => { feedback.value = error.message }
})
const deleteMutation = useMutation({
  mutation: api.deleteRecord,
  onSuccess: async () => { feedback.value = 'Record deleted'; await queryCache.invalidateQueries({ key: ['records'] }) },
  onError: (error) => { feedback.value = error.message }
})
function submit() {
  feedback.value = ''
  createMutation.mutate({ ...form.value, ttl: Number(form.value.ttl), priority: Number(form.value.priority), weight: Number(form.value.weight), port: Number(form.value.port) })
}
function remove(record: DNSRecord) {
  if (window.confirm('Delete ' + record.name + ' (' + record.type + ')?')) deleteMutation.mutate(record.id)
}
const lookupName = ref('example.com')
const lookupType = ref('A')
const lookupResult = ref('')
async function testLookup() {
  try { const r = await api.lookup(lookupName.value, lookupType.value); lookupResult.value = r.rcode + '\n' + r.answers.join('\n') }
  catch (e) { lookupResult.value = e instanceof Error ? e.message : 'Lookup failed' }
}
</script>

<template>
  <UApp>
    <div class="shell">
      <aside class="sidebar">
        <div class="brand"><span class="brand-mark">✒</span><span>QuillDNS<span style="color:#52dab3">.</span></span></div>
        <div class="nav-heading">Workspace</div>
        <nav class="nav" aria-label="Primary navigation">
          <button v-for="tab in tabs" :key="tab.key" :class="['nav-item', { active: page === tab.key }]" @click="page = tab.key">
            <UIcon :name="tab.icon" class="size-4" /> {{ tab.label }}
          </button>
        </nav>
        <div class="nav-heading">Connection</div>
        <div style="padding:12px 14px;font-size:12px;color:#9bb0c4"><span class="dot" /> Local instance<br><span style="padding-left:15px;color:#61758c">Built-in DNS management</span></div>
      </aside>
      <main class="main">
        <header style="display:flex;justify-content:space-between;gap:20px;align-items:center;flex-wrap:wrap">
          <div><div class="eyebrow">DNS infrastructure</div><h1 class="h1">{{ tabs.find(t => t.key === page)?.label }}</h1><p class="subheading">DNS, under your control.</p></div>
          <UBadge color="success" variant="subtle"><UIcon name="i-lucide-shield-check" class="size-4" /> Self-hosted</UBadge>
        </header>

        <template v-if="page === 'overview'">
          <section class="kpis" aria-label="DNS metrics">
            <div v-for="item in [
              { label: 'Total queries', value: metrics?.queries ?? 0, icon: 'i-lucide-radio-tower' },
              { label: 'Local answers', value: metrics?.local ?? 0, icon: 'i-lucide-server' },
              { label: 'Cache answers', value: metrics?.cache ?? 0, icon: 'i-lucide-database' },
              { label: 'Upstream answers', value: metrics?.upstream ?? 0, icon: 'i-lucide-globe' }
            ]" :key="item.label" class="panel">
              <div class="kpi-label"><UIcon :name="item.icon" class="size-4" /> {{ item.label }}</div><div class="kpi-value">{{ item.value.toLocaleString() }}</div>
            </div>
          </section>
          <section class="grid2">
            <div class="panel"><h2 style="font-size:18px;font-weight:700;margin-bottom:8px">Resolver activity</h2><p class="muted" style="font-size:13px">Live counters since server start</p>
              <div style="margin-top:25px;display:grid;gap:15px">
                <div v-for="entry in [{label:'Local', value:metrics?.local ?? 0},{label:'Cache',value:metrics?.cache ?? 0},{label:'Upstream',value:metrics?.upstream ?? 0}]" :key="entry.label">
                  <div style="display:flex;justify-content:space-between;font-size:12px;margin-bottom:6px"><span>{{ entry.label }}</span><span class="muted">{{ entry.value }}</span></div>
                  <div style="background:#263347;height:7px;border-radius:10px;overflow:hidden"><div :style="{width: (metrics?.queries ? entry.value / metrics.queries * 100 : 0) + '%', background:'#4de0ae',height:'100%',borderRadius:'10px'}"/></div>
                </div>
              </div>
            </div>
            <div class="panel"><h2 style="font-size:18px;font-weight:700;margin-bottom:8px">Quick DNS lookup</h2><p class="muted" style="font-size:13px">Test the resolver directly</p>
              <div style="display:grid;gap:12px;margin-top:22px"><UInput v-model="lookupName" placeholder="example.com" size="lg" />
                <div style="display:flex;gap:10px"><USelect v-model="lookupType" :items="['A','AAAA','TXT','MX','CNAME','SRV']" style="width:125px" /><UButton icon="i-lucide-search" block @click="testLookup">Resolve</UButton></div>
                <pre v-if="lookupResult" style="font-size:12px;white-space:pre-wrap;overflow-wrap:anywhere;color:#89e7c4">{{ lookupResult }}</pre>
              </div>
            </div>
          </section>
        </template>

        <template v-if="page === 'records'">
          <div style="display:flex;justify-content:space-between;gap:12px;align-items:center;margin:28px 0 17px;flex-wrap:wrap">
            <UInput v-model="search" icon="i-lucide-search" placeholder="Search records…" style="width:min(100%,340px)" />
            <UButton icon="i-lucide-plus" @click="showCreate = true">Add DNS Record</UButton>
          </div>
          <div v-if="feedback" role="status" class="panel" style="margin-bottom:14px;padding:12px;color:#88dec1">{{ feedback }}</div>
          <div class="panel" style="overflow:auto">
            <table class="table"><thead><tr><th>Hostname</th><th>Type</th><th>Value</th><th>TTL</th><th></th></tr></thead>
              <tbody>
                <tr v-for="record in filtered" :key="record.id">
                  <td style="font-weight:600">{{ record.name }}</td><td><UBadge color="neutral" variant="soft">{{ record.type }}</UBadge></td>
                  <td class="mono">{{ record.value }}</td><td class="muted">{{ record.ttl }}s</td>
                  <td><UButton color="error" variant="ghost" icon="i-lucide-trash-2" :aria-label="'Delete '+record.name" @click="remove(record)" /></td>
                </tr>
                <tr v-if="!filtered.length"><td colspan="5" class="muted">{{ recordsStatus === 'pending' ? 'Loading records…' : 'No records found. Create your first override.' }}</td></tr>
              </tbody>
            </table>
            <p v-if="recordsError" style="color:#ff9da6;font-size:13px">{{ recordsError.message }}</p>
          </div>
          <UModal v-model:open="showCreate" title="Add DNS Record" description="Local answers take precedence over upstream DNS.">
            <template #body>
              <form id="record-form" style="display:grid;gap:17px" @submit.prevent="submit">
                <div><label class="field-label" for="dns-name">Hostname</label><UInput id="dns-name" v-model="form.name" placeholder="api.example.com" class="w-full" required /></div>
                <div class="grid2">
                  <div><label class="field-label">Record type</label><USelect v-model="form.type" :items="['A','AAAA','CNAME','TXT','MX','SRV','PTR']" class="w-full" /></div>
                  <div><label class="field-label" for="dns-ttl">TTL in seconds</label><UInput id="dns-ttl" v-model.number="form.ttl" type="number" min="1" max="86400" class="w-full" /></div>
                </div>
                <div><label class="field-label" for="dns-value">{{ form.type === 'TXT' ? 'Text content' : form.type === 'A' || form.type === 'AAAA' ? 'IP address' : 'Target / value' }}</label><UTextarea v-if="form.type === 'TXT'" id="dns-value" v-model="form.value" class="w-full" /><UInput v-else id="dns-value" v-model="form.value" class="w-full" required :placeholder="form.type === 'A' ? '10.0.0.20' : form.type === 'AAAA' ? '2001:db8::20' : 'target.example.com'" /></div>
                <div v-if="form.type === 'MX' || form.type === 'SRV'" class="grid2">
                  <div><label class="field-label">Priority</label><UInput v-model.number="form.priority" type="number" min="0" max="65535" class="w-full" /></div>
                  <div v-if="form.type === 'SRV'"><label class="field-label">Weight</label><UInput v-model.number="form.weight" type="number" min="0" max="65535" class="w-full" /></div>
                  <div v-if="form.type === 'SRV'"><label class="field-label">Port</label><UInput v-model.number="form.port" type="number" min="0" max="65535" class="w-full" /></div>
                </div>
              </form>
            </template>
            <template #footer><div style="display:flex;justify-content:flex-end;gap:10px;width:100%"><UButton color="neutral" variant="outline" @click="showCreate = false">Cancel</UButton><UButton type="submit" form="record-form" :loading="createMutation.status.value === 'pending'">Save record</UButton></div></template>
          </UModal>
        </template>

        <template v-if="page === 'queries'">
          <div class="panel" style="margin-top:28px;overflow:auto"><h2 style="font-weight:700;font-size:17px;margin-bottom:10px">Recent DNS queries</h2><p class="muted" style="font-size:12px;margin-bottom:16px">In-memory rolling history; newest first</p>
            <table class="table"><thead><tr><th>Time</th><th>Query</th><th>Type</th><th>Source</th><th>Latency</th></tr></thead><tbody>
              <tr v-for="(event, index) in queries" :key="index"><td class="muted">{{ new Date(event.time).toLocaleTimeString() }}</td><td>{{ event.name }}</td><td>{{ event.type }}</td><td><UBadge :color="event.source==='local'?'success':'neutral'" variant="soft">{{ event.source }}</UBadge></td><td>{{ event.duration_ms.toFixed(2) }}ms</td></tr>
              <tr v-if="queries.length === 0"><td colspan="5" class="muted">No DNS activity recorded yet.</td></tr>
            </tbody></table>
          </div>
        </template>

        <template v-if="page === 'settings'">
          <div class="panel" style="margin-top:28px"><h2 style="font-weight:700;font-size:17px">Server configuration</h2><p class="muted" style="font-size:14px;margin-top:12px">The initial foundation configures listeners and upstreams via CLI flags. UI-based settings and OIDC authentication are planned but are not implemented yet.</p><p class="muted" style="font-size:13px;margin-top:14px">Keep the management interface bound to loopback or protected by a trusted access layer until authentication is available.</p></div>
        </template>
        <footer style="color:#62768d;font-size:12px;border-top:1px solid #233040;margin-top:48px;padding-top:20px;display:flex;justify-content:space-between"><span>QuillDNS • Self-hosted infrastructure</span><span>Made for operators</span></footer>
      </main>
    </div>
  </UApp>
</template>
