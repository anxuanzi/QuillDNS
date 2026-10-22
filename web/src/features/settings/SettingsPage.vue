<script setup lang="ts">
import { ref, watch } from 'vue'
import { useQuery, useQueryCache } from '@pinia/colada'
import { api } from '../../lib/api'

const { data: session, error: sessionError } = useQuery({ key: ['session'], query: api.session })
const { data: upstreams, error: upstreamError } = useQuery({ key: ['upstreams'], query: api.upstreams })
const queryCache = useQueryCache()
const addresses = ref<string[]>([])
const edited = ref(false)
const feedback = ref('')
const applying = ref(false)
watch(upstreams, value => { if (value && !edited.value) addresses.value = [...value.data] }, { immediate: true })
async function applyUpstreams() {
  if (applying.value || !edited.value || !addresses.value.length) return
  applying.value = true
  feedback.value = ''
  try {
    const result = await api.setUpstreams([...addresses.value])
    addresses.value = [...result.data]
    edited.value = false
    feedback.value = 'Upstream DNS configuration saved and activated'
    await queryCache.invalidateQueries({ key: ['upstreams'] })
  } catch (error) {
    feedback.value = error instanceof Error ? error.message : 'Unable to save upstreams'
  } finally {
    applying.value = false
  }
}
function addAddress() { addresses.value = [...addresses.value, '']; edited.value = true }
function changeAddress(index: number, value: string) { addresses.value[index] = value; edited.value = true }
function removeAddress(index: number) { addresses.value.splice(index, 1); edited.value = true }
async function logout() {
  try { await api.logout(); window.location.assign('/auth/login') }
  catch (error) { feedback.value = error instanceof Error ? error.message : 'Unable to sign out' }
}
</script>

<template>
  <section class="settings-layout">
    <div class="panel">
      <div class="settings-icon"><UIcon name="i-lucide-globe" class="size-6" /></div>
      <h2 class="section-title">Upstream DNS servers</h2>
      <p class="subheading" style="margin-top:10px">Unmatched queries are forwarded to these public resolvers in priority order. Changes take effect without restarting DNS.</p>
      <div class="editor-form" style="margin-top:22px">
        <label v-for="(item, index) in addresses" :key="index" class="field-label">
          Resolver {{ index + 1 }}
          <div class="resolver-input-row">
            <UInput :model-value="item" placeholder="1.1.1.1:53" class="w-full" @update:model-value="(value: string | number) => changeAddress(index, String(value))" />
            <UButton icon="i-lucide-trash-2" variant="ghost" color="error" :aria-label="'Remove resolver ' + (index + 1)"  :disabled="applying" @click="removeAddress(index)" />
          </div>
        </label>
        <UButton color="neutral" variant="outline" icon="i-lucide-plus"  :disabled="applying" @click="addAddress">Add upstream</UButton>
        <UButton icon="i-lucide-check" :disabled="!edited || !addresses.length || applying" :loading="applying" @click="applyUpstreams">Apply settings</UButton>
        <p v-if="upstreamError" class="notice-error">{{ upstreamError.message }}</p>
        <p v-if="feedback" role="status" class="footnote">{{ feedback }}</p>
        <p class="footnote">Use IPv4:port or [IPv6]:port. Maximum eight resolvers.</p>
      </div>
    </div>
    <div class="panel">
      <div class="settings-icon"><UIcon name="i-lucide-shield-check" class="size-6" /></div>
      <h2 class="section-title">Access and authentication</h2>
      <p class="subheading" style="margin-top:10px">OpenID Connect is optional. When disabled, the management listener is restricted to loopback.</p>
      <div class="inline-metric" style="margin-top:24px">
        <span>Authentication mode</span>
        <UBadge :color="session?.authenticated ? 'success' : 'neutral'" variant="subtle">{{ session?.authenticated ? 'OIDC active' : 'Local access' }}</UBadge>
      </div>
      <div v-if="session?.email" class="inline-metric"><span>Signed-in user</span><strong>{{ session.email }}</strong></div>
      <div class="inline-metric"><span>Query storage</span><strong>Embedded SQLite</strong></div>
      <div class="inline-metric"><span>DNS transport</span><strong>UDP + TCP</strong></div>
      <div v-if="session?.authenticated" style="margin-top:20px"><UButton icon="i-lucide-log-out" color="neutral" variant="outline" @click="logout">Sign out</UButton></div>
      <p v-if="sessionError" class="notice-error">{{ sessionError.message }}</p>
    </div>
  </section>
</template>
