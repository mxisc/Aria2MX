import type { ApiResponse, AppAbout, AppConfig, Aria2Task, CurrentUser, GlobalStat, ManagedOptionsSaveResult, PeerGuardSnapshot } from './types'

class AuthRedirectError extends Error {
  constructor() {
    super('')
    this.name = 'AuthRedirectError'
  }
}

let authRedirecting = false

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, {
    credentials: 'include',
    headers: init?.body instanceof FormData ? undefined : { 'Content-Type': 'application/json' },
    ...init,
  })
  if (response.status === 401) {
    if (!authRedirecting && window.location.pathname !== '/login') {
      authRedirecting = true
      window.location.replace('/login')
    }
    throw new AuthRedirectError()
  }
  const payload = (await response.json()) as ApiResponse<T>
  if (!response.ok || !payload.ok) {
    throw new Error(payload.error?.message || '请求失败，请稍后重试。')
  }
  return payload.data as T
}

export const api = {
  login(username: string, password: string) {
    return request<CurrentUser>('/api/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
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
  updateConfig(payload: Partial<AppConfig> & { aria2Secret?: string; newPassword?: string }) {
    return request<void>('/api/config', {
      method: 'PUT',
      body: JSON.stringify(payload),
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
