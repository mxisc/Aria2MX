<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { SlidersHorizontal } from 'lucide-vue-next'
import { api } from '@/api'
import type { Aria2OptionMap } from '@/types'

type OptionCategory = {
  id: string
  name: string
  keys: string[]
}

type OptionCopy = {
  name: string
  description?: string
  readonly?: boolean
  choices?: string[]
}

const options = ref<Aria2OptionMap>({})
const baseline = ref<Aria2OptionMap>({})
const loading = ref(false)
const message = ref('')
const errorDialog = ref('')
const activeCategory = ref('basic')
const search = ref('')

const multilineOptionKeys = new Set(['header', 'bt-tracker', 'bt-exclude-tracker', 'no-proxy'])
const commaSeparatedOptionKeys = new Set(['bt-tracker', 'bt-exclude-tracker', 'no-proxy'])

const categories: OptionCategory[] = [
  {
    id: 'basic',
    name: '基本设置',
    keys: ['dir', 'log', 'max-concurrent-downloads', 'check-integrity', 'continue'],
  },
  {
    id: 'httpFtpSFtp',
    name: 'HTTP/FTP/SFTP',
    keys: ['all-proxy', 'all-proxy-user', 'all-proxy-passwd', 'connect-timeout', 'dry-run', 'lowest-speed-limit', 'max-connection-per-server', 'max-file-not-found', 'max-tries', 'min-split-size', 'netrc-path', 'no-netrc', 'no-proxy', 'proxy-method', 'remote-time', 'reuse-uri', 'retry-wait', 'server-stat-of', 'server-stat-timeout', 'split', 'stream-piece-selector', 'timeout', 'uri-selector'],
  },
  {
    id: 'http',
    name: 'HTTP',
    keys: ['check-certificate', 'http-accept-gzip', 'http-auth-challenge', 'http-no-cache', 'http-user', 'http-passwd', 'http-proxy', 'http-proxy-user', 'http-proxy-passwd', 'https-proxy', 'https-proxy-user', 'https-proxy-passwd', 'referer', 'enable-http-keep-alive', 'enable-http-pipelining', 'header', 'save-cookies', 'use-head', 'user-agent'],
  },
  {
    id: 'ftpSFtp',
    name: 'FTP/SFTP',
    keys: ['ftp-user', 'ftp-passwd', 'ftp-pasv', 'ftp-proxy', 'ftp-proxy-user', 'ftp-proxy-passwd', 'ftp-type', 'ftp-reuse-connection', 'ssh-host-key-md'],
  },
  {
    id: 'bt',
    name: 'BitTorrent',
    keys: ['bt-detach-seed-only', 'bt-enable-hook-after-hash-check', 'bt-enable-lpd', 'bt-exclude-tracker', 'bt-external-ip', 'bt-force-encryption', 'bt-hash-check-seed', 'bt-load-saved-metadata', 'bt-max-open-files', 'bt-max-peers', 'bt-metadata-only', 'bt-min-crypto-level', 'bt-prioritize-piece', 'bt-remove-unselected-file', 'bt-require-crypto', 'bt-request-peer-speed-limit', 'bt-save-metadata', 'bt-seed-unverified', 'bt-stop-timeout', 'bt-tracker', 'bt-tracker-connect-timeout', 'bt-tracker-interval', 'bt-tracker-timeout', 'dht-file-path', 'dht-file-path6', 'dht-listen-port', 'dht-message-timeout', 'enable-dht', 'enable-dht6', 'enable-peer-exchange', 'follow-torrent', 'listen-port', 'max-overall-upload-limit', 'max-upload-limit', 'peer-id-prefix', 'peer-agent', 'seed-ratio', 'seed-time'],
  },
  {
    id: 'metalink',
    name: 'Metalink',
    keys: ['follow-metalink', 'metalink-base-uri', 'metalink-language', 'metalink-location', 'metalink-os', 'metalink-version', 'metalink-preferred-protocol', 'metalink-enable-unique-protocol'],
  },
  {
    id: 'rpc',
    name: 'RPC',
    keys: ['enable-rpc', 'pause-metadata', 'rpc-allow-origin-all', 'rpc-listen-all', 'rpc-listen-port', 'rpc-max-request-size', 'rpc-save-upload-metadata', 'rpc-secure'],
  },
  {
    id: 'advanced',
    name: '高级设置',
    keys: ['allow-overwrite', 'allow-piece-length-change', 'always-resume', 'async-dns', 'auto-file-renaming', 'auto-save-interval', 'conditional-get', 'conf-path', 'console-log-level', 'content-disposition-default-utf8', 'daemon', 'deferred-input', 'disable-ipv6', 'disk-cache', 'download-result', 'dscp', 'rlimit-nofile', 'enable-color', 'enable-mmap', 'event-poll', 'file-allocation', 'force-save', 'save-not-found', 'hash-check-only', 'human-readable', 'keep-unfinished-download-result', 'max-download-result', 'max-mmap-limit', 'max-resume-failure-tries', 'min-tls-version', 'log-level', 'optimize-concurrent-downloads', 'piece-length', 'show-console-readout', 'summary-interval', 'max-overall-download-limit', 'max-download-limit', 'no-conf', 'no-file-allocation-limit', 'parameterized-uri', 'quiet', 'realtime-chunk-checksum', 'remove-control-file', 'save-session', 'save-session-interval', 'socket-recv-buffer-size', 'stop', 'truncate-console-readout'],
  },
]

