<script setup lang="ts">
import { computed, defineAsyncComponent, onMounted, onUnmounted, ref } from 'vue'
import { Activity, ArrowLeft, Blocks, Eraser, FileCode2, Gauge, Info, Link2, ListTree, LogOut, Pause, Play, Plus, Save, Search, Settings, Shield, SlidersHorizontal } from 'lucide-vue-next'
import { api, fetchDashboard } from '@/api'
import AddTaskPanel from '@/components/AddTaskPanel.vue'
import Aria2OptionsPanel from '@/components/Aria2OptionsPanel.vue'
import ConnectionInfoPanel from '@/components/ConnectionInfoPanel.vue'
import MCPPanel from '@/components/MCPPanel.vue'
import MetricCard from '@/components/MetricCard.vue'
import NodeSubscriptionPanel from '@/components/NodeSubscriptionPanel.vue'
import PeerGuardPanel from '@/components/PeerGuardPanel.vue'
import SettingsPanel from '@/components/SettingsPanel.vue'
import TaskDetail from '@/components/TaskDetail.vue'
import TaskList from '@/components/TaskList.vue'
import { builtInTrackerSubscriptions } from '@/data/nodeSubscriptions'
import type { AppAbout, Aria2Task, GlobalStat, TaskBucket } from '@/types'
import { percent, speed, taskName } from '@/utils/format'

const emit = defineEmits<{ loggedOut: [] }>()
const ScriptSettingsPanel = defineAsyncComponent(() => import('@/components/ScriptSettingsPanel.vue'))

const active = ref<Aria2Task[]>([])
const waiting = ref<Aria2Task[]>([])
const stopped = ref<Aria2Task[]>([])
const stat = ref<GlobalStat>({ downloadSpeed: '0', uploadSpeed: '0', numActive: '0', numWaiting: '0', numStopped: '0' })
const selected = ref<Aria2Task>()
const bucket = ref<TaskBucket>('active')
const activePage = ref<'overview' | 'tasks' | 'taskDetail' | 'add' | 'subscriptions' | 'options' | 'scripts' | 'peerGuard' | 'connection' | 'mcp' | 'settings'>('overview')
const query = ref('')
const sortBy = ref<'name' | 'progress' | 'speed' | 'size'>('name')
const error = ref('')
const about = ref<AppAbout>({ panelVersion: '-', aria2Version: '-', rpcPath: '/jsonrpc', httpRpcUrl: '/jsonrpc', wsRpcUrl: '/jsonrpc', mcpHttpUrl: '/mcp', mcpEnabled: true, panelRpcSecret: '-' })
const refreshIntervalMs = ref(1500)
let timer: number | undefined

const pageMeta = {
  overview: { title: '总览', subtitle: '查看下载速度、任务数量和常用操作。', badge: '控制台' },
  tasks: { title: '任务列表', subtitle: '筛选、检索并管理当前下载任务。', badge: '任务列表' },
  taskDetail: { title: '任务详情', subtitle: '查看单个任务的完整状态、文件和选项。', badge: '任务详情' },
  add: { title: '新建任务', subtitle: '提交链接、磁力或种子文件。', badge: '新建任务' },
  subscriptions: { title: '节点订阅', subtitle: '选择内置订阅源并写入 aria2 的 bt-tracker。', badge: '节点订阅' },
  options: { title: 'Aria2', subtitle: '按分类维护 aria2 全局参数。', badge: 'Aria2' },
  scripts: { title: '脚本设置', subtitle: '编辑任务完成后自动执行的脚本。', badge: '脚本设置' },
  peerGuard: { title: '节点防护', subtitle: '封禁只从本机获取数据而不回传的吸血节点；', badge: '节点防护' },
  connection: { title: '连接信息', subtitle: '查看当前面板代理、内置 RPC、MCP 和版本信息。', badge: '连接信息' },
  mcp: { title: 'MCP', subtitle: '查看 MCP 可用工具。', badge: 'MCP' },
  settings: { title: '面板设置', subtitle: '维护 RPC、刷新间隔和面板登录配置。', badge: '面板设置' },
} as const

