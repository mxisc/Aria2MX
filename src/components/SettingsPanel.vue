<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { Settings } from 'lucide-vue-next'
import { api } from '@/api'
import { applySkin, applyTheme, type ColorMode } from '@/theme'

const emit = defineEmits<{ saved: [refreshIntervalMs: number] }>()

const aria2RpcUrl = ref('')
const aria2Secret = ref('')
const refreshIntervalMs = ref(1500)
const defaultDownloadDir = ref('')
const colorMode = ref<ColorMode>('system')
const skinEnabled = ref(false)
const skinName = ref('default')
const skinApiTemplate = ref('')
const mcpEnabled = ref(true)
const rpcOriginCheckMode = ref<'disabled' | 'same_origin' | 'whitelist'>('same_origin')
const rpcOriginWhitelistText = ref('')
const newPassword = ref('')
const hasSecret = ref(false)
const aria2Managed = ref(false)
const message = ref('')
const loading = ref(false)

const originModeTextMap = {
  disabled: '关闭Origin校验',
  same_origin: '开启Origin校验',
  whitelist: '白名单模式',
} as const

const overviewItems = computed(() => [
  { label: 'aria2 接入', value: aria2Managed.value ? '内置托管' : '外部连接' },
  { label: 'RPC 同源校验', value: originModeTextMap[rpcOriginCheckMode.value] },
  { label: 'MCP', value: mcpEnabled.value ? '已开启' : '已关闭' },
  { label: '刷新间隔', value: `${refreshIntervalMs.value} ms` },
])

const whitelistHint = computed(() => {
  const count = rpcOriginWhitelistText.value
    .split('\n')
    .map((item) => item.trim())
    .filter(Boolean).length
  return count > 0 ? `当前已配置 ${count} 条白名单来源。` : '逐行填写域名或完整来源地址。'
})

onMounted(load)

async function load() {
  const config = await api.getConfig()
  aria2RpcUrl.value = config.aria2RpcUrl
  refreshIntervalMs.value = config.refreshIntervalMs
  defaultDownloadDir.value = config.defaultDownloadDir
  colorMode.value = config.colorMode
  skinEnabled.value = config.skinEnabled
  skinName.value = config.skinName
  skinApiTemplate.value = config.skinApiTemplate
  mcpEnabled.value = config.mcpEnabled
  rpcOriginCheckMode.value = config.rpcOriginCheckMode
  rpcOriginWhitelistText.value = config.rpcOriginWhitelist.join('\n')
  hasSecret.value = config.hasAria2Secret
  aria2Managed.value = config.aria2Managed
  applyTheme(config.theme, config.colorMode)
  applySkin(config.skinEnabled, config.skinName, config.skinApiTemplate)
}

