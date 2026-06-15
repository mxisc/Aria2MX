<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { api } from '@/api'
import type { Aria2OptionMap, Aria2Peer, Aria2Server, Aria2Task } from '@/types'
import { boolLabel, bytes, numberText, percent, speed, statusLabel, taskName } from '@/utils/format'

const props = defineProps<{ task?: Aria2Task }>()
const emit = defineEmits<{ changed: [] }>()

const tab = ref<'overview' | 'files' | 'options' | 'peers' | 'servers' | 'trackers'>('overview')
const options = ref<Aria2OptionMap>({})
const peers = ref<Aria2Peer[]>([])
const servers = ref<Aria2Server[]>([])
const optionDraft = ref('')
const message = ref('')

const trackerRows = computed(() => props.task?.bittorrent?.announceList?.flatMap((group, index) => group.map((url) => ({ group: index + 1, url }))) || [])
const selectedFiles = computed(() => (props.task?.files || []).filter((file) => file.selected === 'true').map((file) => file.index).join(','))
const overviewItems = computed(() => {
  if (!props.task) return []
  const task = props.task
  return [
    { key: 'status', label: '状态', value: statusLabel(task.status) },
    { key: 'progress', label: '进度', value: `${percent(task)}%` },
    { key: 'size', label: '大小', value: bytes(task.totalLength) },
    { key: 'completed', label: '已完成', value: bytes(task.completedLength) },
    { key: 'downloadSpeed', label: '下载速度', value: speed(task.downloadSpeed) },
    { key: 'uploadSpeed', label: '上传速度', value: speed(task.uploadSpeed) },
    { key: 'uploadLength', label: '上传量', value: bytes(task.uploadLength) },
    { key: 'connections', label: '连接数', value: task.connections || '0' },
    { key: 'seeders', label: 'Seed 数', value: task.numSeeders || '0' },
    { key: 'seeder', label: '做种', value: boolLabel(task.seeder) },
    { key: 'pieces', label: '分片', value: `${numberText(task.numPieces)} × ${bytes(task.pieceLength)}` },
    { key: 'gid', label: 'GID', value: task.gid, wide: true },
    { key: 'dir', label: '目录', value: task.dir || '-', wide: true },
  ]
})

watch(() => props.task?.gid, () => {
  tab.value = 'overview'
  message.value = ''
  loadDetail()
}, { immediate: true })

async function loadDetail() {
  if (!props.task?.gid) return
  try {
    const gid = props.task.gid
    const [nextOptions, nextPeers, nextServers] = await Promise.all([
      api.aria2<Aria2OptionMap>('aria2.getOption', [gid]),
      api.aria2<Aria2Peer[]>('aria2.getPeers', [gid]).catch(() => []),
      api.aria2<Aria2Server[]>('aria2.getServers', [gid]).catch(() => []),
    ])
    options.value = nextOptions
    peers.value = nextPeers
    servers.value = nextServers
    optionDraft.value = Object.entries(nextOptions).map(([key, value]) => `${key}=${value}`).join('\n')
  } catch (error) {
    message.value = error instanceof Error ? error.message : '任务详情读取失败。'
  }
}

async function saveOptions() {
  if (!props.task?.gid) return
  const patch: Record<string, string> = {}
  for (const line of optionDraft.value.split('\n')) {
    const trimmed = line.trim()
    if (!trimmed || trimmed.startsWith('#')) continue
    const index = trimmed.indexOf('=')
    if (index <= 0) continue
    patch[trimmed.slice(0, index).trim()] = trimmed.slice(index + 1).trim()
  }
  try {
    await api.aria2('aria2.changeOption', [props.task.gid, patch])
    message.value = '任务选项已保存。'
    await loadDetail()
    emit('changed')
  } catch (error) {
    message.value = error instanceof Error ? error.message : '任务选项保存失败。'
  }
}

async function setSelectedFiles(value: string) {
  if (!props.task?.gid) return
  try {
    await api.aria2('aria2.changeOption', [props.task.gid, { 'select-file': value }])
    message.value = '文件选择已更新。'
    emit('changed')
  } catch (error) {
    message.value = error instanceof Error ? error.message : '文件选择更新失败。'
  }
}

function toggleFile(fileIndex: string, checked: boolean) {
  const next = new Set((props.task?.files || []).filter((file) => file.selected === 'true').map((file) => file.index))
  if (checked) next.add(fileIndex)
  else next.delete(fileIndex)
  setSelectedFiles(Array.from(next).join(','))
}
</script>

