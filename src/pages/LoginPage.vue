<script setup lang="ts">
import { ref } from 'vue'
import { LockKeyhole } from 'lucide-vue-next'
import { api } from '@/api'

const emit = defineEmits<{ authenticated: [] }>()
const username = ref('admin')
const password = ref('')
const loading = ref(false)
const message = ref('')

async function login() {
  loading.value = true
  message.value = ''
  try {
    await api.login(username.value, password.value)
    emit('authenticated')
  } catch (error) {
    message.value = error instanceof Error ? error.message : '登录失败，请重试。'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <main class="login-page">
    <section class="login-card">
      <div class="brand-mark">
        <LockKeyhole :size="22" />
      </div>
      <p class="eyebrow">
        ARIA2 CONTROL SURFACE
      </p>
      <h1>AriaMX</h1>
      <p class="login-copy">
        用一个轻量面板接管 aria2，RPC Secret 保持在服务端。
      </p>
      <form @submit.prevent="login">
        <label>
          <span>用户名</span>
          <input v-model="username" autocomplete="username">
        </label>
        <label>
          <span>密码</span>
          <input v-model="password" type="password" autocomplete="current-password" autofocus>
        </label>
        <button class="primary wide" :disabled="loading">
          {{ loading ? '登录中' : '登录' }}
        </button>
      </form>
      <p v-if="message" class="hint danger">
        {{ message }}
      </p>
      <p class="tiny">
        首次启动默认账号为 admin，密码来自 ARIAMX_ADMIN_PASSWORD，未设置时为 admin。
      </p>
    </section>
  </main>
</template>
