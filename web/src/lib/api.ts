export interface DNSRecord {
  id: string
  name: string
  type: 'A' | 'AAAA' | 'TXT' | 'CNAME' | 'MX' | 'SRV' | 'PTR'
  value: string
  ttl: number
  enabled: boolean
  priority?: number
  weight?: number
  port?: number
}
export interface QueryEvent {
  time: string
  name: string
  type: string
  client: string
  source: string
  rcode: string
  duration_ms: number
}
export interface Metrics {
  queries: number
  local: number
  cache: number
  upstream: number
  errors: number
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch('/api/v1' + path, {
    ...init,
    headers: { 'Content-Type': 'application/json', ...init?.headers }
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
  deleteRecord: (id: string) => request<void>('/records/' + encodeURIComponent(id), { method: 'DELETE' }),
  metrics: () => request<Metrics>('/metrics/summary'),
  queries: () => request<{ data: QueryEvent[] }>('/queries'),
  lookup: (name: string, type: string) => request<{ rcode: string, answers: string[] }>('/dns/resolve', { method: 'POST', body: JSON.stringify({ name, type }) })
}
