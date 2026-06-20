<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ExternalLink, Link2 } from 'lucide-vue-next'
import { api } from '@/api'
import { builtInTrackerSubscriptions } from '@/data/nodeSubscriptions'

const loading = ref(false)
const enabled = ref(false)
const selectedSource = ref('')
const message = ref('')
const currentTrackerCount = ref(0)

const selectedEntry = computed(() => {
  return builtInTrackerSubscriptions.find((item) => item.key === selectedSource.value) || builtInTrackerSubscriptions[0]
})

const overviewItems = computed(() => [
  { label: '当前状态', value: enabled.value ? '已开启' : '已关闭' },
  { label: '当前来源', value: selectedEntry.value?.name || '未选择' },
  { label: '已应用节点', value: `${currentTrackerCount.value} 条` },
])

onMounted(load)

async function load() {
  loading.value = true
  try {
    const state = await api.getTrackerSubscription()
    enabled.value = state.enabled
    selectedSource.value = state.selectedSource || builtInTrackerSubscriptions[0]?.key || ''
    currentTrackerCount.value = state.currentTrackerCount
    message.value = state.message || ''
  } finally {
    loading.value = false
  }
}

async function save() {
  loading.value = true
  message.value = ''
  try {
    const state = await api.updateTrackerSubscription(enabled.value, selectedSource.value)
    enabled.value = state.enabled
    selectedSource.value = state.selectedSource || selectedSource.value
    currentTrackerCount.value = state.currentTrackerCount
    message.value = state.message || '节点订阅已保存。'
  } catch (error) {
    message.value = error instanceof Error ? error.message : '节点订阅保存失败。'
  }
  loading.value = false
}
</script>

<template>
  <section class="panel node-subscription-panel">
    <div class="section-title">
      <Link2 :size="17" />
      <span>节点订阅</span>
    </div>

    <section class="settings-overview-grid">
      <article v-for="item in overviewItems" :key="item.label" class="settings-overview-card">
        <span>{{ item.label }}</span>
        <strong>{{ item.value }}</strong>
      </article>
    </section>

    <article class="node-subscription-card">
      <div class="settings-field-grid">
        <label>
            <span>启用节点订阅</span>
          <select v-model="enabled" :disabled="loading">
            <option :value="true">
              开启
            </option>
            <option :value="false">
              关闭
            </option>
          </select>
        </label>
        <label>
          <span>订阅源</span>
          <select v-model="selectedSource" :disabled="loading || !enabled">
            <option v-for="item in builtInTrackerSubscriptions" :key="item.key" :value="item.key">
              {{ item.name }}
            </option>
          </select>
        </label>
      </div>
      <p class="hint">
        开启后，保存时会立即拉取订阅源，之后每 24 小时自动同步一次。
      </p>
      <div v-if="selectedEntry" class="node-subscription-list">
        <article class="node-subscription-card">
          <div class="node-subscription-card-head">
            <div>
              <h3>{{ selectedEntry.name }}</h3>
              <p>{{ selectedEntry.summary }}</p>
            </div>
          </div>

          <div class="node-subscription-tags">
            <span v-for="tag in selectedEntry.tags" :key="`${selectedEntry.key}-${tag}`">
              {{ tag }}
            </span>
          </div>

          <div class="node-subscription-lines">
            <div class="node-subscription-line">
              <span>订阅地址</span>
              <strong>{{ selectedEntry.url }}</strong>
            </div>
            <div class="node-subscription-line">
              <span>说明</span>
              <strong>{{ selectedEntry.note }}</strong>
            </div>
          </div>

          <div class="node-subscription-actions">
            <button class="primary" type="button" :disabled="loading" @click="save">
              保存并应用
            </button>
            <a class="ghost button-link" :href="selectedEntry.homepage" target="_blank" rel="noreferrer">
              <ExternalLink :size="15" /> 打开来源
            </a>
          </div>
        </article>
      </div>
    </article>
    <p v-if="message" class="hint">
      {{ message }}
    </p>
  </section>
</template>

<style scoped>
.node-subscription-panel {
  display: grid;
  height: 100%;
  min-height: 0;
  grid-template-rows: auto auto minmax(0, 1fr) auto;
  gap: 16px;
}

.node-subscription-card {
  display: grid;
  align-content: start;
  min-height: 0;
  border: 1px solid var(--soft-line);
  border-radius: 12px;
  background: var(--node-subscription-card-bg, var(--panel));
  padding: 16px;
}

.node-subscription-list {
  display: grid;
  gap: 14px;
  margin-top: 16px;
}

.node-subscription-card-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.node-subscription-card-head h3,
.node-subscription-card-head p {
  margin: 0;
}

.node-subscription-card-head p {
  margin-top: 6px;
  color: var(--muted);
  font-size: 13px;
}

.node-subscription-format {
  flex: 0 0 auto;
  padding: 6px 10px;
  border-radius: 999px;
  background: var(--soft);
  color: var(--muted);
  font-size: 12px;
  font-weight: 700;
}

.node-subscription-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.node-subscription-tags span {
  padding: 4px 8px;
  border-radius: 999px;
  background: color-mix(in srgb, var(--accent) 10%, transparent);
  color: var(--accent);
  font-size: 12px;
  font-weight: 700;
}

.node-subscription-lines {
  display: grid;
  gap: 10px;
}

.node-subscription-line {
  display: grid;
  gap: 4px;
}

.node-subscription-line span {
  color: var(--muted);
  font-size: 12px;
}

.node-subscription-line strong {
  font-size: 13px;
  line-height: 1.5;
  word-break: break-all;
}

.node-subscription-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}

.button-link {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  text-decoration: none;
}

:global([data-skin='custom']) .node-subscription-card {
  --node-subscription-card-bg: color-mix(in srgb, var(--panel) 18%, transparent);
  border-color: color-mix(in srgb, var(--soft-line) 26%, transparent);
  backdrop-filter: blur(10px);
  -webkit-backdrop-filter: blur(10px);
  background-clip: padding-box;
  isolation: isolate;
  box-shadow: none;
}
</style>
