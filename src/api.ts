import type { ApiResponse, AppConfig, Aria2Task, CurrentUser, GlobalStat } from './types'

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, {
    credentials: 'include',
    headers: init?.body instanceof FormData ? undefined : { 'Content-Type': 'application/json' },
    ...init,
  })
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
  uploadTorrent(file: File) {
    const data = new FormData()
    data.set('torrent', file)
    return request<string>('/api/aria2/upload-torrent', {
      method: 'POST',
      body: data,
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
