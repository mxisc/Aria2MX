<script setup lang="ts">
import { ref } from 'vue'
import { Link2, Upload } from 'lucide-vue-next'
import { api } from '@/api'

const emit = defineEmits<{ created: [] }>()

const links = ref('')
const dir = ref('')
const out = ref('')
const split = ref(16)
const maxConnection = ref(8)
const downloadLimit = ref('')
const seedRatio = ref('')
const seedTime = ref('')
const headers = ref('')
const paused = ref(false)
const loading = ref(false)
const message = ref('')
const torrentInput = ref<HTMLInputElement | null>(null)

async function submitLinks() {
  const urls = links.value.split('\n').map((item) => item.trim()).filter(Boolean)
  if (urls.length === 0) {
    message.value = '请先输入 URL 或磁力链接。'
    return
  }
  loading.value = true
  message.value = ''
  try {
    for (const url of urls) {
      const options: Record<string, string> = {}
      if (dir.value) options.dir = dir.value
      if (out.value) options.out = out.value
      if (split.value) options.split = String(split.value)
      if (maxConnection.value) options['max-connection-per-server'] = String(maxConnection.value)
      if (downloadLimit.value) options['max-download-limit'] = downloadLimit.value
      if (seedRatio.value) options['seed-ratio'] = seedRatio.value
      if (seedTime.value) options['seed-time'] = seedTime.value
      if (paused.value) options.pause = 'true'
      const headerLines = headers.value.split('\n').map((item) => item.trim()).filter(Boolean)
      if (headerLines.length > 0) options.header = headerLines.join('\n')
      await api.aria2<string>('aria2.addUri', [[url], options])
    }
    links.value = ''
    message.value = '任务已提交。'
    emit('created')
  } catch (error) {
    message.value = error instanceof Error ? error.message : '任务创建失败。'
  } finally {
    loading.value = false
  }
}

async function uploadTorrent(event: Event) {
  const file = (event.target as HTMLInputElement).files?.[0]
  if (!file) return
  loading.value = true
  message.value = ''
  try {
    await api.uploadTorrent(file)
    message.value = '种子任务已提交。'
    emit('created')
  } catch (error) {
    message.value = error instanceof Error ? error.message : '种子上传失败。'
  } finally {
    loading.value = false
    if (torrentInput.value) torrentInput.value.value = ''
  }
}
</script>

<template>
  <section class="panel add-panel">
    <div class="section-title">
      <Link2 :size="17" />
      <span>新建任务</span>
    </div>
    <textarea v-model="links" placeholder="每行一个 URL 或 magnet 链接" />
    <div class="form-grid">
      <label>
        <span>下载目录</span>
        <input v-model="dir" placeholder="留空使用 aria2 默认值">
      </label>
      <label>
        <span>输出文件名</span>
        <input v-model="out" placeholder="单链接任务可用">
      </label>
      <label>
        <span>分片数</span>
        <input v-model.number="split" type="number" min="1" max="64">
      </label>
      <label>
        <span>单服务器连接数</span>
        <input v-model.number="maxConnection" type="number" min="1" max="64">
      </label>
      <label>
        <span>任务限速</span>
        <input v-model="downloadLimit" placeholder="例如 2M 或 512K">
      </label>
      <label>
        <span>做种分享率</span>
        <input v-model="seedRatio" placeholder="BT 任务，例如 1.0">
      </label>
      <label>
        <span>做种时间</span>
        <input v-model="seedTime" placeholder="分钟，BT 任务">
      </label>
      <label class="check-line">
        <input v-model="paused" type="checkbox">
        <span>创建后暂停</span>
      </label>
    </div>
    <label>
      <span>请求 Header</span>
      <textarea v-model="headers" class="small-textarea" placeholder="每行一个 Header，例如：Cookie: a=b" />
    </label>
    <div class="button-row">
      <button class="primary" :disabled="loading" @click="submitLinks">
        提交链接
      </button>
      <label class="ghost upload-button">
        <Upload :size="15" />
        上传种子
        <input ref="torrentInput" type="file" accept=".torrent" @change="uploadTorrent">
      </label>
    </div>
    <p v-if="message" class="hint">
      {{ message }}
    </p>
  </section>
</template>