const allTasks = computed(() => [...active.value, ...waiting.value, ...stopped.value])
const currentPageMeta = computed(() => {
  if (activePage.value === 'taskDetail' && selected.value) {
    return {
      title: taskName(selected.value),
      subtitle: '查看单个任务的完整状态、文件和选项。',
      badge: '任务详情',
    }
  }
  return pageMeta[activePage.value]
})
const currentBucketCount = computed(() => {
  if (activePage.value === 'taskDetail') return selected.value ? '1' : '0'
  if (activePage.value === 'subscriptions') return String(builtInTrackerSubscriptions.length)
  if (activePage.value !== 'tasks') return String(allTasks.value.length)
  if (bucket.value === 'waiting') return stat.value.numWaiting
  if (bucket.value === 'stopped') return stat.value.numStopped
  return stat.value.numActive
})

const visibleTasks = computed(() => {
  const source = bucket.value === 'waiting' ? waiting.value : bucket.value === 'stopped' ? stopped.value : active.value
  const keyword = query.value.trim().toLowerCase()
  return source
    .filter((task) => {
      if (!keyword) return true
      return [taskName(task), task.gid, task.dir, task.status].filter(Boolean).join(' ').toLowerCase().includes(keyword)
    })
    .slice()
    .sort((left, right) => {
      if (sortBy.value === 'progress') return percent(right) - percent(left)
      if (sortBy.value === 'speed') return Number(right.downloadSpeed || 0) - Number(left.downloadSpeed || 0)
      if (sortBy.value === 'size') return Number(right.totalLength || 0) - Number(left.totalLength || 0)
      return taskName(left).localeCompare(taskName(right), 'zh-CN')
    })
})

onMounted(async () => {
  await loadRefreshConfig()
  await refresh()
  loadAbout()
  startRefreshTimer()
})

onUnmounted(() => {
  stopRefreshTimer()
})

function startRefreshTimer() {
  stopRefreshTimer()
  timer = window.setInterval(refresh, refreshIntervalMs.value)
}

function stopRefreshTimer() {
  if (timer) window.clearInterval(timer)
  timer = undefined
}

async function loadRefreshConfig() {
  try {
    const config = await api.getConfig()
    applyRefreshInterval(config.refreshIntervalMs)
  } catch {
    applyRefreshInterval(1500)
  }
}

function applyRefreshInterval(next: number) {
  refreshIntervalMs.value = next >= 500 ? next : 1500
  if (timer) {
    startRefreshTimer()
  }
}

async function refresh() {
  try {
    const data = await fetchDashboard()
    active.value = data.active
    waiting.value = data.waiting
    stopped.value = data.stopped
    stat.value = data.stat
    const all = [...data.active, ...data.waiting, ...data.stopped]
    selected.value = all.find((task) => task.gid === selected.value?.gid) || all[0]
    if (activePage.value === 'taskDetail' && !selected.value) {
      activePage.value = 'tasks'
    }
    error.value = ''
  } catch (caught) {
    error.value = caught instanceof Error ? caught.message : '无法连接 aria2。'
  }
}

function openTask(task: Aria2Task) {
  selected.value = task
  bucket.value = task.status === 'waiting' || task.status === 'paused' ? 'waiting' : task.status === 'active' ? 'active' : 'stopped'
  activePage.value = 'taskDetail'
}

async function loadAbout() {
  try {
    about.value = await api.getAbout()
  } catch {
    about.value = { panelVersion: '-', aria2Version: '-', rpcPath: '/jsonrpc', httpRpcUrl: '/jsonrpc', wsRpcUrl: '/jsonrpc', mcpHttpUrl: '/mcp', mcpEnabled: true, panelRpcSecret: '-' }
  }
}

async function taskAction(method: string, gid: string) {
  try {
    await api.aria2(method, [gid])
    await refresh()
  } catch (caught) {
    error.value = caught instanceof Error ? caught.message : '操作失败。'
  }
}

async function restartTask(gid: string) {
  try {
    const result = await api.restartTask(gid)
    await refresh()
    activePage.value = 'tasks'
    const all = [...active.value, ...waiting.value, ...stopped.value]
    const next = all.find((task) => task.gid === result.gid)
    if (next) {
      selected.value = next
      bucket.value = next.status === 'waiting' ? 'waiting' : next.status === 'active' ? 'active' : 'stopped'
    } else {
      bucket.value = 'active'
    }
    error.value = ''
  } catch (caught) {
    error.value = caught instanceof Error ? caught.message : '任务重新开始失败。'
  }
}

