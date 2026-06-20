import type { ApiResponse, AppAbout, AppConfig, Aria2Task, CurrentUser, GlobalStat, ManagedOptionsSaveResult, PeerGuardSnapshot, PublicPanelStyle, ScriptHookItem, ScriptHookState, TrackerSubscriptionState } from './types'

class AuthRedirectError extends Error {
  constructor() {
    super('')
    this.name = 'AuthRedirectError'
  }
}

let authRedirecting = false

function fallbackRequestErrorMessage(url: string, status: number): string {
  if (url === '/api/tracker-subscription') {
    if (status >= 500) return '节点订阅暂时不可用，请稍后重试。'
    return '节点订阅保存失败，请检查设置后重试。'
  }
  if (status >= 500) return '服务暂时不可用，请稍后重试。'
  return '请求失败，请稍后重试。'
}

async function sha256Hex(value: string): Promise<string> {
  const data = new TextEncoder().encode(value)
  const digest = await crypto.subtle.digest('SHA-256', data)
  return Array.from(new Uint8Array(digest))
    .map((byte) => byte.toString(16).padStart(2, '0'))
    .join('')
}

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, {
    credentials: 'include',
    headers: init?.body instanceof FormData ? undefined : { 'Content-Type': 'application/json' },
    ...init,
  })
  const rawPayload = await response.text()
  let payload: ApiResponse<T> | null = null
  try {
    payload = rawPayload ? (JSON.parse(rawPayload) as ApiResponse<T>) : null
  } catch {
    payload = null
  }
  if (response.status === 401) {
    if (url === '/api/auth/login') {
      throw new Error(payload?.error?.message || '用户名或密码不正确。')
    }
    if (!authRedirecting && window.location.pathname !== '/login') {
      authRedirecting = true
      window.location.replace('/login')
    }
    throw new AuthRedirectError()
  }
  if (!response.ok || !payload.ok) {
    throw new Error(payload?.error?.message || fallbackRequestErrorMessage(url, response.status))
  }
  return payload.data as T
}

export const api = {
  async login(username: string, password: string) {
    const passwordSha256 = await sha256Hex(password)
    return request<CurrentUser>('/api/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username, passwordSha256 }),
    })
  },
  logout() {
    return request<void>('/api/auth/logout', { method: 'POST' })
  },
  me() {
    return request<CurrentUser>('/api/auth/me')
  },
  getAbout() {
    return request<AppAbout>('/api/about')
  },
  getConfig() {
    return request<AppConfig>('/api/config')
  },
  getPublicPanelStyle() {
    return request<PublicPanelStyle>('/api/panel-style')
  },
  async updateConfig(payload: Partial<AppConfig> & { aria2Secret?: string; newPassword?: string }) {
    const requestPayload: Record<string, unknown> = { ...payload }
    if (typeof payload.newPassword === 'string' && payload.newPassword.length > 0) {
      requestPayload.newPasswordSha256 = await sha256Hex(payload.newPassword)
      delete requestPayload.newPassword
    }
    return request<void>('/api/config', {
      method: 'PUT',
      body: JSON.stringify(requestPayload),
    })
  },
  aria2<T>(method: string, params: unknown[] = []) {
    return request<T>('/api/aria2/call', {
      method: 'POST',
      body: JSON.stringify({ method, params }),
    })
  },
  restartTask(gid: string) {
    return request<{ gid: string }>('/api/aria2/restart', {
      method: 'POST',
      body: JSON.stringify({ gid }),
    })
  },
  removeTask(gid: string) {
    return request<{ deletedPaths?: string[] }>('/api/aria2/remove', {
      method: 'POST',
      body: JSON.stringify({ gid }),
    })
  },
  saveManagedAria2Options(patch: Record<string, string>) {
    return request<ManagedOptionsSaveResult>('/api/aria2/options', {
      method: 'POST',
      body: JSON.stringify({ patch }),
    })
  },
  resetManagedAria2Options() {
    return request<ManagedOptionsSaveResult>('/api/aria2/options/reset', {
      method: 'POST',
    })
  },
  getTrackerSubscription() {
    return request<TrackerSubscriptionState>('/api/tracker-subscription')
  },
  updateTrackerSubscription(enabled: boolean, source: string) {
    return request<TrackerSubscriptionState>('/api/tracker-subscription', {
      method: 'POST',
      body: JSON.stringify({ enabled, source }),
    })
  },
  getScriptHooks() {
    return request<ScriptHookState>('/api/scripts')
  },
  updateScriptHooks(hooks: ScriptHookItem[]) {
    return request<ScriptHookState>('/api/scripts', {
      method: 'POST',
      body: JSON.stringify({ hooks }),
    })
  },
  uploadTorrent(file: File, options?: Record<string, string>) {
    const data = new FormData()
    data.set('torrent', file)
    Object.entries(options || {}).forEach(([key, value]) => {
      if (value !== '') {
        data.append(key, value)
      }
    })
    return request<string>('/api/aria2/upload-torrent', {
      method: 'POST',
      body: data,
    })
  },
  getPeerGuard() {
    return request<PeerGuardSnapshot>('/api/peer-guard')
  },
  updatePeerGuardSettings(autoBanEnabled: boolean) {
    return request<PeerGuardSnapshot>('/api/peer-guard/settings', {
      method: 'POST',
      body: JSON.stringify({ autoBanEnabled }),
    })
  },
  banPeer(ip: string, reason = '') {
    return request<PeerGuardSnapshot>('/api/peer-guard/ban', {
      method: 'POST',
      body: JSON.stringify({ ip, reason }),
    })
  },
  unbanPeer(ip: string) {
    return request<PeerGuardSnapshot>('/api/peer-guard/unban', {
      method: 'POST',
      body: JSON.stringify({ ip }),
    })
  },
}

export async function fetchDashboard() {
  const keys = [
    'gid',
    'status',
    'totalLength',
    'completedLength',
    'uploadLength',
    'downloadSpeed',
    'uploadSpeed',
    'connections',
    'dir',
    'files',
    'bittorrent',
    'followedBy',
    'following',
    'belongsTo',
    'numPieces',
    'pieceLength',
    'numSeeders',
    'seeder',
    'errorCode',
    'errorMessage',
  ]
  const [stat, active, waiting, stopped] = await Promise.all([
    api.aria2<GlobalStat>('aria2.getGlobalStat'),
    api.aria2<Aria2Task[]>('aria2.tellActive', [keys]),
    api.aria2<Aria2Task[]>('aria2.tellWaiting', [0, 100, keys]),
    api.aria2<Aria2Task[]>('aria2.tellStopped', [0, 100, keys]),
  ])
  return { stat, active, waiting, stopped }
}
