<script setup lang="ts">
import { computed, ref } from 'vue'
import { useMutation, useQuery, useQueryCache } from '@pinia/colada'
import { api, type DNSRecord } from '../../lib/api'

type Editable = Omit<DNSRecord, 'id'>
const defaultForm = (): Editable => ({ name: '', type: 'A', value: '', ttl: 300, enabled: true, priority: 10, weight: 0, port: 443, flags: 0, tag: "issue" })
const form = ref<Editable>(defaultForm())
const editID = ref<string | null>(null)
const showModal = ref(false)
const search = ref('')
const notice = ref('')
const pending = ref(false)
const queryCache = useQueryCache()
const { data, error, status } = useQuery({ key: ['records'], query: api.records })
const records = computed(() => data.value?.data ?? [])
const filtered = computed(() => records.value.filter(r =>
  (r.name + r.value + r.type).toLowerCase().includes(search.value.toLowerCase())
))
const create = useMutation({
  mutation: api.createRecord,
  onSuccess: async () => { pending.value = false; showModal.value = false; notice.value = 'DNS record created'; await queryCache.invalidateQueries({ key: ['records'] }) },
  onError: e => { pending.value = false; notice.value = e.message }
})
const update = useMutation({
  mutation: (input: { id: string; data: Editable }) => api.updateRecord(input.id, input.data),
  onSuccess: async () => { pending.value = false; showModal.value = false; notice.value = 'DNS record updated'; await queryCache.invalidateQueries({ key: ['records'] }) },
  onError: e => { pending.value = false; notice.value = e.message }
})
const remove = useMutation({
  mutation: api.deleteRecord,
  onSuccess: async () => { notice.value = 'DNS record deleted'; await queryCache.invalidateQueries({ key: ['records'] }) },
  onError: e => { notice.value = e.message }
})
function openCreate() { editID.value = null; form.value = defaultForm(); notice.value = ''; showModal.value = true }
function openEdit(record: DNSRecord) {
  editID.value = record.id
  const { id: _id, ...data } = record
  form.value = { ...defaultForm(), ...data }
  notice.value = ''
  showModal.value = true
}
function submit() {
  if (pending.value) return
  notice.value = ''
  pending.value = true
  const payload = { ...form.value, ttl: Number(form.value.ttl), priority: Number(form.value.priority), weight: Number(form.value.weight), port: Number(form.value.port), flags: Number(form.value.flags ?? 0) }
  if (editID.value) update.mutate({ id: editID.value, data: payload })
  else create.mutate(payload)
}
function deleteRecord(record: DNSRecord) {
  if (window.confirm('Delete ' + record.name + ' (' + record.type + ')?')) remove.mutate(record.id)
}
</script>

<template>
  <section class="toolbar">
    <div>
      <div class="section-title">Your local records</div>
      <p class="subheading">{{ records.length }} configured DNS overrides</p>
    </div>
    <div class="toolbar-controls">
      <UInput v-model="search" icon="i-lucide-search" placeholder="Search records…" />
      <UButton icon="i-lucide-plus" @click="openCreate">Add Record</UButton>
    </div>
  </section>
  <div v-if="notice" role="status" class="notice">{{ notice }}</div>
  <div v-if="error" role="alert" class="notice-error">Unable to load records: {{ error.message }}</div>
  <div class="panel records-panel">
    <table class="table" aria-label="DNS record management">
      <thead><tr><th>Hostname</th><th>Type</th><th>Value</th><th>TTL</th><th>Manage</th></tr></thead>
      <tbody>
        <tr v-for="record in filtered" :key="record.id">
          <td class="hostname">{{ record.name }}</td>
          <td><UBadge color="neutral" variant="subtle">{{ record.type }}</UBadge></td>
          <td class="mono record-value">{{ record.value }}</td>
          <td class="muted">{{ record.ttl }}s</td>
          <td><div class="record-actions">
            <UButton color="neutral" variant="ghost" icon="i-lucide-pencil" :aria-label="'Edit ' + record.name" @click="openEdit(record)" />
            <UButton color="error" variant="ghost" icon="i-lucide-trash-2" :aria-label="'Delete ' + record.name" @click="deleteRecord(record)" />
          </div></td>
        </tr>
        <tr v-if="!filtered.length"><td colspan="5" class="empty-table">
          {{ status === 'pending' ? 'Loading DNS records…' : 'No records found. Add your first DNS override.' }}
        </td></tr>
      </tbody>
    </table>
  </div>
  <p class="footnote">Local answers override public DNS by hostname and record type. Unmatched queries fall back to your upstream resolvers.</p>

  <UModal v-model:open="showModal" :title="editID ? 'Edit DNS record' : 'Add DNS record'" description="Manage your local DNS overrides without restarting the resolver.">
    <template #body>
      <form id="record-editor" class="editor-form" @submit.prevent="submit">
        <div><label class="field-label" for="record-name">Hostname</label><UInput id="record-name" v-model="form.name" placeholder="api.internal.example" class="w-full" required /></div>
        <div class="grid2">
          <div><label class="field-label">Record type</label><USelect v-model="form.type" :items="['A','AAAA','CNAME','TXT','MX','SRV','PTR','CAA']" class="w-full" /></div>
          <div><label class="field-label" for="record-ttl">TTL (seconds)</label><UInput id="record-ttl" v-model.number="form.ttl" type="number" min="1" max="86400" class="w-full" /></div>
        </div>
        <div>
          <label class="field-label" for="record-value">{{ form.type === 'TXT' ? 'Text value' : form.type === 'CAA' ? 'Issuer or policy value' : ['A','AAAA'].includes(form.type) ? 'IP address' : 'Target hostname' }}</label>
          <UTextarea v-if="form.type === 'TXT'" id="record-value" v-model="form.value" class="w-full" />
          <UInput v-else id="record-value" v-model="form.value" class="w-full" required :placeholder="form.type === 'A' ? '10.0.0.20' : form.type === 'AAAA' ? '2001:db8::20' : 'target.example.com'" />
        </div>
        <div v-if="form.type === 'CAA'" class="grid2">
          <div><label class="field-label" for="caa-tag">CAA property tag</label><USelect id="caa-tag" v-model="form.tag" :items="['issue','issuewild','iodef']" class="w-full" /></div>
          <div><label class="field-label" for="caa-flags">CAA flags</label><UInput id="caa-flags" v-model.number="form.flags" type="number" min="0" max="255" class="w-full" /></div>
        </div>
        <div v-if="form.type === 'MX' || form.type === 'SRV'" class="grid2">
          <div><label class="field-label">Priority</label><UInput v-model.number="form.priority" type="number" min="0" max="65535" class="w-full" /></div>
          <div v-if="form.type === 'SRV'"><label class="field-label">Weight</label><UInput v-model.number="form.weight" type="number" min="0" max="65535" class="w-full" /></div>
          <div v-if="form.type === 'SRV'"><label class="field-label">Port</label><UInput v-model.number="form.port" type="number" min="0" max="65535" class="w-full" /></div>
        </div>
        <label class="enabled-row"><USwitch v-model="form.enabled" /> Enabled</label>
      </form>
    </template>
    <template #footer>
      <div class="modal-actions">
        <UButton color="neutral" variant="outline" @click="showModal = false">Cancel</UButton>
        <UButton type="submit" form="record-editor" :loading="pending">{{ editID ? 'Save Changes' : 'Create Record' }}</UButton>
      </div>
    </template>
  </UModal>
</template>
