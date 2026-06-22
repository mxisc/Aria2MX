<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { FileCode2 } from 'lucide-vue-next'
import { basicSetup } from 'codemirror'
import { StreamLanguage } from '@codemirror/language'
import { shell } from '@codemirror/legacy-modes/mode/shell'
import { EditorState } from '@codemirror/state'
import { EditorView } from '@codemirror/view'
import { api } from '@/api'
import { currentSkinEnabled } from '@/theme'
import type { ScriptHookItem } from '@/types'

const hooks = ref<ScriptHookItem[]>([])
const activeKey = ref('downloadComplete')
const editorHost = ref<HTMLElement>()
const loading = ref(false)
const message = ref('')
const error = ref('')
let editorView: EditorView | undefined
let syncingEditor = false

const activeHook = computed(() => hooks.value.find((hook) => hook.key === activeKey.value) || hooks.value[0])
const skinStyle = computed(() => {
  if (!currentSkinEnabled.value) return {}
  return {
    '--script-tab-bg': 'color-mix(in srgb, var(--panel) 18%, transparent)',
    '--script-tab-border': 'color-mix(in srgb, var(--soft-line) 28%, transparent)',
    '--script-tab-text': 'var(--text)',
    '--script-tab-active-bg': 'color-mix(in srgb, var(--accent) 62%, transparent)',
    '--script-tab-active-border': 'color-mix(in srgb, var(--accent) 82%, var(--soft-line))',
    '--script-tab-active-text': '#fff',
    '--script-editor-bg': 'color-mix(in srgb, var(--field-bg) 18%, transparent)',
    '--script-editor-border': 'color-mix(in srgb, var(--soft-line) 28%, transparent)',
    '--script-editor-gutter-bg': 'color-mix(in srgb, var(--panel) 16%, transparent)',
    '--script-editor-gutter-border': 'color-mix(in srgb, var(--soft-line) 24%, transparent)',
    '--script-editor-active-line-bg': 'color-mix(in srgb, var(--accent) 12%, transparent)',
  }
})

onMounted(load)
onBeforeUnmount(() => {
  editorView?.destroy()
  editorView = undefined
})

watch(activeKey, () => {
  nextTick(syncEditorDocument)
})

watch(() => activeHook.value?.content, () => {
  if (!syncingEditor) {
    nextTick(syncEditorDocument)
  }
})

async function load() {
  loading.value = true
  message.value = ''
  error.value = ''
  try {
    const state = await api.getScriptHooks()
    hooks.value = state.hooks
    if (!hooks.value.some((hook) => hook.key === activeKey.value)) {
      activeKey.value = hooks.value[0]?.key || 'downloadComplete'
    }
    await nextTick()
    ensureEditor()
    syncEditorDocument()
  } catch (caught) {
    error.value = caught instanceof Error ? caught.message : '脚本设置读取失败。'
  } finally {
    loading.value = false
  }
}

async function save() {
  loading.value = true
  message.value = ''
  error.value = ''
  try {
    const state = await api.updateScriptHooks(hooks.value)
    hooks.value = state.hooks
    message.value = state.message || '脚本设置已保存。'
  } catch (caught) {
    error.value = caught instanceof Error ? caught.message : '脚本设置保存失败。'
  } finally {
    loading.value = false
  }
}

function updateActiveHook(patch: Partial<ScriptHookItem>) {
  const key = activeHook.value?.key
  if (!key) return
  hooks.value = hooks.value.map((hook) => hook.key === key ? { ...hook, ...patch } : hook)
}

function ensureEditor() {
  if (editorView || !editorHost.value) return
  editorView = new EditorView({
    parent: editorHost.value,
    state: EditorState.create({
      doc: activeHook.value?.content || '',
      extensions: [
        basicSetup,
        StreamLanguage.define(shell),
        EditorView.lineWrapping,
        EditorView.updateListener.of((update) => {
          if (!update.docChanged) return
          syncingEditor = true
          updateActiveHook({ content: update.state.doc.toString() })
          syncingEditor = false
        }),
      ],
    }),
  })
}

