<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { Activity, Eraser, Gauge, ListTree, LogOut, Pause, Play, Plus, Save, Search, Settings, SlidersHorizontal } from 'lucide-vue-next'
import { api, fetchDashboard } from '@/api'
import AddTaskPanel from '@/components/AddTaskPanel.vue'
import Aria2OptionsPanel from '@/components/Aria2OptionsPanel.vue'
import MetricCard from '@/components/MetricCard.vue'
import SettingsPanel from '@/components/SettingsPanel.vue'
import TaskDetail from '@/components/TaskDetail.vue'
import TaskList from '@/components/TaskList.vue'
import type { AppAbout, Aria2Task, GlobalStat, TaskBucket } from '@/types'
import { percent, speed, taskName } from '@/utils/format'

const emit = defineEmits<{ loggedOut: [] }>()

const active = ref<Aria2Task[]>([])
const waiting = ref<Aria2Task[]>([])
const stopped = ref<Aria2Task[]>([])
const stat = ref<GlobalStat>({ downloadSpeed: '0', uploadSpeed: '0', numActive: '0', numWaiting: '0', numStopped: '0' })
const selected = ref<Aria2Task>()
const bucket = ref<TaskBucket>('active')
const activePage = ref<'overview' | 'tasks' | 'add' | 'options' | 'settings'>('overview')
const query = ref('')
const sortBy = ref<'name' | 'progress' | 'speed' | 'size'>('name')
const error = ref('')
const about = ref<AppAbout>({ panelVersion: '-', aria2Version: '-', rpcPath: '/jsonrpc' })
let timer: number | undefined

const pageMeta = {
  overview: { title: '总览', subtitle: '查看下载速度、任务数量和常用操作。', badge: '控制台' },
  tasks: { title: '任务列表', subtitle: '筛选、检索并管理当前下载任务。', badge: '任务列表' },
  add: { title: '新建任务', subtitle: '提交链接、磁力或种子文件。', badge: '新建任务' },
  options: { title: 'Aria2', subtitle: '按分类维护 aria2 全局参数。', badge: 'Aria2' },
  settings: { title: '面板设置', subtitle: '维护 RPC、刷新间隔和面板登录配置。', badge: '面板设置' },
} as const

const allTasks = computed(() => [...active.value, ...waiting.value, ...stopped.value])
const currentPageMeta = computed(() => pageMeta[activePage.value])
const currentBucketCount = computed(() => {
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

onMounted(() => {
  refresh()
  loadAbout()
  timer = window.setInterval(refresh, 1500)
})

onUnmounted(() => {
  if (timer) window.clearInterval(timer)
})

async function refresh() {
  try {
    const data = await fetchDashboard()
    active.value = data.active
    waiting.value = data.waiting
    stopped.value = data.stopped
    stat.value = data.stat
    const all = [...data.active, ...data.waiting, ...data.stopped]
    selected.value = all.find((task) => task.gid === selected.value?.gid) || all[0]
    error.value = ''
  } catch (caught) {
    error.value = caught instanceof Error ? caught.message : '无法连接 aria2。'
  }
}

async function loadAbout() {
  try {
    about.value = await api.getAbout()
  } catch {
    about.value = { panelVersion: '-', aria2Version: '-', rpcPath: '/jsonrpc' }
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
</script>

<template>
  <main class="shell">
    <aside class="sidebar">
      <div class="sidebar-main">
        <div class="sidebar-brand">
          <strong>AriaMX</strong>
          <span>面板 v{{ about.panelVersion || '-' }}</span>
          <span>aria2 v{{ about.aria2Version || '-' }}</span>
        </div>
        <nav class="sidebar-nav">
          <button :class="{ active: activePage === 'overview' }" @click="activePage = 'overview'">
            <Gauge :size="16" /> 总览
          </button>
          <button :class="{ active: activePage === 'tasks' }" @click="activePage = 'tasks'">
            <ListTree :size="16" /> 任务列表
          </button>
          <button :class="{ active: activePage === 'add' }" @click="activePage = 'add'">
            <Plus :size="16" /> 新建任务
          </button>
          <button :class="{ active: activePage === 'options' }" @click="activePage = 'options'">
            <SlidersHorizontal :size="16" /> Aria2
          </button>
          <button :class="{ active: activePage === 'settings' }" @click="activePage = 'settings'">
            <Settings :size="16" /> 面板设置
          </button>
        </nav>
      </div>
      <div class="user-card">
        <div class="user-card-head">
          <span class="avatar">A</span>
          <div class="user-meta">
            <strong>admin</strong>
            <small>管理员会话</small>
          </div>
        </div>
        <button class="ghost logout" @click="logout">
          <LogOut :size="16" /> 退出登录
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
              @select="selected = $event; activePage = 'tasks'"
              @action="taskAction"
              @move="taskMove"
            />
          </section>
        </section>

        <section v-else-if="activePage === 'tasks'" class="tasks-page">
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
            <TaskList :tasks="visibleTasks" :selected-gid="selected?.gid" @select="selected = $event" @action="taskAction" @move="taskMove" />
          </section>

          <TaskDetail :task="selected" @changed="refresh" />
        </section>

        <AddTaskPanel v-else-if="activePage === 'add'" @created="refresh" />
        <Aria2OptionsPanel v-else-if="activePage === 'options'" />
        <SettingsPanel v-else />
      </section>
    </section>
  </main>
</template>
