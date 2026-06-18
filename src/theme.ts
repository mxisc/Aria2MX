import { ref } from 'vue'

export type AppTheme = 'ariamx'
export type ColorMode = 'system' | 'light' | 'dark'
export type ResolvedColorMode = 'light' | 'dark'

export const currentTheme = ref<AppTheme>('ariamx')
export const currentColorMode = ref<ColorMode>('system')
export const currentResolvedColorMode = ref<ResolvedColorMode>('light')
export const currentSkinEnabled = ref(false)
export const currentSkinName = ref('default')
export const currentSkinApiTemplate = ref('')

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

function escapeCSSURL(url: string) {
  return `url(${JSON.stringify(url)})`
}

function buildSkinProxyURL(skinName: string, apiTemplate: string) {
  const cacheKey = encodeURIComponent(`${skinName}|${apiTemplate}`)
  return `/api/skin-image?v=${cacheKey}`
}

function resolveSkinImageURL(enabled?: boolean, skinName?: string, apiTemplate?: string) {
  if (!enabled) return ''
  const template = (apiTemplate || '').trim()
  if (!template) return ''
  const name = (skinName || 'default').trim() || 'default'
  if (template.includes('{skin}')) {
    return template.split('{skin}').join(encodeURIComponent(name))
  }
  return template
}

export function applySkin(enabled?: boolean, skinName?: string, apiTemplate?: string) {
  const normalizedEnabled = Boolean(enabled)
  const normalizedName = (skinName || 'default').trim() || 'default'
  const normalizedTemplate = (apiTemplate || '').trim()
  const resolvedURL = resolveSkinImageURL(normalizedEnabled, normalizedName, normalizedTemplate)
  const skinImageURL = resolvedURL ? buildSkinProxyURL(normalizedName, normalizedTemplate) : ''
  currentSkinEnabled.value = normalizedEnabled && resolvedURL.length > 0
  currentSkinName.value = normalizedName
  currentSkinApiTemplate.value = normalizedTemplate
  const skinState = currentSkinEnabled.value ? 'custom' : 'none'
  document.documentElement.dataset.skin = skinState
  document.body.dataset.skin = skinState
  document.documentElement.style.setProperty('--skin-image', currentSkinEnabled.value ? escapeCSSURL(skinImageURL) : 'none')
  return {
    skinEnabled: currentSkinEnabled.value,
    skinName: normalizedName,
    skinApiTemplate: normalizedTemplate,
    skinImageUrl: skinImageURL,
  }
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
