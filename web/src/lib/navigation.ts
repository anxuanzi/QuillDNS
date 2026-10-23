import { ref } from 'vue'

export type Page = 'overview' | 'records' | 'queries' | 'settings'

// State-based page switching: the shell renders views[page], matching the
// single-page console layout. A shared store lets a page navigate on the
// operator's behalf — the Overview client drilldown opens Query Logs.
export const activePage = ref<Page>('overview')

// clientFilter carries a drilldown target from the Overview top-activity
// panel into the Query Logs client filter; null means no drilldown filter.
export const drilldownClient = ref<string | null>(null)

export function openClientQueries(client: string) {
  drilldownClient.value = client
  activePage.value = 'queries'
}
