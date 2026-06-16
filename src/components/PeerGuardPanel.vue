<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { Shield } from 'lucide-vue-next'
import { api } from '@/api'
import type { PeerGuardSnapshot } from '@/types'
import { speed } from '@/utils/format'

const loading = ref(false)
const message = ref('')
const manualIP = ref('')
const manualReason = ref('')
const data = ref<PeerGuardSnapshot>({
  firewallMode: 'pf',
  firewallReady: true,
  firewallOperable: true,
  autoBanEnabled: false,
  autoBanMinScore: 3,
  blockedPeers: [],
  suspiciousPeers: [],
})

const suspiciousPeers = computed(() => data.value.suspiciousPeers.slice().sort((left, right) => right.score - left.score))
const actionLocked = computed(() => !data.value.firewallOperable)

onMounted(load)

async function load() {
  loading.value = true
  message.value = ''
  try {
    data.value = await api.getPeerGuard()
  } catch (error) {
    message.value = error instanceof Error ? error.message : '节点防护信息读取失败。'
  } finally {
    loading.value = false
  }
}

async function ban(ip: string, reason: string) {
  loading.value = true
  message.value = ''
  try {
    data.value = await api.banPeer(ip, reason)
    manualIP.value = ''
    manualReason.value = ''
    message.value = '节点已封禁。'
  } catch (error) {
    message.value = error instanceof Error ? error.message : '节点封禁失败。'
  } finally {
    loading.value = false
  }
}

async function unban(ip: string) {
  loading.value = true
  message.value = ''
  try {
    data.value = await api.unbanPeer(ip)
    message.value = '节点已移出封禁名单。'
  } catch (error) {
    message.value = error instanceof Error ? error.message : '解除封禁失败。'
  } finally {
    loading.value = false
  }
}

async function banManual() {
  if (!manualIP.value.trim()) {
    message.value = '请先输入节点 IP。'
    return
  }
  await ban(manualIP.value.trim(), manualReason.value.trim())
}

async function updateAutoBan(enabled: boolean) {
  loading.value = true
  message.value = ''
  try {
    data.value = await api.updatePeerGuardSettings(enabled)
    message.value = enabled ? `自动封禁已开启，评分达到 ${data.value.autoBanMinScore} 分会自动封禁。` : '自动封禁已关闭。'
  } catch (error) {
    message.value = error instanceof Error ? error.message : '自动封禁设置失败。'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <section class="panel peer-guard-panel">
    <div v-if="actionLocked" class="peer-guard-overlay">
      <strong>当前无法操作节点防护</strong>
      <p>{{ data.actionBlockedReason || '当前进程缺少系统防火墙权限。请修复权限后再操作。' }}</p>
    </div>

    <div class="peer-guard-head">
      <div class="section-title">
        <Shield :size="17" />
        <span>节点防护</span>
      </div>
      <div class="peer-guard-topbar">
        <div class="peer-guard-radio-group" role="radiogroup" aria-label="自动封禁">
          <button
            type="button"
            class="peer-guard-radio"
            :class="{ active: !data.autoBanEnabled }"
            :disabled="loading || actionLocked"
            @click="updateAutoBan(false)"
          >
            手动封禁
          </button>
          <button
            type="button"
            class="peer-guard-radio"
            :class="{ active: data.autoBanEnabled }"
            :disabled="loading || actionLocked"
            @click="updateAutoBan(true)"
          >
            自动封禁
          </button>
        </div>
        <div class="peer-guard-threshold">
          阈值 {{ data.autoBanMinScore }} 分
        </div>
      </div>
    </div>

    <div class="peer-guard-summary">
      <article class="connection-overview-card">
        <span>防火墙模式</span>
        <strong>{{ data.firewallMode }}</strong>
      </article>
      <article class="connection-overview-card">
        <span>已封禁</span>
        <strong>{{ data.blockedPeers.length }}</strong>
      </article>
      <article class="connection-overview-card">
        <span>疑似节点</span>
        <strong>{{ suspiciousPeers.length }}</strong>
      </article>
      <article class="connection-overview-card">
        <span>最近应用</span>
        <strong>{{ data.lastAppliedAt || '未应用' }}</strong>
      </article>
    </div>

    <div v-if="data.lastError" class="banner">
      {{ data.lastError }}
    </div>

    <div class="peer-guard-surface">
      <div class="peer-guard-toolbar">
        <label>
          <span>手动封禁 IP</span>
          <input v-model="manualIP" :disabled="loading || actionLocked" placeholder="例如 203.0.113.10">
        </label>
        <label>
          <span>原因</span>
          <input v-model="manualReason" :disabled="loading || actionLocked" placeholder="可选">
        </label>
        <div class="peer-guard-action-row">
          <button class="primary" :disabled="loading || actionLocked" @click="banManual">
            封禁节点
          </button>
          <button class="ghost" :disabled="loading" @click="load">
            刷新
          </button>
        </div>
      </div>

      <div class="peer-guard-lists">
        <section class="peer-guard-section">
          <div class="section-title">
            <span>高风险节点</span>
          </div>
          <div class="data-table peer-guard-table">
            <div class="table-row head peer-guard-grid">
              <span>任务</span><span>节点</span><span>下载</span><span>上传</span><span>评分</span><span>原因</span><span>操作</span>
            </div>
            <div v-for="peer in suspiciousPeers" :key="`${peer.gid}-${peer.ip}-${peer.port}`" class="table-row peer-guard-grid">
              <span>{{ peer.taskName }}</span>
              <span>{{ peer.ip }}:{{ peer.port }}</span>
              <span>{{ speed(peer.downloadSpeed) }}</span>
              <span>{{ speed(peer.uploadSpeed) }}</span>
              <span>{{ peer.score }} 分</span>
              <span>{{ peer.reason }}</span>
              <span>
                <button v-if="!peer.blocked" class="ghost" :disabled="loading || actionLocked" @click="ban(peer.ip, peer.reason)">
                  封禁
                </button>
                <span v-else class="hint">已封禁</span>
              </span>
            </div>
            <div v-if="!loading && suspiciousPeers.length === 0" class="empty-state">
              当前没有识别到高风险节点。
            </div>
          </div>
        </section>

        <section class="peer-guard-section peer-guard-blocked-section">
          <div class="section-title">
            <span>封禁名单</span>
          </div>
          <div class="data-table peer-guard-table">
            <div class="table-row head peer-guard-blocked-grid">
              <span>IP</span><span>原因</span><span>时间</span><span>操作</span>
            </div>
            <div v-for="peer in data.blockedPeers" :key="peer.ip" class="table-row peer-guard-blocked-grid">
              <span>{{ peer.ip }}</span>
              <span>{{ peer.reason || '-' }}</span>
              <span>{{ peer.createdAt || '-' }}</span>
              <span>
                <button class="ghost" :disabled="loading || actionLocked" @click="unban(peer.ip)">
                  解除
                </button>
              </span>
            </div>
            <div v-if="!loading && data.blockedPeers.length === 0" class="empty-state">
              当前没有封禁节点。
            </div>
          </div>
        </section>
      </div>
    </div>

    <p v-if="message" class="hint">
      {{ message }}
    </p>
  </section>
</template>