function syncEditorDocument() {
  ensureEditor()
  if (!editorView) return
  const nextContent = activeHook.value?.content || ''
  const currentContent = editorView.state.doc.toString()
  if (nextContent === currentContent) return
  editorView.dispatch({
    changes: { from: 0, to: currentContent.length, insert: nextContent },
  })
}
</script>

<template>
  <section class="panel script-settings-panel" :style="skinStyle">
    <div class="script-head">
      <div class="section-title">
        <FileCode2 :size="17" />
        <span>任务脚本</span>
      </div>
      <div class="button-row">
        <button class="ghost" :disabled="loading" @click="load">
          刷新
        </button>
        <button class="primary" :disabled="loading" @click="save">
          保存
        </button>
      </div>
    </div>

    <div class="script-tabs" role="tablist" aria-label="脚本类型">
      <button
        v-for="hook in hooks"
        :key="hook.key"
        role="tab"
        :aria-selected="activeKey === hook.key"
        :class="{ active: activeKey === hook.key }"
        @click="activeKey = hook.key"
      >
        {{ hook.title }}
      </button>
    </div>

    <template v-if="activeHook">
      <div ref="editorHost" class="script-editor" :class="{ disabled: loading }" />
    </template>

    <p v-if="message" class="hint">
      {{ message }}
    </p>
    <p v-if="error" class="form-error">
      {{ error }}
    </p>
  </section>
</template>

<style scoped>
.script-settings-panel {
  display: flex;
  flex-direction: column;
  gap: 16px;
  height: 100%;
  min-width: 0;
  overflow: hidden;
  width: 100%;
}

.script-head {
  align-items: center;
  display: flex;
  gap: 16px;
  justify-content: space-between;
}

.script-tabs {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.script-tabs button {
  background: var(--script-tab-bg, var(--panel-muted));
  border: 1px solid var(--script-tab-border, var(--line));
  border-radius: 8px;
  color: var(--script-tab-text, var(--muted));
  min-height: 40px;
  padding: 0 12px;
}

.script-tabs button.active {
  background: var(--script-tab-active-bg, var(--accent-soft));
  border-color: var(--script-tab-active-border, color-mix(in srgb, var(--accent) 35%, var(--line)));
  color: var(--script-tab-active-text, var(--accent-text));
}

.script-editor {
  flex: 1 1 auto;
  max-width: 100%;
  min-height: 0;
  min-width: 0;
  overflow: auto;
  width: 100%;
}

.script-editor.disabled {
  opacity: 0.72;
  pointer-events: none;
}

.script-editor :deep(.cm-editor) {
  background: var(--script-editor-bg, var(--field-bg));
  border: 1px solid var(--script-editor-border, var(--line));
  border-radius: 8px;
  color: var(--text);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", monospace;
  height: 100%;
  overflow: hidden;
}

.script-editor :deep(.cm-focused) {
  border-color: var(--accent);
  box-shadow: 0 0 0 4px color-mix(in srgb, var(--accent) 16%, transparent);
  outline: none;
}

.script-editor :deep(.cm-scroller) {
  overflow: auto;
}

.script-editor :deep(.cm-gutters) {
  background: var(--script-editor-gutter-bg, var(--panel-muted));
  border-color: var(--script-editor-gutter-border, var(--line));
  color: var(--muted);
}

.script-editor :deep(.cm-activeLine),
.script-editor :deep(.cm-activeLineGutter) {
  background: var(--script-editor-active-line-bg, color-mix(in srgb, var(--accent) 9%, transparent));
}

.script-editor :deep(.cm-content) {
  min-height: 100%;
  padding: 12px 0;
}

.script-editor :deep(.cm-line) {
  padding: 0 14px;
}

.form-error {
  color: var(--danger);
  margin: 0;
}

@media (max-width: 760px) {
  .script-head {
    align-items: stretch;
    flex-direction: column;
  }
}
</style>
