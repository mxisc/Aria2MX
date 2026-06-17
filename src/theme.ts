import { ref } from 'vue'

export type AppTheme = 'ariamx'
export type ColorMode = 'system' | 'light' | 'dark'
export type ResolvedColorMode = 'light' | 'dark'

export const currentTheme = ref<AppTheme>('ariamx')
export const currentColorMode = ref<ColorMode>('system')
export const currentResolvedColorMode = ref<ResolvedColorMode>('light')

let systemColorSchemeMedia: MediaQueryList | null = null
let removeSystemColorSchemeListener: (() => void) | null = null

export function normalizeTheme(value?: string): AppTheme {
  return 'ariamx'
}

export function normalizeColorMode(value?: string): ColorMode {
  return value === 'dark' || value === 'light' || value === 'system' ? value : 'system'
}

function resolveColorMode(mode: ColorMode): ResolvedColorMode {
  if (mode === 'system') {
    if (typeof window !== 'undefined' && window.matchMedia('(prefers-color-scheme: dark)').matches) {
      return 'dark'
    }
    return 'light'
  }
  return mode
}

function applyResolvedTheme(theme: AppTheme, mode: ResolvedColorMode) {
  currentResolvedColorMode.value = mode
  document.documentElement.dataset.theme = theme
  document.documentElement.dataset.mode = mode
  document.body.dataset.theme = theme
  document.body.dataset.mode = mode
}

function clearSystemColorSchemeListener() {
  if (removeSystemColorSchemeListener) {
    removeSystemColorSchemeListener()
    removeSystemColorSchemeListener = null
  }
}

function ensureSystemColorSchemeListener(theme: AppTheme, mode: ColorMode) {
  clearSystemColorSchemeListener()
  if (mode !== 'system' || typeof window === 'undefined') {
    return
  }

  systemColorSchemeMedia = window.matchMedia('(prefers-color-scheme: dark)')
  const handleChange = () => {
    applyResolvedTheme(theme, resolveColorMode(currentColorMode.value))
  }
  if (typeof systemColorSchemeMedia.addEventListener === 'function') {
    systemColorSchemeMedia.addEventListener('change', handleChange)
    removeSystemColorSchemeListener = () => systemColorSchemeMedia?.removeEventListener('change', handleChange)
    return
  }
  systemColorSchemeMedia.addListener(handleChange)
  removeSystemColorSchemeListener = () => systemColorSchemeMedia?.removeListener(handleChange)
}

export function applyTheme(value?: string, colorMode?: string) {
  const theme = normalizeTheme(value)
  const mode = normalizeColorMode(colorMode)
  currentTheme.value = theme
  currentColorMode.value = mode
  applyResolvedTheme(theme, resolveColorMode(mode))
  ensureSystemColorSchemeListener(theme, mode)
  return { theme, colorMode: mode }
}