async function save() {
  loading.value = true
  message.value = ''
  try {
    const payload: Record<string, unknown> = {
      refreshIntervalMs: refreshIntervalMs.value,
      defaultDownloadDir: defaultDownloadDir.value,
      mcpEnabled: mcpEnabled.value,
      rpcOriginCheckMode: rpcOriginCheckMode.value,
      theme: 'ariamx',
      colorMode: colorMode.value,
      skinEnabled: skinEnabled.value,
      skinName: skinName.value,
      skinApiTemplate: skinApiTemplate.value,
      newPassword: newPassword.value || undefined,
      rpcOriginWhitelist: rpcOriginWhitelistText.value
        .split('\n')
        .map((item) => item.trim())
        .filter(Boolean),
    }
    if (!aria2Managed.value) {
      payload.aria2RpcUrl = aria2RpcUrl.value
      if (aria2Secret.value) payload.aria2Secret = aria2Secret.value
    }
    await api.updateConfig(payload)
    aria2Secret.value = ''
    newPassword.value = ''
    await load()
    emit('saved', refreshIntervalMs.value)
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
    <section class="settings-overview-grid">
      <article v-for="item in overviewItems" :key="item.label" class="settings-overview-card">
        <span>{{ item.label }}</span>
        <strong>{{ item.value }}</strong>
      </article>
    </section>
    <div class="settings-section-list">
      <article class="settings-section-card">
        <div class="settings-section-head">
          <div>
            <h3>面板行为</h3>
          </div>
        </div>
        <div class="settings-field-grid">
          <label>
            <span>默认下载目录</span>
            <input v-model="defaultDownloadDir" placeholder="留空使用 aria2 默认值">
          </label>
          <label>
            <span>刷新间隔 ms</span>
            <input v-model.number="refreshIntervalMs" type="number" min="500" step="100">
          </label>
          <label>
            <span>显示模式</span>
            <select v-model="colorMode">
              <option value="system">
                跟随系统
              </option>
              <option value="light">
                浅色模式
              </option>
              <option value="dark">
                深色模式
              </option>
            </select>
          </label>
          <label>
            <span>MCP</span>
            <select v-model="mcpEnabled">
              <option :value="true">
                开启
              </option>
              <option :value="false">
                关闭
              </option>
            </select>
          </label>
          <label>
            <span>背景图片</span>
            <select v-model="skinEnabled">
              <option :value="false">
                关闭
              </option>
              <option :value="true">
                开启
              </option>
            </select>
          </label>
          <label class="settings-field-span-2">
            <span>背景图片地址</span>
            <input
              v-model="skinApiTemplate"
              placeholder="填写图片直链或返回图片内容的 API 地址"
            >
          </label>
          <p class="hint settings-field-span-2">
            支持图片直链或直接返回图片内容的 HTTPS 地址。背景图由浏览器直接加载；如果接口会跳到 HTTP、要求特殊鉴权，或最终不返回图片，页面就不会显示。
          </p>
        </div>
      </article>

      <article class="settings-section-card">
        <div class="settings-section-head">
          <div>
            <h3>接入与安全</h3>
          </div>
        </div>
        <div class="settings-field-grid">
          <label class="settings-field-span-2">
            <span>同源校验</span>
            <select v-model="rpcOriginCheckMode">
              <option value="disabled">
                关闭Origin校验
              </option>
              <option value="same_origin">
                开启Origin校验
              </option>
              <option value="whitelist">
                白名单模式
              </option>
            </select>
          </label>
          <p class="hint settings-field-span-2">
            开启Origin校验[默认]：无法进行跨站调用。关闭Origin校验：允许任何网站调用。白名单模式：只允许特定网站调用。 注：AriaNg等调用需要开启。
          </p>
          <label v-if="rpcOriginCheckMode === 'whitelist'" class="settings-field-span-2">
            <span>允许的来源白名单</span>
            <textarea
              v-model="rpcOriginWhitelistText"
              class="small-textarea"
              placeholder="逐行填写域名或完整来源地址，例如&#10;ariang.example.com&#10;https://panel.example.com"
            />
          </label>
          <p v-if="rpcOriginCheckMode === 'whitelist'" class="hint settings-field-span-2">
            {{ whitelistHint }}
          </p>
          <label v-if="!aria2Managed">
            <span>aria2 RPC 地址</span>
            <input v-model="aria2RpcUrl">
          </label>
          <label v-if="!aria2Managed">
            <span>RPC Secret {{ hasSecret ? '（已设置）' : '' }}</span>
            <input v-model="aria2Secret" type="password" placeholder="留空不修改">
          </label>
          <label class="settings-field-span-2">
            <span>新面板密码</span>
            <input v-model="newPassword" type="password" placeholder="至少 6 位，留空不修改">
          </label>
        </div>
      </article>
    </div>
    <div class="button-row settings-action-row">
      <button class="primary" :disabled="loading" @click="save">
        保存
      </button>
      <button v-if="!aria2Managed" class="ghost" :disabled="loading" @click="testConnection">
        测试连接
      </button>
    </div>
    <p v-if="message" class="hint settings-message">
      {{ message }}
    </p>
  </section>
</template>
