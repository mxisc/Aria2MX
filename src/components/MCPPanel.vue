<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Blocks } from 'lucide-vue-next'
import { api } from '@/api'

const loading = ref(false)
const error = ref('')
const mcpEnabled = ref(true)

const toolList = [
  'aria2_get_version',
  'aria2_get_global_stat',
  'aria2_get_global_option',
  'aria2_tell_active',
  'aria2_tell_waiting',
  'aria2_tell_stopped',
  'aria2_add_uri',
  'aria2_pause',
  'aria2_unpause',
  'aria2_remove',
  'aria2_pause_all',
  'aria2_unpause_all',
  'aria2_save_session',
]

onMounted(load)

async function load() {
  loading.value = true
  error.value = ''
  try {
    const config = await api.getConfig()
    mcpEnabled.value = config.mcpEnabled
  } catch (caught) {
    error.value = caught instanceof Error ? caught.message : 'MCP 信息读取失败。'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <section class="panel connection-info-panel">
    <div class="section-title">
      <Blocks :size="17" />
      <span>MCP</span>
    </div>
    <div v-if="error" class="banner">
      {{ error }}
    </div>
    <div v-else-if="!mcpEnabled" class="banner">
      MCP 当前已关闭，可在面板设置中开启。
    </div>
    <div v-else class="connection-layout">
      <section class="connection-section-list">
        <article class="connection-section-card connection-tools-card">
          <div class="connection-section-head">
            <div>
              <h3>可用工具</h3>
              <p>当前 MCP 对外暴露的工具列表。</p>
            </div>
          </div>
          <ul class="connection-tool-list">
            <li v-for="tool in toolList" :key="tool">
              <code>{{ tool }}</code>
            </li>
          </ul>
        </article>
      </section>
    </div>
  </section>
</template>
