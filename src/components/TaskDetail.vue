<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { api } from '@/api'
import type { Aria2Peer, Aria2Server, Aria2Task } from '@/types'
import { boolLabel, bytes, numberText, percent, speed, statusLabel, taskName } from '@/utils/format'

const props = defineProps<{ task?: Aria2Task }>()
const emit = defineEmits<{ changed: [] }>()

type BlockStatusSnapshot = {
  bitfield?: string
  numPieces?: string
  pieceLength?: string
  totalLength?: string
  completedLength?: string
}

type BlockCell = {
  key: string
  state: 'done' | 'partial' | 'empty'
  completed: number
  total: number
  fillPercent: number
}

type PeerRow = {
  key: string
  address: string
  addressTitle?: string
  downloadSpeed: string
  uploadSpeed: string
  seeder?: string
  barStyle: Record<string, string>
  progressText: string
  progressPercent: string
  isLocal?: boolean
}

const maxBlockCells = 960

const tab = ref<'overview' | 'files' | 'blocks' | 'peers' | 'servers' | 'trackers'>('overview')
const peers = ref<Aria2Peer[]>([])
const servers = ref<Aria2Server[]>([])
const blockStatus = ref<BlockStatusSnapshot>({})
const message = ref('')

const hasFiles = computed(() => (props.task?.files?.length || 0) > 0)
const hasPeerRows = computed(() => peers.value.length > 0)
const localPeerRow = computed<PeerRow | null>(() => {
  if (!props.task || !isBitTorrentTask.value) return null
  const task = props.task
  const exactBitfield = blockStatusView.value.numPieces > 0 && stringsPresent(blockStatusView.value.bitfield)
  return {
    key: `local-${task.gid}`,
    address: '本机',
    addressTitle: '当前 AriaMX / aria2 会话',
    downloadSpeed: task.downloadSpeed || '0',
    uploadSpeed: task.uploadSpeed || '0',
    seeder: task.seeder,
    barStyle: exactBitfield
      ? { '--peer-bar-bg': buildPieceBarGradient(decodeBitfield(blockStatusView.value.bitfield, blockStatusView.value.numPieces)) }
      : buildRatioBarStyle(Number(percent(task))),
    progressText: `${statusLabel(task.status)} · ${percent(task)}%`,
    progressPercent: `${percent(task)}%`,
    isLocal: true,
  }
})
const peerRows = computed<PeerRow[]>(() => {
  const pieceCount = blockStatusView.value.numPieces
  return peers.value.map((peer) => {
    const completedPieces = countPeerCompletedPieces(peer, pieceCount)
    const progressRatio = pieceCount > 0 ? (completedPieces / pieceCount) * 100 : 0
    return {
      key: `${peer.ip}:${peer.port}`,
      address: formatPeerAddress(peer),
      addressTitle: normalizePeerId(peer.peerId) || undefined,
      downloadSpeed: peer.downloadSpeed,
      uploadSpeed: peer.uploadSpeed,
      seeder: peer.seeder,
      barStyle: buildPeerBarStyle(peer, pieceCount),
      progressText: pieceCount > 0 ? `${numberText(completedPieces)} / ${numberText(pieceCount)}` : '-',
      progressPercent: pieceCount > 0 ? `${progressRatio.toFixed(2)}%` : '-',
      isLocal: false,
    }
  })
})
const statusRows = computed<PeerRow[]>(() => localPeerRow.value ? [localPeerRow.value, ...peerRows.value] : peerRows.value)
const serverRows = computed(() => servers.value.flatMap((server) => server.servers.map((item) => ({
  key: `${server.index}-${item.uri}`,
  fileIndex: server.index,
  uri: item.currentUri || item.uri,
  downloadSpeed: item.downloadSpeed,
}))))
const trackerRows = computed(() => props.task?.bittorrent?.announceList?.flatMap((group, index) => group.map((url) => ({ group: index + 1, url }))) || [])
const hasTrackerRows = computed(() => trackerRows.value.length > 0)
const isBitTorrentTask = computed(() => Boolean(props.task?.bittorrent))
const blockStatusView = computed(() => ({
  bitfield: blockStatus.value.bitfield || '',
  numPieces: Number(blockStatus.value.numPieces || props.task?.numPieces || 0),
  pieceLength: Number(blockStatus.value.pieceLength || props.task?.pieceLength || 0),
  totalLength: Number(blockStatus.value.totalLength || props.task?.totalLength || 0),
  completedLength: Number(blockStatus.value.completedLength || props.task?.completedLength || 0),
}))
const hasBlockInfo = computed(() => blockStatusView.value.numPieces > 0 || blockStatusView.value.totalLength > 0)
const hasExactBlockMap = computed(() => blockStatusView.value.numPieces > 0 && stringsPresent(blockStatusView.value.bitfield))
const blockMapMode = computed<'exact' | 'estimated' | 'progress' | 'empty'>(() => {
  const { numPieces, totalLength, bitfield } = blockStatusView.value
  if (numPieces > 0 && stringsPresent(bitfield)) return 'exact'
  if (numPieces > 0) return 'estimated'
  if (totalLength > 0) return 'progress'
  return 'empty'
})
const blockCells = computed<BlockCell[]>(() => {
  const { bitfield, numPieces, completedLength, totalLength } = blockStatusView.value
  if (numPieces <= 0) {
    return buildProgressFallbackCells(completedLength, totalLength)
  }
  if (!stringsPresent(bitfield)) {
    return buildEstimatedPieceCells(numPieces, completedLength, totalLength, maxBlockCells)
  }
  return buildExactPieceCells(bitfield, numPieces)
})
const completedPieceCount = computed(() => blockCells.value.reduce((sum, cell) => sum + cell.completed, 0))
const blockSummary = computed(() => {
  const { numPieces, pieceLength } = blockStatusView.value
  return [
    { key: 'pieces', label: '总分片', value: numPieces > 0 ? numberText(numPieces) : '-' },
    { key: 'done', label: '已完成分片', value: numPieces > 0 ? `${numberText(completedPieceCount.value)} / ${numberText(numPieces)}` : '-' },
    { key: 'pieceLength', label: '单片大小', value: pieceLength > 0 ? bytes(pieceLength) : '-' },
    {
      key: 'density',
      label: '显示精度',
      value: blockMapMode.value === 'exact'
        ? '1 格 / 1 分片'
        : blockMapMode.value === 'estimated'
          ? '估算图'
          : blockMapMode.value === 'progress'
            ? '进度图'
            : '-',
    },
  ]
})
const selectedFiles = computed(() => (props.task?.files || []).filter((file) => file.selected === 'true').map((file) => file.index).join(','))
const overviewItems = computed(() => {
  if (!props.task) return []
  const task = props.task
  const items: Array<{ key: string, label: string, value: string, wide?: boolean }> = [
    { key: 'status', label: '状态', value: statusLabel(task.status) },
    { key: 'progress', label: '进度', value: `${percent(task)}%` },
    { key: 'size', label: '大小', value: bytes(task.totalLength) },
    { key: 'completed', label: '已完成', value: bytes(task.completedLength) },
    { key: 'downloadSpeed', label: '下载速度', value: speed(task.downloadSpeed) },
  ]
  if (Number(task.uploadLength || 0) > 0 || Number(task.uploadSpeed || 0) > 0 || isBitTorrentTask.value) {
    items.push({ key: 'uploadSpeed', label: '上传速度', value: speed(task.uploadSpeed) })
  }
  if (Number(task.uploadLength || 0) > 0) {
    items.push({ key: 'uploadLength', label: '上传量', value: bytes(task.uploadLength) })
  }
  if (isBitTorrentTask.value && task.connections) {
    items.push({ key: 'connections', label: '连接数', value: task.connections || '0' })
  }
  if (isBitTorrentTask.value && task.numSeeders) {
    items.push({ key: 'seeders', label: 'Seed 数', value: task.numSeeders || '0' })
  }
  if (isBitTorrentTask.value && task.seeder) {
    items.push({ key: 'seeder', label: '做种', value: boolLabel(task.seeder) })
  }
  if (isBitTorrentTask.value && (task.numPieces || task.pieceLength)) {
    items.push({ key: 'pieces', label: '分片', value: `${numberText(task.numPieces)} × ${bytes(task.pieceLength)}` })
  }
  items.push(
    { key: 'gid', label: 'GID', value: task.gid, wide: true },
    { key: 'dir', label: '目录', value: task.dir || '-', wide: true },
  )
  return items
})
const availableTabs = computed(() => {
  const tabs: Array<{ key: typeof tab.value, label: string }> = [{ key: 'overview', label: '概览' }]
    if (hasFiles.value) tabs.push({ key: 'files', label: '文件列表' })
    if (hasBlockInfo.value) tabs.push({ key: 'blocks', label: '区块信息' })
    if (isBitTorrentTask.value || hasPeerRows.value) tabs.push({ key: 'peers', label: '连接状态' })
  if (serverRows.value.length > 0) tabs.push({ key: 'servers', label: 'Server' })
    if (hasTrackerRows.value) tabs.push({ key: 'trackers', label: '节点信息' })
  return tabs
})