const copy: Record<string, OptionCopy> = {
  'dir': { name: '下载路径' },
  'log': { name: '日志文件', description: '日志文件路径。为空时不写入磁盘。' },
  'max-concurrent-downloads': { name: '最大同时下载数' },
  'check-integrity': { name: '检查完整性', choices: ['true', 'false'], description: '通过分块或完整哈希校验文件。主要用于 BT、Metalink 或带 checksum 的 HTTP(S)/FTP。' },
  'continue': { name: '断点续传', choices: ['true', 'false'], description: '继续下载部分完成的文件。' },
  'all-proxy': { name: '代理服务器' },
  'all-proxy-user': { name: '代理服务器用户名' },
  'all-proxy-passwd': { name: '代理服务器密码' },
  'connect-timeout': { name: '连接超时时间', description: '建立 HTTP/FTP/代理连接的超时时间，单位秒。' },
  'dry-run': { name: '模拟运行', choices: ['true', 'false'] },
  'lowest-speed-limit': { name: '最小速度限制', description: '下载速度低于该值时关闭连接，0 表示不限制。' },
  'max-connection-per-server': { name: '单服务器最大连接数' },
  'max-file-not-found': { name: '文件未找到重试次数' },
  'max-tries': { name: '最大尝试次数' },
  'min-split-size': { name: '最小文件分片大小' },
  'netrc-path': { name: 'Netrc 路径', readonly: true },
  'no-netrc': { name: '禁用 netrc', choices: ['true', 'false'] },
  'no-proxy': { name: '不使用代理的地址' },
  'proxy-method': { name: '代理请求方法', choices: ['get', 'tunnel'] },
  'remote-time': { name: '使用服务器时间', choices: ['true', 'false'] },
  'reuse-uri': { name: '复用 URI', choices: ['true', 'false'] },
  'retry-wait': { name: '重试等待时间' },
  'server-stat-of': { name: '服务器状态保存文件' },
  'server-stat-timeout': { name: '服务器状态超时时间', readonly: true },
  'split': { name: '分片数' },
  'stream-piece-selector': { name: '流式分片选择算法', choices: ['default', 'inorder', 'random', 'geom'] },
  'timeout': { name: '超时时间' },
  'uri-selector': { name: 'URI 选择算法', choices: ['inorder', 'feedback', 'adaptive'] },
  'check-certificate': { name: '检查证书', choices: ['true', 'false'], readonly: true },
  'http-accept-gzip': { name: '接受 GZip', choices: ['true', 'false'] },
  'http-auth-challenge': { name: 'HTTP 认证质询', choices: ['true', 'false'] },
  'http-no-cache': { name: '禁用 HTTP 缓存', choices: ['true', 'false'] },
  'http-user': { name: 'HTTP 用户名' },
  'http-passwd': { name: 'HTTP 密码' },
  'http-proxy': { name: 'HTTP 代理' },
  'http-proxy-user': { name: 'HTTP 代理用户名' },
  'http-proxy-passwd': { name: 'HTTP 代理密码' },
  'https-proxy': { name: 'HTTPS 代理' },
  'https-proxy-user': { name: 'HTTPS 代理用户名' },
  'https-proxy-passwd': { name: 'HTTPS 代理密码' },
  'referer': { name: 'Referer' },
  'enable-http-keep-alive': { name: 'HTTP Keep-Alive', choices: ['true', 'false'] },
  'enable-http-pipelining': { name: 'HTTP 管线化', choices: ['true', 'false'] },
  'header': { name: '自定义请求头', description: '每行一个 Header。' },
  'save-cookies': { name: '保存 Cookie 文件' },
  'use-head': { name: '使用 HEAD 请求', choices: ['true', 'false'] },
  'user-agent': { name: 'User-Agent' },
  'ftp-user': { name: 'FTP 用户名' },
  'ftp-passwd': { name: 'FTP 密码' },
  'ftp-pasv': { name: 'FTP 被动模式', choices: ['true', 'false'] },
  'ftp-proxy': { name: 'FTP 代理' },
  'ftp-proxy-user': { name: 'FTP 代理用户名' },
  'ftp-proxy-passwd': { name: 'FTP 代理密码' },
  'ftp-type': { name: 'FTP 传输类型', choices: ['binary', 'ascii'] },
  'ftp-reuse-connection': { name: '复用 FTP 连接', choices: ['true', 'false'] },
  'ssh-host-key-md': { name: 'SSH 主机公钥指纹' },
  'bt-enable-hook-after-hash-check': { name: '哈希检查后触发 Hook', choices: ['true', 'false'] },
  'bt-enable-lpd': { name: '启用本地节点发现', choices: ['true', 'false'] },
  'bt-exclude-tracker': { name: '排除 BT Tracker' },
  'bt-external-ip': { name: '外部 IP' },
  'bt-force-encryption': { name: '强制加密', choices: ['true', 'false'] },
  'bt-hash-check-seed': { name: '做种前哈希检查', choices: ['true', 'false'] },
  'bt-load-saved-metadata': { name: '加载已保存元数据', choices: ['true', 'false'] },
  'bt-max-open-files': { name: 'BT 最大打开文件数' },
  'bt-max-peers': { name: '最大连接节点数' },
  'bt-metadata-only': { name: '仅下载元数据', choices: ['true', 'false'] },
  'bt-min-crypto-level': { name: '最低加密级别', choices: ['plain', 'arc4'] },
  'bt-prioritize-piece': { name: '优先下载分片' },
  'bt-remove-unselected-file': { name: '删除未选择文件', choices: ['true', 'false'] },
  'bt-require-crypto': { name: '需要加密', choices: ['true', 'false'] },
  'bt-request-peer-speed-limit': { name: '请求节点速度阈值' },
  'bt-save-metadata': { name: '保存磁链元数据', choices: ['true', 'false'] },
  'bt-seed-unverified': { name: '未校验也做种', choices: ['true', 'false'] },
  'bt-stop-timeout': { name: 'BT 停止超时' },
  'bt-tracker': { name: 'BT Tracker 服务器' },
  'bt-tracker-connect-timeout': { name: 'Tracker 连接超时' },
  'bt-tracker-interval': { name: 'Tracker 请求间隔' },
  'bt-tracker-timeout': { name: 'Tracker 超时' },
  'dht-file-path': { name: 'DHT IPv4 文件', readonly: true },
  'dht-file-path6': { name: 'DHT IPv6 文件', readonly: true },
  'dht-listen-port': { name: 'DHT 监听端口', readonly: true },
  'dht-message-timeout': { name: 'DHT 消息超时', readonly: true },
  'enable-dht': { name: '启用 DHT IPv4', choices: ['true', 'false'], readonly: true },
  'enable-dht6': { name: '启用 DHT IPv6', choices: ['true', 'false'], readonly: true },
  'enable-peer-exchange': { name: '启用节点交换', choices: ['true', 'false'] },
  'follow-torrent': { name: '下载种子中的文件', choices: ['true', 'false', 'mem'] },
  'listen-port': { name: 'BT 监听端口', readonly: true },
  'max-overall-upload-limit': { name: '全局最大上传速度' },
  'max-upload-limit': { name: '任务最大上传速度' },
  'peer-id-prefix': { name: '节点 ID 前缀', readonly: true },
  'peer-agent': { name: 'Peer Agent', readonly: true },
  'seed-ratio': { name: '最小分享率' },
  'seed-time': { name: '最小做种时间' },
  'follow-metalink': { name: '下载 Metalink 中的文件', choices: ['true', 'false', 'mem'] },
  'metalink-base-uri': { name: '基础 URI' },
  'metalink-language': { name: '语言' },
  'metalink-location': { name: '首选服务器位置' },
  'metalink-os': { name: '操作系统' },
  'metalink-version': { name: '版本号' },
  'metalink-preferred-protocol': { name: '首选协议', choices: ['http', 'https', 'ftp', 'none'] },
  'metalink-enable-unique-protocol': { name: '仅使用唯一协议', choices: ['true', 'false'] },
  'enable-rpc': { name: '启用 RPC', choices: ['true', 'false'], readonly: true },
  'pause-metadata': { name: '种子文件下载完后暂停', choices: ['true', 'false'] },
  'rpc-allow-origin-all': { name: '接受所有远程请求', choices: ['true', 'false'], readonly: true },
  'rpc-listen-all': { name: '在所有网卡上监听', choices: ['true', 'false'], readonly: true },
  'rpc-listen-port': { name: 'RPC 监听端口', readonly: true },
  'rpc-max-request-size': { name: 'RPC 最大请求大小', readonly: true },
  'rpc-save-upload-metadata': { name: '保存上传元数据', choices: ['true', 'false'] },
  'rpc-secure': { name: '启用 SSL/TLS', choices: ['true', 'false'], readonly: true },
  'allow-overwrite': { name: '允许覆盖', choices: ['true', 'false'] },
  'allow-piece-length-change': { name: '允许分片大小变化', choices: ['true', 'false'] },
  'always-resume': { name: '始终断点续传', choices: ['true', 'false'] },
  'async-dns': { name: '异步 DNS', choices: ['true', 'false'] },
  'auto-file-renaming': { name: '文件自动重命名', choices: ['true', 'false'] },
  'auto-save-interval': { name: '自动保存间隔', readonly: true },
  'conditional-get': { name: '条件下载', choices: ['true', 'false'] },
  'conf-path': { name: '配置文件路径', readonly: true },
  'console-log-level': { name: '控制台日志级别', choices: ['debug', 'info', 'notice', 'warn', 'error'], readonly: true },
  'content-disposition-default-utf8': { name: '使用 UTF-8 处理 Content-Disposition', choices: ['true', 'false'] },
  'daemon': { name: '启用后台进程', choices: ['true', 'false'], readonly: true },
  'deferred-input': { name: '延迟加载', choices: ['true', 'false'], readonly: true },
  'disable-ipv6': { name: '禁用 IPv6', choices: ['true', 'false'], readonly: true },
  'disk-cache': { name: '磁盘缓存', readonly: true },
  'download-result': { name: '下载结果', choices: ['default', 'full', 'hide'] },
  'dscp': { name: 'DSCP', readonly: true },
  'rlimit-nofile': { name: '最多打开的文件描述符', readonly: true },
  'enable-color': { name: '终端输出使用颜色', choices: ['true', 'false'], readonly: true },
  'enable-mmap': { name: '启用 MMap', choices: ['true', 'false'] },
  'event-poll': { name: '事件轮询方法', choices: ['epoll', 'kqueue', 'port', 'poll', 'select'], readonly: true },
  'file-allocation': { name: '文件分配方法', choices: ['none', 'prealloc', 'trunc', 'falloc'] },
  'force-save': { name: '强制保存', choices: ['true', 'false'] },
  'save-not-found': { name: '保存未找到的文件', choices: ['true', 'false'] },
  'hash-check-only': { name: '仅哈希检查', choices: ['true', 'false'] },
  'human-readable': { name: '控制台可读输出', choices: ['true', 'false'], readonly: true },
  'keep-unfinished-download-result': { name: '保留未完成任务', choices: ['true', 'false'] },
  'max-download-result': { name: '最多下载结果' },
  'max-mmap-limit': { name: 'MMap 最大限制' },
  'max-resume-failure-tries': { name: '最大断点续传尝试次数' },
  'min-tls-version': { name: '最低 TLS 版本', choices: ['SSLv3', 'TLSv1', 'TLSv1.1', 'TLSv1.2'], readonly: true },
  'log-level': { name: '日志级别', choices: ['debug', 'info', 'notice', 'warn', 'error'] },
  'optimize-concurrent-downloads': { name: '优化并发下载' },
  'piece-length': { name: '文件分片大小' },
  'show-console-readout': { name: '显示控制台输出', choices: ['true', 'false'], readonly: true },
  'summary-interval': { name: '下载摘要输出间隔', readonly: true },
  'max-overall-download-limit': { name: '全局最大下载速度' },
  'max-download-limit': { name: '任务最大下载速度' },
  'no-conf': { name: '禁用配置文件', choices: ['true', 'false'], readonly: true },
  'no-file-allocation-limit': { name: '文件分配限制' },
  'parameterized-uri': { name: '启用参数化 URI 支持', choices: ['true', 'false'] },
  'quiet': { name: '禁用控制台输出', choices: ['true', 'false'], readonly: true },
  'realtime-chunk-checksum': { name: '实时数据块验证', choices: ['true', 'false'] },
  'remove-control-file': { name: '删除控制文件', choices: ['true', 'false'] },
  'save-session': { name: '状态保存文件' },
  'save-session-interval': { name: '保存状态间隔', readonly: true },
  'socket-recv-buffer-size': { name: 'Socket 接收缓冲区大小', readonly: true },
  'stop': { name: '自动关闭时间', readonly: true },
  'truncate-console-readout': { name: '缩短控制台输出内容', choices: ['true', 'false'], readonly: true },
}

