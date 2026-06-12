<script setup lang="ts">
import { ArrowDownToLine, ArrowUpToLine, Pause, Play, RotateCcw, Trash2, Zap } from 'lucide-vue-next'
import type { Aria2Task } from '@/types'
import { bytes, percent, speed, statusLabel, taskName } from '@/utils/format'

defineProps<{
  tasks: Aria2Task[]
  selectedGid?: string
}>()

defineEmits<{
  select: [task: Aria2Task]
  action: [method: string, gid: string]
  move: [gid: string, position: number, how: string]
}>()
</script>

<template>
  <div class="task-list">
    <button
      v-for="task in tasks"
      :key="task.gid"
      class="task-row"
      :class="{ selected: task.gid === selectedGid }"
      @click="$emit('select', task)"
    >
      <span class="status-dot" :class="task.status" />
      <span class="task-main">
        <span class="task-title">{{ taskName(task) }}</span>
        <span class="task-meta">
          {{ statusLabel(task.status) }} · {{ bytes(task.completedLength) }} / {{ bytes(task.totalLength) }} · {{ speed(task.downloadSpeed) }}
        </span>
        <span class="progress-track">
          <span class="progress-fill" :style="{ width: `${percent(task)}%` }" />
        </span>
      </span>
      <span class="task-percent">{{ percent(task) }}%</span>
      <span class="task-actions" @click.stop>
        <button title="暂停" @click="$emit('action', 'aria2.pause', task.gid)">
          <Pause :size="15" />
        </button>
        <button title="强制暂停" @click="$emit('action', 'aria2.forcePause', task.gid)">
          <Zap :size="15" />
        </button>
        <button title="继续" @click="$emit('action', 'aria2.unpause', task.gid)">
          <Play :size="15" />
        </button>
        <button title="移动到顶部" @click="$emit('move', task.gid, 0, 'POS_SET')">
          <ArrowUpToLine :size="15" />
        </button>
        <button title="移动到底部" @click="$emit('move', task.gid, -1, 'POS_END')">
          <ArrowDownToLine :size="15" />
        </button>
        <button title="重新开始" @click="$emit('action', 'aria2.removeDownloadResult', task.gid)">
          <RotateCcw :size="15" />
        </button>
        <button
          title="移除"
          @click="$emit('action', ['complete', 'error', 'removed'].includes(task.status) ? 'aria2.removeDownloadResult' : 'aria2.remove', task.gid)"
        >
          <Trash2 :size="15" />
        </button>
      </span>
    </button>
    <div v-if="tasks.length === 0" class="empty-state">
      没有任务。新建一个下载，或者检查 aria2 连接。
    </div>
  </div>
</template>