async function removeTask(gid: string) {
  try {
    const result = await api.removeTask(gid)
    await refresh()
    const deletedCount = result.deletedPaths?.length || 0
    if (deletedCount > 0) {
      error.value = `任务已移除，${deletedCount} 个文件已删除。`
    } else {
      error.value = ''
    }
  } catch (caught) {
    error.value = caught instanceof Error ? caught.message : '任务移除失败。'
  }
}

async function taskMove(gid: string, position: number, how: string) {
  try {
    await api.aria2('aria2.changePosition', [gid, position, how])
    await refresh()
  } catch (caught) {
    error.value = caught instanceof Error ? caught.message : '调整队列失败。'
  }
}

async function globalAction(method: string) {
  try {
    await api.aria2(method)
    await refresh()
  } catch (caught) {
    error.value = caught instanceof Error ? caught.message : '全局操作失败。'
  }
}

async function logout() {
  await api.logout()
  emit('loggedOut')
}

function handleSettingsSaved(nextRefreshIntervalMs: number) {
  applyRefreshInterval(nextRefreshIntervalMs)
}
</script>

<template>
  <main class="shell">
    <aside class="sidebar">
      <div class="sidebar-main">
        <div class="sidebar-brand">
            <div class="sidebar-brand-mark" aria-hidden="true">
              <span class="sidebar-brand-core">A</span>
            </div>
            <div class="sidebar-brand-copy">
              <strong>Aria2MX</strong>
              <span>下载控制面板</span>
            </div>
        </div>
        <nav class="sidebar-nav">
          <button :class="{ active: activePage === 'overview' }" @click="activePage = 'overview'">
            <Gauge :size="16" /> 总览
          </button>
          <button :class="{ active: activePage === 'add' }" @click="activePage = 'add'">
            <Plus :size="16" /> 新建任务
          </button>
          <button :class="{ active: activePage === 'tasks' || activePage === 'taskDetail' }" @click="activePage = 'tasks'">
            <ListTree :size="16" /> 任务列表
          </button>
          <button :class="{ active: activePage === 'subscriptions' }" @click="activePage = 'subscriptions'">
            <Link2 :size="16" /> 节点订阅
          </button>
          <button :class="{ active: activePage === 'peerGuard' }" @click="activePage = 'peerGuard'">
            <Shield :size="16" /> 节点防护
          </button>
          <button :class="{ active: activePage === 'connection' }" @click="activePage = 'connection'">
            <Info :size="16" /> 连接信息
          </button>
          <button :class="{ active: activePage === 'mcp' }" @click="activePage = 'mcp'">
            <Blocks :size="16" /> MCP
          </button>
          <button :class="{ active: activePage === 'options' }" @click="activePage = 'options'">
            <SlidersHorizontal :size="16" /> Aria2
          </button>
          <button :class="{ active: activePage === 'scripts' }" @click="activePage = 'scripts'">
            <FileCode2 :size="16" /> 脚本设置
          </button>
          <button :class="{ active: activePage === 'settings' }" @click="activePage = 'settings'">
            <Settings :size="16" /> 面板设置
          </button>
        </nav>
      </div>
      <div class="account-strip">
        <span class="account-name">admin</span>
        <button class="ghost logout logout-inline" @click="logout">
          <LogOut :size="15" /> 退出
        </button>
      </div>
    </aside>

    <section class="workspace">
      <header class="page-header">
        <div class="page-heading">
          <h2>{{ currentPageMeta.title }}</h2>
          <p>{{ currentPageMeta.subtitle }}</p>
        </div>
        <div class="page-header-actions">
          <span class="badge">
            {{ currentPageMeta.badge }} {{ currentBucketCount }}
          </span>
          <button v-if="activePage === 'taskDetail'" class="ghost" @click="activePage = 'tasks'">
            <ArrowLeft :size="15" /> 返回列表
          </button>
          <button class="primary" @click="refresh">
            刷新
          </button>
        </div>
      </header>

      <div v-if="error" class="banner">
        {{ error }}
      </div>

      <section class="single-page">
        <section v-if="activePage === 'overview'" class="overview-page">
          <div class="topbar">
            <MetricCard label="下载" :value="speed(stat.downloadSpeed)" tone="accent" />
            <MetricCard label="上传" :value="speed(stat.uploadSpeed)" />
            <MetricCard label="活动" :value="stat.numActive" />
            <MetricCard label="等待" :value="stat.numWaiting" tone="warn" />
            <MetricCard label="历史" :value="stat.numStopped" />
          </div>

          <section class="panel overview-actions">
            <div class="section-title">
              <Activity :size="17" />
              <span>快速操作</span>
            </div>
            <div class="bulk-actions">
              <button class="ghost" @click="globalAction('aria2.pauseAll')">
                <Pause :size="15" /> 暂停全部
              </button>
              <button class="ghost" @click="globalAction('aria2.unpauseAll')">
                <Play :size="15" /> 继续全部
              </button>
              <button class="ghost" @click="globalAction('aria2.purgeDownloadResult')">
                <Eraser :size="15" /> 清理结果
              </button>
              <button class="ghost" @click="globalAction('aria2.saveSession')">
                <Save :size="15" /> 保存会话
              </button>
            </div>
          </section>

          <section class="panel recent-panel">
            <div class="section-title">
              <ListTree :size="17" />
              <span>最近任务</span>
            </div>
            <TaskList
              :tasks="allTasks.slice(0, 8)"
              :selected-gid="selected?.gid"
              @select="openTask"
              @action="taskAction"
              @move="taskMove"
              @restart="restartTask"
              @remove="removeTask"
            />
          </section>
        </section>

        <section v-else-if="activePage === 'tasks'" class="page-shell">
          <section class="tasks-page">
            <section class="panel queue-panel">
              <div class="queue-head">
                <div class="section-title">
                  <Activity :size="17" />
                  <span>任务队列</span>
                </div>
                <div class="segmented">
                  <button :class="{ active: bucket === 'active' }" @click="bucket = 'active'">
                    活动
                  </button>
                  <button :class="{ active: bucket === 'waiting' }" @click="bucket = 'waiting'">
                    等待
                  </button>
                  <button :class="{ active: bucket === 'stopped' }" @click="bucket = 'stopped'">
                    历史
                  </button>
                </div>
              </div>
              <div class="queue-toolbar">
                <label class="search-box">
                  <Search :size="15" />
                  <input v-model="query" placeholder="搜索名称、GID、目录、状态">
                </label>
                <select v-model="sortBy">
                  <option value="name">
                    按名称
                  </option>
                  <option value="progress">
                    按进度
                  </option>
                  <option value="speed">
                    按速度
                  </option>
                  <option value="size">
                    按大小
                  </option>
                </select>
              </div>
              <div class="bulk-actions">
                <button class="ghost" @click="globalAction('aria2.pauseAll')">
                  <Pause :size="15" /> 暂停全部
                </button>
                <button class="ghost" @click="globalAction('aria2.unpauseAll')">
                  <Play :size="15" /> 继续全部
                </button>
                <button class="ghost" @click="globalAction('aria2.purgeDownloadResult')">
                  <Eraser :size="15" /> 清理结果
                </button>
                <button class="ghost" @click="globalAction('aria2.saveSession')">
                  <Save :size="15" /> 保存会话
                </button>
              </div>
              <TaskList
                :tasks="visibleTasks"
                :selected-gid="selected?.gid"
                @select="openTask"
                @action="taskAction"
                @move="taskMove"
                @restart="restartTask"
                @remove="removeTask"
              />
            </section>
          </section>
        </section>
        <section v-else-if="activePage === 'taskDetail'" class="page-shell">
          <TaskDetail :task="selected" @changed="refresh" />
        </section>

        <section v-else-if="activePage === 'add'" class="page-shell">
          <AddTaskPanel @created="refresh" />
        </section>
        <section v-else-if="activePage === 'subscriptions'" class="page-shell">
          <NodeSubscriptionPanel />
        </section>
        <section v-else-if="activePage === 'options'" class="page-shell">
          <Aria2OptionsPanel />
        </section>
        <section v-else-if="activePage === 'scripts'" class="page-shell">
          <ScriptSettingsPanel />
        </section>
        <section v-else-if="activePage === 'peerGuard'" class="page-shell">
          <PeerGuardPanel />
        </section>
        <section v-else-if="activePage === 'connection'" class="page-shell">
          <ConnectionInfoPanel />
        </section>
        <section v-else-if="activePage === 'mcp'" class="page-shell">
          <MCPPanel />
        </section>
        <section v-else class="page-shell">
          <SettingsPanel @saved="handleSettingsSaved" />
        </section>
      </section>
    </section>
  </main>
</template>