const currentCategory = computed(() => categories.find((category) => category.id === activeCategory.value) || categories[0])

const visibleKeys = computed(() => {
  const keyword = search.value.trim().toLowerCase()
  return currentCategory.value.keys.filter((key) => {
    if (!keyword) return true
    const item = copy[key]
    return key.includes(keyword) || item?.name.toLowerCase().includes(keyword) || item?.description?.toLowerCase().includes(keyword)
  })
})

onMounted(load)

async function load() {
  loading.value = true
  message.value = ''
  try {
    const loaded = await api.aria2<Aria2OptionMap>('aria2.getGlobalOption')
    options.value = normalizeOptionsForEditor(loaded)
    baseline.value = { ...options.value }
  } catch (error) {
    showError(error instanceof Error ? error.message : '全局选项读取失败。')
  } finally {
    loading.value = false
  }
}

async function save() {
  const patch: Record<string, string> = {}
  const allKeys = new Set([...Object.keys(baseline.value), ...Object.keys(options.value)])
  for (const key of allKeys) {
    const nextValue = options.value[key] ?? ''
    const prevValue = baseline.value[key] ?? ''
    if (nextValue !== prevValue) patch[key] = nextValue
  }
  if (Object.keys(patch).length === 0) {
    message.value = '没有需要保存的变化。'
    return
  }
  loading.value = true
  message.value = ''
  try {
    const result = await api.saveManagedAria2Options(patch)
    message.value = result.message
    await load()
  } catch (error) {
    showError(error instanceof Error ? error.message : '全局选项保存失败。')
  } finally {
    loading.value = false
  }
}

