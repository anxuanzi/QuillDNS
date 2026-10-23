export interface DNSRecord {
  id: string
  name: string
  type: 'A' | 'AAAA' | 'TXT' | 'CNAME' | 'MX' | 'SRV' | 'PTR' | 'CAA'
  value: string
  ttl: number
  enabled: boolean
  priority?: number
  weight?: number
  port?: number
  flags?: number
  tag?: string
}
export interface QueryEvent {
  id?: number
  time: string
  name: string
  type: string
  client: string
  source: string
  rcode: string
  duration_ms: number
}
export interface QueryLogPage {
  data: QueryEvent[]
  next_cursor?: number
  total?: number
}
export interface QueryLogFilter {
  q?: string
  name?: string
  client?: string
  source?: string
  rcode?: string
  type?: string
  since?: string
  until?: string
  sort?: 'asc' | 'desc'
  cursor?: number
  limit?: number
  count?: boolean
}
export interface TopEntry {
  value: string
  count: number
}
export interface Tops {
  domains: TopEntry[]
  clients: TopEntry[]
  types: TopEntry[]
  rcodes: TopEntry[]
}
export interface CacheStats {
  entries: number
  max: number
  hits: number
  misses: number
  evictions: number
}
export interface UpstreamHealth {
  address: string
  requests: number
  errors: number
  avg_latency_ms: number
  latency_sum_ms: number
  status: 'healthy' | 'failing' | 'cooling'
  last_error?: string
  last_error_at?: string
}
export interface ResolverHealth {
  cache: CacheStats
  upstreams: UpstreamHealth[]
}
export interface Metrics {
  queries: number
  local: number
  cache: number
  upstream: number
  errors: number
  nxdomain: number
  failures: number
  dropped_logs: number
  average_latency_ms: number
  p95_latency_ms: number
  p99_latency_ms: number
  local_avg_latency_ms: number
  cache_avg_latency_ms: number
  upstream_avg_latency_ms: number
}
export interface TrendPoint {
  time: string
  queries: number
  average_latency_ms: number
  local: number
  cache: number
  upstream: number
}
export interface Session {
  mode?: string
  authenticated: boolean
  email?: string
  subject?: string
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch('/api/v1' + path, {
    ...init,
    signal: init?.signal ?? AbortSignal.timeout(15000),
    headers: { 'Content-Type': 'application/json', ...init?.headers },
    credentials: 'same-origin'
  })
  if (!response.ok) {
    const body = await response.json().catch(() => ({}))
    throw new Error(body.error ?? 'Request failed (' + response.status + ')')
  }
  return (response.status === 204 ? undefined : response.json()) as T
}
export const api = {
  records: () => request<{ data: DNSRecord[] }>('/records'),
  createRecord: (record: Omit<DNSRecord, 'id'>) => request<DNSRecord>('/records', { method: 'POST', body: JSON.stringify(record) }),
  updateRecord: (id: string, record: Omit<DNSRecord, 'id'>) => request<DNSRecord>('/records/' + encodeURIComponent(id), { method: 'PUT', body: JSON.stringify(record) }),
  deleteRecord: (id: string) => request<void>('/records/' + encodeURIComponent(id), { method: 'DELETE' }),
  metrics: () => request<Metrics>('/metrics/summary'),
  trend: (window = '1h') => request<{ data: TrendPoint[]; bucket_seconds: number }>('/metrics/trend?window=' + encodeURIComponent(window)),
  tops: (window = '1h', limit = 10) => request<{ data: Tops }>('/metrics/tops?window=' + encodeURIComponent(window) + '&limit=' + limit),
  resolver: () => request<ResolverHealth>('/metrics/resolver'),
  queries: (filter: QueryLogFilter = {}) => {
    const params = new URLSearchParams()
    for (const [key, value] of Object.entries(filter)) {
      if (value === undefined || value === '' || value === false) continue
      params.set(key, value === true ? '1' : String(value))
    }
    return request<QueryLogPage>('/queries?' + params.toString())
  },
  queriesExportPath: (filter: QueryLogFilter = {}) => {
    const params = new URLSearchParams()
    for (const [key, value] of Object.entries(filter)) {
      if (['cursor', 'count', 'limit'].includes(key) || value === undefined || value === '' || value === false) continue
      params.set(key, value === true ? '1' : String(value))
    }
    return '/api/v1/queries/export?' + params.toString()
  },
  lookup: (name: string, type: string) => request<{ rcode: string, answers: string[], duration_ms: number }>('/dns/resolve', { method: 'POST', body: JSON.stringify({ name, type }) }),
  upstreams: () => request<{ data: string[] }>('/upstreams'),
  setUpstreams: (addresses: string[]) => request<{ data: string[] }>('/upstreams', { method: 'PUT', body: JSON.stringify({ addresses }) }),
  session: () => request<Session>('/session'),
  logout: async () => {
    const response = await fetch('/auth/logout', { method: 'POST', credentials: 'same-origin' })
    if (!response.ok) throw new Error('Unable to sign out')
  }
}
