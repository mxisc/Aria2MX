<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { Activity, Download, Eraser, LogOut, Pause, Play, Plus, Save, Search, SlidersHorizontal } from 'lucide-vue-next'
import { api, fetchDashboard } from '@/api'
import AddTaskPanel from '@/components/AddTaskPanel.vue'
import Aria2OptionsPanel from '@/components/Aria2OptionsPanel.vue'
import MetricCard from '@/components/MetricCard.vue'
import SettingsPanel from '@/components/SettingsPanel.vue'
import TaskDetail from '@/components/TaskDetail.vue'
import TaskList from '@/components/TaskList.vue'
import type { Aria2Task, GlobalStat, TaskBucket } from '@/types'
import { percent, speed, taskName } from '@/utils/format'

const emit = defineEmits<{ loggedOut: [] }>()

const active = ref<Aria2Task[]>([])
const waiting = ref<Aria2Task[]>([])
const stopped = ref<Aria2Task[]>([])
const stat = ref<GlobalStat>({ downloadSpeed: '0', uploadSpeed: '0', numActive: '0', numWaiting: '0', numStopped: '0' })
const selected = ref<Aria2Task>()
const bucket = ref<TaskBucket>('active')
const panel = ref<'add' | 'options' | 'settings'>('add')
const query = ref('')
const sortBy = ref<'name' | 'progress' | 'speed' | 'size'>('name')
const error = ref('')
let timer: number | undefined

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
      <div>
        <p class="eyebrow">
          AriaMX
        </p>
        <h1>下载面板</h1>
      </div>
      <nav>
        <button :class="{ active: panel === 'add' }" @click="panel = 'add'">
          <Plus :size="16" /> 新建
        </button>
        <button :class="{ active: panel === 'options' }" @click="panel = 'options'">
          <SlidersHorizontal :size="16" /> aria2 选项
        </button>
        <button :class="{ active: panel === 'settings' }" @click="panel = 'settings'">
          <SlidersHorizontal :size="16" /> 设置
        </button>
      </nav>
      <button class="ghost logout" @click="logout">
        <LogOut :size="16" /> 退出
      </button>
    </aside>

    <section class="workspace">
      <header class="topbar">
        <MetricCard label="下载" :value="speed(stat.downloadSpeed)" tone="accent" />
        <MetricCard label="上传" :value="speed(stat.uploadSpeed)" />
        <MetricCard label="活动" :value="stat.numActive" />
        <MetricCard label="等待" :value="stat.numWaiting" tone="warn" />
        <MetricCard label="历史" :value="stat.numStopped" />
      </header>

      <div v-if="error" class="banner">
        {{ error }}
      </div>

      <section class="content-grid">
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

        <section class="side-stack">
          <AddTaskPanel v-if="panel === 'add'" @created="refresh" />
          <Aria2OptionsPanel v-else-if="panel === 'options'" />
          <SettingsPanel v-else />
          <section class="panel compact-note">
            <div class="section-title">
              <Download :size="17" /><span>部署目标</span>
            </div>
            <p>前端构建产物会嵌入 Go 二进制，发布时只需要携带 `ariamx` 可执行文件和运行时配置。</p>
          </section>
        </section>
      </section>
    </section>
  </main>
</template>
