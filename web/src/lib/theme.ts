import { ref } from 'vue'

export type ColorMode = 'light' | 'dark'

const storageKey = 'quilldns:color-mode'
const mode = ref<ColorMode>('light')

export function applyColorMode(next: ColorMode): void {
  mode.value = next
  document.documentElement.classList.toggle('dark', next === 'dark')
  document.documentElement.classList.toggle('light', next === 'light')
  document.documentElement.style.colorScheme = next
  try {
    window.localStorage.setItem(storageKey, next)
  } catch {
    // Browsers with disabled storage can still switch themes for the current session.
  }
}

export function initializeColorMode(): void {
  let saved: string | null = null
  try {
    saved = window.localStorage.getItem(storageKey)
  } catch {
    // The light theme is the accessible default when persistent storage is unavailable.
  }
  applyColorMode(saved === 'dark' ? 'dark' : 'light')
}

export function useColorMode() {
  return {
    mode,
    toggle: () => applyColorMode(mode.value === 'light' ? 'dark' : 'light')
  }
}
