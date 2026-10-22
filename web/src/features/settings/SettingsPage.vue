<script setup lang="ts">
import { useQuery } from '@pinia/colada'
import { api } from '../../lib/api'
const { data: session, error } = useQuery({ key: ['session'], query: api.session })
async function logout() {
  await api.logout()
  window.location.assign('/auth/login')
}
</script>

<template>
  <section class="settings-layout">
    <div class="panel">
      <div class="settings-icon"><UIcon name="i-lucide-shield-check" class="size-6" /></div>
      <h2 class="section-title">Access and authentication</h2>
      <p class="subheading" style="margin-top:10px">OpenID Connect is optional. When disabled, the management service is restricted to loopback.</p>
      <div class="inline-metric" style="margin-top:24px">
        <span>Authentication mode</span>
        <UBadge :color="session?.authenticated ? 'success' : 'neutral'" variant="subtle">{{ session?.authenticated ? 'OIDC active' : 'Local access' }}</UBadge>
      </div>
      <div v-if="session?.email" class="inline-metric"><span>Signed-in user</span><strong>{{ session.email }}</strong></div>
      <div v-if="session?.authenticated" style="margin-top:20px"><UButton icon="i-lucide-log-out" color="neutral" variant="outline" @click="logout">Sign out</UButton></div>
      <p v-if="error" class="notice-error">{{ error.message }}</p>
    </div>
    <div class="panel">
      <div class="settings-icon"><UIcon name="i-lucide-server" class="size-6" /></div>
      <h2 class="section-title">Resolver configuration</h2>
      <p class="subheading" style="margin-top:10px">Upstream servers and network listeners are currently managed with startup flags. GUI-based configuration is planned.</p>
      <div class="inline-metric" style="margin-top:24px"><span>DNS transport</span><strong>UDP + TCP</strong></div>
      <div class="inline-metric"><span>Telemetry</span><strong>Built-in SQLite</strong></div>
      <div class="inline-metric"><span>DNS record storage</span><strong>SQLite</strong></div>
    </div>
  </section>
</template>
