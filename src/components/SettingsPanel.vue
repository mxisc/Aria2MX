<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Settings } from 'lucide-vue-next'
import { api } from '@/api'
import { applyTheme, type AppTheme } from '@/theme'

const aria2RpcUrl = ref('')
const aria2Secret = ref('')
const refreshIntervalMs = ref(1500)
const defaultDownloadDir = ref('')
const theme = ref<AppTheme>('design')
const newPassword = ref('')
const hasSecret = ref(false)
const aria2Managed = ref(false)
const managedRpcPort = ref(16800)
const message = ref('')
const loading = ref(false)

onMounted(load)

async function load() {
  const config = await api.getConfig()
  aria2RpcUrl.value = config.aria2RpcUrl
  refreshIntervalMs.value = config.refreshIntervalMs
  defaultDownloadDir.value = config.defaultDownloadDir
  theme.value = config.theme
  hasSecret.value = config.hasAria2Secret
  aria2Managed.value = config.aria2Managed
  managedRpcPort.value = config.managedRpcPort
  applyTheme(config.theme)
}

async function save() {
  loading.value = true
  message.value = ''
  try {
    const payload: Record<string, string | number | undefined> = {
      refreshIntervalMs: refreshIntervalMs.value,
      defaultDownloadDir: defaultDownloadDir.value,
      theme: theme.value,
      newPassword: newPassword.value || undefined,
    }
    if (!aria2Managed.value) {
      payload.aria2RpcUrl = aria2RpcUrl.value
      if (aria2Secret.value) payload.aria2Secret = aria2Secret.value
    }
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
      <span>面板设置</span>
    </div>
    <label v-if="!aria2Managed">
      <span>aria2 RPC 地址</span>
      <input v-model="aria2RpcUrl">
    </label>
    <label v-if="!aria2Managed">
      <span>RPC Secret {{ hasSecret ? '（已设置）' : '' }}</span>
      <input v-model="aria2Secret" type="password" placeholder="留空不修改">
    </label>
    <p v-else class="hint">
      当前为 all-in-one 模式，aria2 由面板内置托管。面板层统一提供 `/jsonrpc` 代理；在 TLS 入口下可直接以 HTTPS / WSS 方式访问该路径。当前内置 aria2 本地 RPC 端口为 {{ managedRpcPort }}。
    </p>
    <label>
      <span>默认下载目录</span>
      <input v-model="defaultDownloadDir" placeholder="留空使用 aria2 默认值">
    </label>
    <label>
      <span>界面皮肤</span>
      <select v-model="theme">
        <option value="design">
          设计稿皮肤
        </option>
        <option value="classic">
          经典皮肤
        </option>
      </select>
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
      <button v-if="!aria2Managed" class="ghost" :disabled="loading" @click="testConnection">
        测试连接
      </button>
    </div>
    <p v-if="message" class="hint">
      {{ message }}
    </p>
  </section>
</template>