async function reset() {
  loading.value = true
  message.value = ''
  try {
    const result = await api.resetManagedAria2Options()
    message.value = result.message
    await load()
  } catch (error) {
    showError(error instanceof Error ? error.message : '全局选项重置失败。')
  } finally {
    loading.value = false
  }
}

function showError(text: string) {
  errorDialog.value = text
}

function closeErrorDialog() {
  errorDialog.value = ''
}

function optionValue(key: string) {
  return options.value[key] ?? ''
}

function updateOption(key: string, event: Event) {
  const target = event.target as HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement
  options.value[key] = target.value
}

function optionTitle(key: string) {
  return copy[key]?.name || key
}

function optionDescription(key: string) {
  return copy[key]?.description || '来自 aria2 全局选项。'
}

function optionControlKind(key: string) {
  if (copy[key]?.choices?.length) return 'select'
  if (multilineOptionKeys.has(key)) return 'textarea'
  return 'input'
}

function optionTypeLabel(key: string) {
  if (copy[key]?.choices?.length) return '枚举'
  if (optionControlKind(key) === 'textarea') return '多行'
  return '文本'
}

function optionDisplayValue(key: string) {
  const value = optionValue(key)
  return value || '默认'
}

function normalizeOptionsForEditor(source: Aria2OptionMap) {
  const normalized: Aria2OptionMap = {}
  for (const [key, value] of Object.entries(source)) {
    normalized[key] = normalizeOptionValueForEditor(key, value)
  }
  return normalized
}

