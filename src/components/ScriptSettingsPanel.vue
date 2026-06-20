<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { FileCode2 } from 'lucide-vue-next'
import { basicSetup } from 'codemirror'
import { StreamLanguage } from '@codemirror/language'
import { shell } from '@codemirror/legacy-modes/mode/shell'
import { EditorState } from '@codemirror/state'
import { EditorView } from '@codemirror/view'
import { api } from '@/api'
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

const safeTemplate = `#!/usr/bin/env bash
set -euo pipefail

GID="$1"
TARGET_USER="www"
TARGET_GROUP="www"
DOWNLOAD_ROOT="/data/downloads"
CONFIG_PATH="\${ARIAMX_CONFIG_PATH:?missing ARIAMX_CONFIG_PATH}"

python3 - "$GID" "$TARGET_USER" "$TARGET_GROUP" "$DOWNLOAD_ROOT" "$CONFIG_PATH" <<'PY'
import json
import os
import pwd
import grp
import sys
import urllib.request

gid, target_user, target_group, download_root, config_path = sys.argv[1:6]
root = os.path.realpath(download_root)
uid = pwd.getpwnam(target_user).pw_uid
group_id = grp.getgrnam(target_group).gr_gid

with open(config_path, "r", encoding="utf-8") as file:
    cfg = json.load(file)

aria2 = cfg["aria2"]
rpc_url = aria2["rpcUrl"]
rpc_secret = aria2["rpcSecret"]

def rpc(method, params):
    payload = json.dumps({
        "jsonrpc": "2.0",
        "id": "hook",
        "method": method,
        "params": [f"token:{rpc_secret}", *params],
    }).encode("utf-8")
    req = urllib.request.Request(rpc_url, data=payload, headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=15) as resp:
        result = json.loads(resp.read())
    if "error" in result:
        raise RuntimeError(result["error"])
    return result["result"]

status = rpc("aria2.tellStatus", [gid, ["gid", "files", "bittorrent"]])
is_bt = bool(status.get("bittorrent"))
paths = []

for item in status.get("files", []):
    path = item.get("path") or ""
    if not path:
        continue
    real = os.path.realpath(path)
    if real == root or not real.startswith(root + os.sep):
        raise RuntimeError(f"refuse unsafe path outside {root}: {real}")
    paths.append(real)

directories = set()
for path in paths:
    if os.path.exists(path):
        os.chown(path, uid, group_id)
        os.chmod(path, 0o640)
    sidecar = path + ".aria2"
    if os.path.exists(sidecar):
        os.remove(sidecar)
    parent = os.path.dirname(path)
    while parent.startswith(root + os.sep):
        directories.add(parent)
        parent = os.path.dirname(parent)

for directory in sorted(directories, key=len, reverse=True):
    if os.path.isdir(directory):
        os.chown(directory, uid, group_id)
        os.chmod(directory, 0o750)

if is_bt:
    try:
        rpc("aria2.remove", [gid])
    except Exception:
        pass

print(f"hook finished: gid={gid} files={len(paths)} bt={is_bt}")
PY
`

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

function useTemplate() {
  updateActiveHook({ content: safeTemplate })
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
  <section class="panel script-settings-panel">
    <div class="script-head">
      <div class="section-title">
        <FileCode2 :size="17" />
        <span>任务脚本</span>
      </div>
      <div class="button-row">
        <button class="ghost" :disabled="loading" @click="useTemplate">
          使用安全模板
        </button>
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
  background: var(--panel-muted);
  border: 1px solid var(--line);
  border-radius: 8px;
  color: var(--muted);
  min-height: 40px;
  padding: 0 12px;
}

.script-tabs button.active {
  background: var(--accent-soft);
  border-color: color-mix(in srgb, var(--accent) 35%, var(--line));
  color: var(--accent-text);
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
  background: var(--field-bg);
  border: 1px solid var(--line);
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
  background: var(--panel-muted);
  border-color: var(--line);
  color: var(--muted);
}

.script-editor :deep(.cm-activeLine),
.script-editor :deep(.cm-activeLineGutter) {
  background: color-mix(in srgb, var(--accent) 9%, transparent);
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
