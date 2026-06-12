<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { SlidersHorizontal } from 'lucide-vue-next'
import { api } from '@/api'
import type { Aria2OptionMap } from '@/types'

const options = ref<Aria2OptionMap>({})
const draft = ref('')
const loading = ref(false)
const message = ref('')

const quickKeys = [
  'dir',
  'max-concurrent-downloads',
  'max-connection-per-server',
  'split',
  'min-split-size',
  'max-overall-download-limit',
  'max-overall-upload-limit',
  'seed-time',
  'seed-ratio',
  'bt-tracker',
  'user-agent',
]

onMounted(load)

async function load() {
  loading.value = true
  message.value = ''
  try {
    options.value = await api.aria2<Aria2OptionMap>('aria2.getGlobalOption')
    draft.value = Object.entries(options.value)
      .sort(([left], [right]) => left.localeCompare(right))
      .map(([key, value]) => `${key}=${value}`)
      .join('\n')
  } catch (error) {
    message.value = error instanceof Error ? error.message : '全局选项读取失败。'
  } finally {
    loading.value = false
  }
}

async function save() {
  const patch: Record<string, string> = {}
  for (const line of draft.value.split('\n')) {
    const trimmed = line.trim()
    if (!trimmed || trimmed.startsWith('#')) continue
    const index = trimmed.indexOf('=')
    if (index <= 0) continue
    const key = trimmed.slice(0, index).trim()
    const value = trimmed.slice(index + 1).trim()
    if (options.value[key] !== value) patch[key] = value
  }
  if (Object.keys(patch).length === 0) {
    message.value = '没有需要保存的变化。'
    return
  }
  loading.value = true
  message.value = ''
  try {
    await api.aria2('aria2.changeGlobalOption', [patch])
    message.value = '全局选项已保存。'
    await load()
  } catch (error) {
    message.value = error instanceof Error ? error.message : '全局选项保存失败。'
  } finally {
    loading.value = false
  }
}

function syncDraft() {
  draft.value = Object.entries(options.value)
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([key, value]) => `${key}=${value}`)
    .join('\n')
}

function updateQuickOption(key: string, event: Event) {
  options.value[key] = (event.target as HTMLInputElement).value
  syncDraft()
}
</script>

<template>
  <section class="panel options-panel">
    <div class="section-title">
      <SlidersHorizontal :size="17" />
      <span>aria2 全局选项</span>
    </div>
    <div class="quick-options">
      <label v-for="key in quickKeys" :key="key">
        <span>{{ key }}</span>
        <input
          :value="options[key] || ''"
          @input="updateQuickOption(key, $event)"
        >
      </label>
    </div>
    <p class="hint">
      高级选项每行一个 `key=value`，只会保存发生变化的项。
    </p>
    <textarea v-model="draft" spellcheck="false" />
    <div class="button-row">
      <button class="primary" :disabled="loading" @click="save">
        保存全局选项
      </button>
      <button class="ghost" :disabled="loading" @click="load">
        重新读取
      </button>
    </div>
    <p v-if="message" class="hint">
      {{ message }}
    </p>
  </section>
</template>
