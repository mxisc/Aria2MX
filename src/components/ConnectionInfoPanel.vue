<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { Info } from 'lucide-vue-next'
import { api } from '@/api'
import type { AppAbout } from '@/types'

const loading = ref(false)
const error = ref('')
const about = ref<AppAbout>({
  panelVersion: '-',
  aria2Version: '-',
  rpcPath: '/jsonrpc',
  httpRpcUrl: '/jsonrpc',
  wsRpcUrl: '/jsonrpc',
  mcpHttpUrl: '/mcp',
  mcpEnabled: true,
  panelRpcSecret: '-',
})

const copyMessage = ref('')

const overviewItems = computed(() => {
  return [
    { label: '面板版本', value: about.value.panelVersion || '-' },
    { label: 'aria2 版本', value: about.value.aria2Version || '-' },
    { label: '面板 Secret', value: about.value.panelRpcSecret || '-' },
    { label: 'MCP', value: about.value.mcpEnabled ? '已开启' : '已关闭' },
  ]
})

const httpExample = computed(() => {
  return `curl '${about.value.httpRpcUrl}' \\
  -H 'Content-Type: application/json' \\
  --data-raw '{"jsonrpc":"2.0","id":"1","method":"aria2.getVersion","params":["token:${about.value.panelRpcSecret || '-'}"]}'`
})

const wsPayload = computed(() => {
  return `{"jsonrpc":"2.0","id":"1","method":"aria2.getVersion","params":["token:${about.value.panelRpcSecret || '-'}"]}`
})

const mcpUrl = computed(() => {
  if (!about.value.mcpEnabled || !about.value.mcpHttpUrl) {
    return '当前已关闭'
  }
  return `${about.value.mcpHttpUrl}?secret=${about.value.panelRpcSecret || '-'}`
})

const mcpInitializeExample = computed(() => {
  if (!about.value.mcpEnabled || !about.value.mcpHttpUrl) {
    return '当前已关闭'
  }
  return `curl '${mcpUrl.value}' \\
  -H 'Content-Type: application/json' \\
  --data-raw '{"jsonrpc":"2.0","id":"init","method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"custom-client","version":"1.0.0"}}}'`
})

onMounted(load)

async function load() {
  loading.value = true
  error.value = ''
  try {
    about.value = await api.getAbout()
  } catch (caught) {
    error.value = caught instanceof Error ? caught.message : '连接信息读取失败。'
  } finally {
    loading.value = false
  }
}

async function copyText(value: string) {
  try {
    await navigator.clipboard.writeText(value)
    copyMessage.value = '已复制。'
    window.setTimeout(() => {
      if (copyMessage.value === '已复制。') {
        copyMessage.value = ''
      }
    }, 1500)
  } catch {
    copyMessage.value = '复制失败。'
  }
}

const connectionSections = computed(() => {
  return [
    {
      key: 'http',
      title: 'HTTP RPC',
      status: 'aria2 / AriaNg 原生 token 写法',
      enabled: true,
      lines: [
        { label: '地址', value: about.value.httpRpcUrl, copy: about.value.httpRpcUrl, multiline: true },
        { label: '鉴权', value: `token:${about.value.panelRpcSecret || '-'}`, copy: `token:${about.value.panelRpcSecret || '-'}` },
      ],
      exampleLabel: '请求',
      exampleValue: httpExample.value,
      exampleCopy: httpExample.value,
    },
    {
      key: 'ws',
      title: 'WebSocket RPC',
      status: '地址和报文分开使用',
      enabled: true,
      lines: [
        { label: '地址', value: about.value.wsRpcUrl, copy: about.value.wsRpcUrl, multiline: true },
        { label: '报文鉴权', value: `token:${about.value.panelRpcSecret || '-'}`, copy: `token:${about.value.panelRpcSecret || '-'}` },
      ],
      exampleLabel: '报文',
      exampleValue: wsPayload.value,
      exampleCopy: wsPayload.value,
    },
    {
      key: 'mcp',
      title: 'MCP',
      status: about.value.mcpEnabled ? '使用面板 Secret 接入' : '当前已关闭',
      enabled: about.value.mcpEnabled,
      lines: [
        { label: '地址', value: mcpUrl.value, copy: mcpUrl.value, multiline: true },
        { label: 'Secret', value: about.value.mcpEnabled ? about.value.panelRpcSecret || '-' : '当前已关闭', copy: about.value.panelRpcSecret || '-' },
      ],
      exampleLabel: '初始化',
      exampleValue: mcpInitializeExample.value,
      exampleCopy: mcpInitializeExample.value,
    },
  ]
})
</script>

<template>
  <section class="panel connection-info-panel">
    <div class="section-title">
      <Info :size="17" />
      <span>连接信息</span>
    </div>
    <div v-if="error" class="banner">
      {{ error }}
    </div>
    <div v-else class="connection-layout">
      <section class="connection-overview-grid">
        <article v-for="item in overviewItems" :key="item.label" class="connection-overview-card">
          <span>{{ item.label }}</span>
          <strong>{{ loading ? '读取中...' : item.value }}</strong>
        </article>
      </section>
      <section class="connection-section-list">
        <article v-for="section in connectionSections" :key="section.key" class="connection-section-card">
          <div class="connection-section-head">
            <div>
              <h3>{{ section.title }}</h3>
              <p>{{ loading ? '读取中...' : section.status }}</p>
            </div>
          </div>
          <div class="connection-line-list">
            <div v-for="line in section.lines" :key="`${section.key}-${line.label}`" class="connection-line" :class="{ 'connection-line-multiline': line.multiline }">
              <span>{{ line.label }}</span>
              <strong>{{ loading ? '读取中...' : line.value }}</strong>
              <button class="ghost" type="button" :disabled="!section.enabled || loading" @click="copyText(line.copy)">
                复制
              </button>
            </div>
          </div>
          <div class="connection-example-block">
            <div class="connection-example-head">
              <span>{{ section.exampleLabel }}</span>
              <button class="ghost" type="button" :disabled="!section.enabled || loading" @click="copyText(section.exampleCopy)">
                复制
              </button>
            </div>
            <pre>{{ loading ? '读取中...' : section.exampleValue }}</pre>
          </div>
        </article>
      </section>
    </div>
    <p v-if="copyMessage" class="hint">
      {{ copyMessage }}
    </p>
  </section>
</template>