watch(() => props.task?.gid, () => {
  tab.value = 'overview'
  message.value = ''
  peers.value = []
  servers.value = []
  blockStatus.value = {}
  loadServers()
}, { immediate: true })

watch(
  () => [props.task?.gid, props.task?.completedLength, props.task?.status, props.task?.numPieces, props.task?.pieceLength] as const,
  () => {
    refreshBlockStatus()
  },
  { immediate: true },
)

watch(
  () => [props.task?.gid, props.task?.status, props.task?.completedLength, props.task?.downloadSpeed, props.task?.uploadSpeed] as const,
  () => {
    refreshPeers()
  },
  { immediate: true },
)

watch(availableTabs, (tabs) => {
  if (!tabs.some((item) => item.key === tab.value)) {
    tab.value = 'overview'
  }
})

async function loadServers() {
  if (!props.task?.gid) return
  try {
    const gid = props.task.gid
    servers.value = await api.aria2<Aria2Server[]>('aria2.getServers', [gid]).catch(() => [])
  } catch (error) {
    message.value = error instanceof Error ? error.message : '任务详情读取失败。'
  }
}

async function refreshPeers() {
  if (!props.task?.gid) return
  try {
    const gid = props.task.gid
    peers.value = await api.aria2<Aria2Peer[]>('aria2.getPeers', [gid]).catch(() => [])
  } catch (error) {
    message.value = error instanceof Error ? error.message : '任务详情读取失败。'
  }
}