<template>
  <section class="panel detail-panel">
    <template v-if="task">
      <div class="section-title">
        <span>任务详情</span>
        <small>{{ statusLabel(task.status) }}</small>
      </div>
      <h2>{{ taskName(task) }}</h2>
      <div class="detail-progress">
        <span :style="{ width: `${percent(task)}%` }" />
      </div>

      <div class="detail-tabs">
        <button :class="{ active: tab === 'overview' }" @click="tab = 'overview'">
          概览
        </button>
        <button :class="{ active: tab === 'files' }" @click="tab = 'files'">
          文件
        </button>
        <button :class="{ active: tab === 'options' }" @click="tab = 'options'">
          选项
        </button>
        <button :class="{ active: tab === 'peers' }" @click="tab = 'peers'">
          Peer
        </button>
        <button :class="{ active: tab === 'servers' }" @click="tab = 'servers'">
          Server
        </button>
        <button :class="{ active: tab === 'trackers' }" @click="tab = 'trackers'">
          Tracker
        </button>
      </div>

      <div class="detail-scroll">
        <div v-if="tab === 'overview'" class="detail-grid">
          <div
            v-for="item in overviewItems"
            :key="item.key"
            class="detail-cell"
            :class="{ wide: item.wide }"
          >
            <span class="detail-cell-label">{{ item.label }}</span>
            <strong class="detail-cell-value">{{ item.value }}</strong>
          </div>
        </div>

        <div v-else-if="tab === 'files'" class="file-list expanded">
          <div class="file-select-bar">
            <span>已选择：{{ selectedFiles || '无' }}</span>
            <button class="ghost" @click="setSelectedFiles((task.files || []).map((file) => file.index).join(','))">
              全选
            </button>
            <button class="ghost" @click="setSelectedFiles('')">
              全不选
            </button>
          </div>
          <div v-for="file in task.files || []" :key="file.index" class="file-row">
            <label class="file-check">
              <input
                type="checkbox"
                :checked="file.selected === 'true'"
                @change="toggleFile(file.index, file.selected !== 'true')"
              >
              <span>{{ file.path || `文件 ${file.index}` }}</span>
            </label>
            <strong>{{ bytes(file.completedLength) }} / {{ bytes(file.length) }}</strong>
          </div>
        </div>

        <div v-else-if="tab === 'options'" class="option-editor">
          <p class="hint">
            每行一个 `key=value`，保存后调用 `aria2.changeOption`。
          </p>
          <textarea v-model="optionDraft" spellcheck="false" />
          <button class="primary" @click="saveOptions">
            保存任务选项
          </button>
        </div>

        <div v-else-if="tab === 'peers'" class="data-table">
          <div class="table-row head">
            <span>地址</span><span>下载</span><span>上传</span><span>做种</span>
          </div>
          <div v-for="peer in peers" :key="`${peer.ip}:${peer.port}`" class="table-row">
            <span>{{ peer.ip }}:{{ peer.port }}</span>
            <span>{{ speed(peer.downloadSpeed) }}</span>
            <span>{{ speed(peer.uploadSpeed) }}</span>
            <span>{{ boolLabel(peer.seeder) }}</span>
          </div>
          <div v-if="peers.length === 0" class="empty-state">
            暂无 Peer 信息。
          </div>
        </div>

        <div v-else-if="tab === 'servers'" class="data-table">
          <template v-for="server in servers" :key="server.index">
            <div v-for="item in server.servers" :key="`${server.index}-${item.uri}`" class="table-row">
              <span>{{ item.currentUri || item.uri }}</span>
              <span>文件 {{ server.index }}</span>
              <span>{{ speed(item.downloadSpeed) }}</span>
            </div>
          </template>
          <div v-if="servers.length === 0" class="empty-state">
            暂无 Server 信息。
          </div>
        </div>

        <div v-else class="data-table">
          <div class="table-row head">
            <span>组</span><span>Tracker</span>
          </div>
          <div v-for="tracker in trackerRows" :key="`${tracker.group}-${tracker.url}`" class="table-row">
            <span>#{{ tracker.group }}</span>
            <span>{{ tracker.url }}</span>
          </div>
          <div v-if="trackerRows.length === 0" class="empty-state">
            暂无 Tracker 信息。
          </div>
        </div>
      </div>
      <p v-if="message" class="hint">
        {{ message }}
      </p>
      <p v-if="task.errorMessage" class="hint danger">
        {{ task.errorMessage }}
      </p>
    </template>
    <div v-else class="empty-state tall">
      选择一个任务查看详情。
    </div>
  </section>
</template>
