import type { Aria2Task } from '@/types'

export function bytes(value?: string | number) {
  const size = Number(value || 0)
  if (!Number.isFinite(size) || size <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const index = Math.min(Math.floor(Math.log(size) / Math.log(1024)), units.length - 1)
  return `${(size / 1024 ** index).toFixed(index === 0 ? 0 : 1)} ${units[index]}`
}

export function speed(value?: string | number) {
  return `${bytes(value)}/s`
}

export function percent(task: Aria2Task) {
  const total = Number(task.totalLength || 0)
  const done = Number(task.completedLength || 0)
  if (total <= 0) return 0
  return Math.min(100, Math.round((done / total) * 100))
}

export function taskName(task: Aria2Task) {
  const btName = task.bittorrent?.info?.name
  if (btName) return btName
  const file = task.files?.[0]?.path
  if (!file) return task.gid
  return file.split('/').filter(Boolean).pop() || task.gid
}

export function statusLabel(status: string) {
  const map: Record<string, string> = {
    active: '下载中',
    waiting: '等待中',
    paused: '已暂停',
    complete: '已完成',
    error: '出错',
    removed: '已移除',
  }
  return map[status] || status
}

export function boolLabel(value?: string) {
  if (value === 'true') return '是'
  if (value === 'false') return '否'
  return value || '-'
}

export function numberText(value?: string | number) {
  const n = Number(value || 0)
  return Number.isFinite(n) ? n.toLocaleString() : String(value || '0')
}
