<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Settings } from 'lucide-vue-next'
import { api } from '@/api'

const aria2RpcUrl = ref('')
const aria2Secret = ref('')
const refreshIntervalMs = ref(1500)
const defaultDownloadDir = ref('')
const newPassword = ref('')
const hasSecret = ref(false)
const message = ref('')
const loading = ref(false)

onMounted(load)

async function load() {
  const config = await api.getConfig()
  aria2RpcUrl.value = config.aria2RpcUrl
  refreshIntervalMs.value = config.refreshIntervalMs
  defaultDownloadDir.value = config.defaultDownloadDir
  hasSecret.value = config.hasAria2Secret
}

async function save() {
  loading.value = true
  message.value = ''
  try {
    const payload: Record<string, string | number | undefined> = {
      aria2RpcUrl: aria2RpcUrl.value,
      refreshIntervalMs: refreshIntervalMs.value,
      defaultDownloadDir: defaultDownloadDir.value,
      newPassword: newPassword.value || undefined,
    }
    if (aria2Secret.value) payload.aria2Secret = aria2Secret.value
    await api.updateConfig(payload)
    aria2Secret.value = ''
    newPassword.value = ''
    await load()
    message.value = '设置已保存。'
  } catch (error) {
    message.value = error instanceof Error ? error.message : '设置保存失败。'
  } finally {
    loading.value = false
  }
}

async function testConnection() {
  loading.value = true
  message.value = ''
  try {
    await api.aria2('aria2.getVersion')
    message.value = 'aria2 连接正常。'
  } catch (error) {
    message.value = error instanceof Error ? error.message : '连接测试失败。'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <section class="panel settings-panel">
    <div class="section-title">
      <Settings :size="17" />
      <span>设置</span>
    </div>
    <label>
      <span>aria2 RPC 地址</span>
      <input v-model="aria2RpcUrl">
    </label>
    <label>
      <span>RPC Secret {{ hasSecret ? '（已设置）' : '' }}</span>
      <input v-model="aria2Secret" type="password" placeholder="留空不修改">
    </label>
    <label>
      <span>默认下载目录</span>
      <input v-model="defaultDownloadDir" placeholder="留空使用 aria2 默认值">
    </label>
    <label>
      <span>刷新间隔 ms</span>
      <input v-model.number="refreshIntervalMs" type="number" min="500" step="100">
    </label>
    <label>
      <span>新面板密码</span>
      <input v-model="newPassword" type="password" placeholder="至少 6 位，留空不修改">
    </label>
    <div class="button-row">
      <button class="primary" :disabled="loading" @click="save">
        保存
      </button>
      <button class="ghost" :disabled="loading" @click="testConnection">
        测试连接
      </button>
    </div>
    <p v-if="message" class="hint">
      {{ message }}
    </p>
  </section>
</template>
