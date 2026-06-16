<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from './api'
import { applyTheme } from './theme'

const router = useRouter()
const ready = ref(false)
const authenticated = ref(false)

onMounted(async () => {
  try {
    await api.me()
    authenticated.value = true
    await loadTheme()
    if (router.currentRoute.value.path === '/login') await router.replace('/')
  } catch {
    authenticated.value = false
    applyTheme('ariamx', 'light')
    await router.replace('/login')
  } finally {
    ready.value = true
  }
})

async function onAuthenticated() {
  authenticated.value = true
  await loadTheme()
  await router.replace('/')
}

async function onLoggedOut() {
  authenticated.value = false
  applyTheme('ariamx', 'light')
  await router.replace('/login')
}

async function loadTheme() {
  try {
    const config = await api.getConfig()
    applyTheme(config.theme, config.colorMode)
  } catch {
    applyTheme('ariamx', 'light')
  }
}
</script>

<template>
  <div v-if="!ready" class="boot-screen">
    AriaMX 正在启动
  </div>
  <router-view v-else :authenticated="authenticated" @authenticated="onAuthenticated" @logged-out="onLoggedOut" />
</template>
