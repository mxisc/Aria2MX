<script setup lang="ts">
defineProps<{
  label: string
  value: string
  tone?: 'accent' | 'muted' | 'warn'
}>()
</script>

<template>
  <section class="metric-card" :class="tone || 'muted'">
    <span>{{ label }}</span>
    <strong>{{ value }}</strong>
  </section>
</template>
