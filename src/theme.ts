import { ref } from 'vue'

export type AppTheme = 'classic' | 'design'

export const currentTheme = ref<AppTheme>('design')

export function normalizeTheme(value?: string): AppTheme {
  return value === 'classic' ? 'classic' : 'design'
}

export function applyTheme(value?: string) {
  const theme = normalizeTheme(value)
  currentTheme.value = theme
  document.documentElement.dataset.theme = theme
  document.body.dataset.theme = theme
  return theme
}