async function refreshBlockStatus() {
  if (!props.task?.gid) return
  try {
    const gid = props.task.gid
    const detail = await api.aria2<BlockStatusSnapshot>('aria2.tellStatus', [gid, ['bitfield', 'numPieces', 'pieceLength', 'totalLength', 'completedLength']]).catch(() => ({}))
    blockStatus.value = detail || {}
  } catch (error) {
    message.value = error instanceof Error ? error.message : '任务详情读取失败。'
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

function decodeBitfield(bitfield: string, pieceCount: number) {
  const states = new Array<boolean>(pieceCount).fill(false)
  if (!bitfield) return states

  let pieceIndex = 0
  for (const char of bitfield) {
    const nibble = Number.parseInt(char, 16)
    if (Number.isNaN(nibble)) continue
    for (let bit = 3; bit >= 0 && pieceIndex < pieceCount; bit -= 1) {
      states[pieceIndex] = ((nibble >> bit) & 1) === 1
      pieceIndex += 1
    }
    if (pieceIndex >= pieceCount) break
  }
  return states
}

function buildBitfieldCells(bitfield: string, pieceCount: number, maxCells: number) {
  if (pieceCount <= 0) return []
  const pieceStates = decodeBitfield(bitfield, pieceCount)
  const cellCount = Math.min(pieceCount, maxCells)
  const piecesPerCell = Math.ceil(pieceCount / cellCount)
  const cells: BlockCell[] = []

  for (let index = 0; index < cellCount; index += 1) {
    const start = index * piecesPerCell
    const end = Math.min(pieceCount, start + piecesPerCell)
    let done = 0
    for (let pieceIndex = start; pieceIndex < end; pieceIndex += 1) {
      if (pieceStates[pieceIndex]) done += 1
    }
    const total = end - start
    const fillPercent = total > 0 ? (done / total) * 100 : 0
    cells.push({
      key: `${start}-${end - 1}`,
      state: done <= 0 ? 'empty' : done >= total ? 'done' : 'partial',
      completed: done,
      total,
      fillPercent,
    })
  }
  return cells
}

function buildExactPieceCells(bitfield: string, pieceCount: number) {
  if (pieceCount <= 0) return []
  const pieceStates = decodeBitfield(bitfield, pieceCount)
  return pieceStates.map((done, index) => ({
    key: `piece-${index}`,
    state: done ? 'done' as const : 'empty' as const,
    completed: done ? 1 : 0,
    total: 1,
    fillPercent: done ? 100 : 0,
  }))
}

function countPeerCompletedPieces(peer: Aria2Peer, pieceCount: number) {
  if (pieceCount <= 0) return 0
  if (stringsPresent(peer.bitfield || '')) {
    return decodeBitfield(peer.bitfield || '', pieceCount).reduce((sum, done) => sum + (done ? 1 : 0), 0)
  }
  if (peer.seeder === 'true') {
    return pieceCount
  }
  return 0
}

function buildPeerBarStyle(peer: Aria2Peer, pieceCount: number) {
  if (pieceCount <= 0) {
    return { '--peer-bar-bg': 'linear-gradient(90deg, var(--soft) 0%, var(--soft) 100%)' }
  }
  if (stringsPresent(peer.bitfield || '')) {
    return { '--peer-bar-bg': buildPieceBarGradient(decodeBitfield(peer.bitfield || '', pieceCount)) }
  }
  if (peer.seeder === 'true') {
    return { '--peer-bar-bg': 'linear-gradient(90deg, var(--accent) 0%, var(--accent) 100%)' }
  }
  return { '--peer-bar-bg': 'linear-gradient(90deg, var(--soft) 0%, var(--soft) 100%)' }
}

function buildRatioBarStyle(progressRatio: number) {
  const clamped = Math.max(0, Math.min(100, progressRatio))
  return {
    '--peer-bar-bg': `linear-gradient(90deg, var(--accent) 0%, var(--accent) ${clamped}%, var(--soft) ${clamped}%, var(--soft) 100%)`,
  }
}

function buildPieceBarGradient(pieceStates: boolean[]) {
  if (pieceStates.length === 0) {
    return 'linear-gradient(90deg, var(--soft) 0%, var(--soft) 100%)'
  }
  const stops: string[] = []
  let start = 0
  for (let index = 1; index <= pieceStates.length; index += 1) {
    if (index < pieceStates.length && pieceStates[index] === pieceStates[start]) continue
    const color = pieceStates[start] ? 'var(--accent)' : 'var(--soft)'
    const from = ((start / pieceStates.length) * 100).toFixed(4)
    const to = ((index / pieceStates.length) * 100).toFixed(4)
    stops.push(`${color} ${from}%`, `${color} ${to}%`)
    start = index
  }
  return `linear-gradient(90deg, ${stops.join(', ')})`
}

function formatPeerAddress(peer: Aria2Peer) {
  const base = `${peer.ip}:${peer.port}`
  const client = detectPeerClient(peer.peerId)
  return client ? `${base} (${client})` : base
}

function normalizePeerId(peerId?: string) {
  if (!stringsPresent(peerId)) return ''
  try {
    return decodeURIComponent(peerId || '')
  } catch {
    return peerId || ''
  }
}

function detectPeerClient(peerId?: string) {
  const value = normalizePeerId(peerId)
  if (!stringsPresent(value)) return ''

  if (value.startsWith('-')) {
    const code = value.slice(1, 3)
    const knownClients: Record<string, string> = {
      AG: 'Ares',
      AR: 'Arctic',
      AT: 'Artemis',
      AV: 'Avicora',
      AX: 'BitPump',
      AZ: 'Vuze',
      BC: 'BitComet',
      BE: 'BitTorrent SDK',
      BF: 'BitFlu',
      BG: 'BTG',
      BT: 'BitTorrent',
      BR: 'BitRocket',
      BS: 'BTSlave',
      BX: 'Bittorrent X',
      CD: 'Enhanced CTorrent',
      CT: 'CTorrent',
      DE: 'Deluge',
      DP: 'Propagate Data Client',
      EB: 'EBit',
      ES: 'electric sheep',
      FC: 'FileCroc',
      FT: 'FoxTorrent',
      GR: 'GetRight',
      HL: 'Halite',
      HM: 'hMule',
      KG: 'KGet',
      KT: 'KTorrent',
      LC: 'LeechCraft',
      LH: 'LH-ABC',
      LP: 'Lphant',
      LT: 'libtorrent',
      lw: 'LimeWire',
      MK: 'Meerkat',
      MO: 'MonoTorrent',
      MP: 'MooPolice',
      MR: 'Miro',
      MT: 'MoonlightTorrent',
      PD: 'Pando',
      PI: 'PicoTorrent',
      qB: 'qBittorrent',
      QD: 'QQDownload',
      QT: 'Qt 4 Torrent example',
      RT: 'Retriever',
      SB: 'Swiftbit',
      SD: 'Thunder',
      SM: 'SoMud',
      SP: 'BitSpirit',
      SS: 'SwarmScope',
      ST: 'SymTorrent',
      SZ: 'Shareaza',
      TB: 'Torch',
      TE: 'terasaur Seed Bank',
      TL: 'Tribler',
      TS: 'Torrentstorm',
      TT: 'TuoTu',
      UL: 'uLeecher',
      UT: 'uTorrent',
      VG: 'Vagaa',
      WD: 'WebTorrent Desktop',
      WT: 'BitLet',
      WW: 'WebTorrent',
      WY: 'FireTorrent',
      XL: 'Xunlei',
      XT: 'XanTorrent',
      XX: 'Xtorrent',
      ZT: 'ZipTorrent',
    }
    return knownClients[code] || ''
  }

  if (value.startsWith('exbc') || value.startsWith('FUTB') || value.startsWith('xUTB')) {
    return 'BitComet'
  }
  if (value.startsWith('Mbrst')) {
    return 'Burst!'
  }
  if (value.startsWith('OP')) {
    return 'Opera'
  }
  if (value.startsWith('XBT')) {
    return 'XBT'
  }

  return ''
}

function buildProgressFallbackCells(completedLength: number, totalLength: number) {
  if (totalLength <= 0) return []
  const cellCount = Math.min(maxBlockCells, 120)
  const doneCells = Math.round((completedLength / totalLength) * cellCount)
  return Array.from({ length: cellCount }, (_, index) => ({
    key: `progress-${index}`,
    state: index < doneCells ? 'done' as const : 'empty' as const,
    completed: index < doneCells ? 1 : 0,
    total: 1,
    fillPercent: index < doneCells ? 100 : 0,
  }))
}

function buildEstimatedPieceCells(pieceCount: number, completedLength: number, totalLength: number, maxCells: number) {
  if (pieceCount <= 0) return buildProgressFallbackCells(completedLength, totalLength)
  const ratio = totalLength > 0 ? completedLength / totalLength : 0
  const estimatedPieces = Math.max(0, Math.min(pieceCount, Math.round(ratio * pieceCount)))
  const cellCount = Math.min(pieceCount, maxCells)
  const piecesPerCell = Math.ceil(pieceCount / cellCount)
  const cells: BlockCell[] = []

  for (let index = 0; index < cellCount; index += 1) {
    const start = index * piecesPerCell
    const end = Math.min(pieceCount, start + piecesPerCell)
    const total = end - start
    const remaining = Math.max(0, estimatedPieces - start)
    const completed = Math.min(total, remaining)
    const fillPercent = total > 0 ? (completed / total) * 100 : 0
    cells.push({
      key: `estimated-${start}-${end - 1}`,
      state: completed <= 0 ? 'empty' : completed >= total ? 'done' : 'partial',
      completed,
      total,
      fillPercent,
    })
  }

  return cells
}

function stringsPresent(value: string) {
  return value.trim().length > 0
}

function cellFillStyle(cell: BlockCell) {
  return { '--cell-fill': `${cell.fillPercent}%` }
}

</script>

<template>
  <section class="panel detail-panel">
    <template v-if="task">
      <div class="section-title">
        <span>任务详情</span>
      </div>
      <h2>{{ taskName(task) }}</h2>
      <div class="detail-progress">
        <span :style="{ width: `${percent(task)}%` }" />
      </div>

      <div class="detail-tabs">
        <button v-for="item in availableTabs" :key="item.key" :class="{ active: tab === item.key }" @click="tab = item.key">
          {{ item.label }}
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

        <div v-else-if="tab === 'blocks'" class="block-panel">
          <div class="detail-grid block-summary-grid">
            <div
              v-for="item in blockSummary"
              :key="item.key"
              class="detail-cell"
            >
              <span class="detail-cell-label">{{ item.label }}</span>
              <strong class="detail-cell-value">{{ item.value }}</strong>
            </div>
          </div>
          <p v-if="blockMapMode === 'estimated'" class="block-note">
            当前 aria2 没有返回 bitfield，下面显示的是估算图，不是 AriaNg 那种逐分片地图。
          </p>
          <p v-else-if="blockMapMode === 'progress'" class="block-note">
            当前任务没有可用的分片数据，下面显示的是按整体进度估算的占位图。
          </p>
          <div v-if="blockCells.length > 0" class="block-legend">
            <span><i class="done" /> 已完成</span>
            <span v-if="!hasExactBlockMap"><i class="partial" /> 部分完成</span>
            <span><i class="empty" /> 未完成</span>
          </div>
          <div v-if="blockCells.length > 0" class="block-grid" :class="{ exact: hasExactBlockMap }">
            <span
              v-for="cell in blockCells"
              :key="cell.key"
              class="block-cell"
              :class="cell.state"
              :style="cellFillStyle(cell)"
              :title="cell.total > 1 ? `${cell.completed}/${cell.total} 个分片` : cell.state === 'done' ? '已完成' : '未完成'"
            />
          </div>
          <div v-else class="empty-state">
            当前任务暂时没有可显示的区块信息。
          </div>
        </div>

        <div v-else-if="tab === 'peers'" class="data-table peer-table">
          <div class="table-row head">
            <span>地址</span><span>状态</span><span>下载</span><span>上传</span><span>做种</span>
          </div>
          <div v-for="peer in statusRows" :key="peer.key" class="table-row" :class="{ local: peer.isLocal }">
              <span class="peer-address-cell" :title="peer.addressTitle">
                <span>{{ peer.address }}</span>
                <b v-if="peer.isLocal" class="peer-local-tag">当前</b>
              </span>
            <span class="peer-progress-cell">
                <span class="peer-piece-bar" :style="peer.barStyle" :title="peer.progressText" />
                <strong class="peer-progress-percent">{{ peer.progressPercent }}</strong>
            </span>
            <span>{{ speed(peer.downloadSpeed) }}</span>
            <span>{{ speed(peer.uploadSpeed) }}</span>
            <span>{{ boolLabel(peer.seeder) }}</span>
          </div>
            <div v-if="statusRows.length === 0" class="empty-state">
              暂无状态信息。
          </div>
        </div>

        <div v-else-if="tab === 'servers'" class="data-table">
          <div v-for="server in serverRows" :key="server.key" class="table-row">
            <span>{{ server.uri }}</span>
            <span>文件 {{ server.fileIndex }}</span>
            <span>{{ speed(server.downloadSpeed) }}</span>
          </div>
        </div>

        <div v-else class="data-table">
            <div class="table-row head">
              <span>组</span><span>节点</span>
            </div>
          <div v-for="tracker in trackerRows" :key="`${tracker.group}-${tracker.url}`" class="table-row">
            <span>#{{ tracker.group }}</span>
            <span>{{ tracker.url }}</span>
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
