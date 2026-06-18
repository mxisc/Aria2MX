<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from './api'
import { applySkin, applyTheme } from './theme'

const router = useRouter()
const ready = ref(false)
const authenticated = ref(false)

onMounted(async () => {
  try {
    await api.me()
    authenticated.value = true
    await loadPrivateTheme()
    if (router.currentRoute.value.path === '/login') await router.replace('/')
  } catch {
    authenticated.value = false
    await loadPublicTheme()
    await router.replace('/login')
  } finally {
    ready.value = true
  }
})

async function onAuthenticated() {
  authenticated.value = true
  await loadPrivateTheme()
  await router.replace('/')
}

async function onLoggedOut() {
  authenticated.value = false
  await loadPublicTheme()
  await router.replace('/login')
}

async function loadPrivateTheme() {
  try {
    const config = await api.getConfig()
    applyTheme(config.theme, config.colorMode)
    applySkin(config.skinEnabled, config.skinName, config.skinApiTemplate)
  } catch {
    await loadPublicTheme()
  }
}

async function loadPublicTheme() {
  try {
    const config = await api.getPublicPanelStyle()
    applyTheme(config.theme, config.colorMode)
    applySkin(config.skinEnabled, config.skinName, config.skinApiTemplate)
  } catch {
    applyTheme('ariamx', 'system')
    applySkin(false, 'default', '')
  }
}
</script>

<template>
  <div v-if="!ready" class="boot-screen">
    AriaMX 正在启动
  </div>
  <router-view v-else :authenticated="authenticated" @authenticated="onAuthenticated" @logged-out="onLoggedOut" />
</template>