function normalizeOptionValueForEditor(key: string, value: string) {
  if (!commaSeparatedOptionKeys.has(key)) return value ?? ''
  return (value ?? '')
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean)
    .join('\n')
}
</script>

<template>
  <section class="panel options-panel">
    <div class="settings-layout">
      <aside class="settings-nav">
        <button
          v-for="category in categories"
          :key="category.id"
          :class="{ active: activeCategory === category.id }"
          @click="activeCategory = category.id"
        >
          <span>{{ category.name }}</span>
          <small>{{ category.keys.length }}</small>
        </button>
      </aside>

      <div class="settings-content">
        <div class="settings-content-head">
          <div class="section-title">
            <SlidersHorizontal :size="17" />
            <span>aria2 全局选项</span>
          </div>
          <label class="search-box">
            <input v-model="search" placeholder="搜索 aria2 选项或中文文案">
          </label>
        </div>

        <div class="option-rows">
          <div
            v-for="key in visibleKeys"
            :key="key"
            class="option-row"
            :class="{ multiline: optionControlKind(key) === 'textarea' }"
          >
            <div class="option-main">
              <b>{{ optionTitle(key) }}</b>
              <small v-if="optionDescription(key)">{{ optionDescription(key) }}</small>
              <code>{{ key }}</code>
            </div>
            <div class="option-meta">
              <span class="type">{{ optionTypeLabel(key) }}</span>
            </div>
            <div class="option-editor-cell">
              <select
                v-if="optionControlKind(key) === 'select'"
                :value="optionValue(key)"
                :disabled="loading"
                @change="updateOption(key, $event)"
              >
                <option v-if="!optionValue(key)" value="">
                  默认
                </option>
                <option v-for="choice in copy[key]?.choices || []" :key="choice" :value="choice">
                  {{ choice }}
                </option>
              </select>
              <textarea
                v-else-if="optionControlKind(key) === 'textarea'"
                :value="optionValue(key)"
                :disabled="loading"
                spellcheck="false"
                @input="updateOption(key, $event)"
              />
              <input
                v-else
                :value="optionValue(key)"
                :disabled="loading"
                @input="updateOption(key, $event)"
              >
            </div>
          </div>
        </div>

        <div class="button-row option-actions">
          <button class="primary" :disabled="loading" @click="save">
            保存
          </button>
          <button class="ghost" :disabled="loading" @click="load">
            刷新
          </button>
          <button class="ghost" :disabled="loading" @click="reset">
            重置默认值
          </button>
        </div>
      </div>
    </div>
    <p v-if="message" class="hint">
      {{ message }}
    </p>
  </section>
  <Teleport to="body">
    <div v-if="errorDialog" class="center-dialog-backdrop" @click="closeErrorDialog">
      <div class="center-dialog" role="alertdialog" aria-modal="true" aria-label="错误提示" @click.stop>
        <strong>操作失败</strong>
        <p>{{ errorDialog }}</p>
        <div class="button-row">
          <button class="primary" @click="closeErrorDialog">
            知道了
          </button>
        </div>
      </div>
    </div>
  </Teleport>
</template>
