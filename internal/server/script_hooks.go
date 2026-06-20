package server

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type scriptHookDefinition struct {
	Key      string
	Option   string
	FileName string
	Title    string
}

type scriptHookItem struct {
	Key     string `json:"key"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

type scriptHookState struct {
	Hooks   []scriptHookItem `json:"hooks"`
	Message string           `json:"message,omitempty"`
}

var managedScriptHookDefinitions = []scriptHookDefinition{
	{Key: "downloadComplete", Option: "on-download-complete", FileName: "download-complete.sh", Title: "普通任务完成"},
	{Key: "btDownloadComplete", Option: "on-bt-download-complete", FileName: "bt-download-complete.sh", Title: "BT 下载完成"},
	{Key: "downloadError", Option: "on-download-error", FileName: "download-error.sh", Title: "下载失败"},
	{Key: "downloadStop", Option: "on-download-stop", FileName: "download-stop.sh", Title: "下载停止"},
}

func (m *ManagedAria2) scriptHookPath(def scriptHookDefinition) string {
	cfg := m.snapshotConfig()
	return filepath.Join(m.stateRoot(cfg), "hooks", def.FileName)
}

func (s *Server) handleScriptHooks(w http.ResponseWriter, r *http.Request) {
	if s.managed == nil {
		writeAPIError(w, http.StatusBadRequest, "managed_disabled", "当前 aria2 不是由面板托管，无法使用脚本设置。")
		return
	}

	switch r.Method {
	case http.MethodGet:
		state, err := s.currentScriptHookState("")
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "script_read_failed", "脚本读取失败，请稍后重试。")
			return
		}
		writeJSON(w, http.StatusOK, apiResponse{OK: true, Data: state})
	case http.MethodPost:
		var payload struct {
			Hooks []scriptHookItem `json:"hooks"`
		}
		if err := readJSON(r, &payload); err != nil {
			writeAPIError(w, http.StatusBadRequest, "bad_request", "请检查脚本内容后重试。")
			return
		}
		if err := s.saveScriptHooks(payload.Hooks); err != nil {
			writeAPIError(w, http.StatusBadRequest, "script_save_failed", err.Error())
			return
		}
		state, err := s.currentScriptHookState("脚本设置已保存。")
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "script_read_failed", "脚本保存成功，但状态读取失败。")
			return
		}
		writeJSON(w, http.StatusOK, apiResponse{OK: true, Data: state})
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) currentScriptHookState(message string) (scriptHookState, error) {
	s.cfgMu.RLock()
	options := s.cfg.Aria2.Options
	s.cfgMu.RUnlock()

	hooks := make([]scriptHookItem, 0, len(managedScriptHookDefinitions))
	for _, def := range managedScriptHookDefinitions {
		scriptPath := s.managed.scriptHookPath(def)
		content, err := os.ReadFile(scriptPath)
		if errors.Is(err, os.ErrNotExist) {
			content = nil
		} else if err != nil {
			return scriptHookState{}, err
		}
		hooks = append(hooks, scriptHookItem{
			Key:   def.Key,
			Title: def.Title,
			Content: scriptHookContentForResponse(
				string(content),
				options != nil && options[def.Option] == scriptPath,
			),
		})
	}

	return scriptHookState{Hooks: hooks, Message: message}, nil
}

func (s *Server) saveScriptHooks(items []scriptHookItem) error {
	itemByKey := make(map[string]scriptHookItem, len(items))
	for _, item := range items {
		itemByKey[item.Key] = item
	}

	patch := make(map[string]string, len(managedScriptHookDefinitions))
	for _, def := range managedScriptHookDefinitions {
		item, ok := itemByKey[def.Key]
		if !ok {
			continue
		}
		scriptPath := s.managed.scriptHookPath(def)
		normalizedContent := normalizeScriptHookContent(item.Content)
		if strings.TrimSpace(normalizedContent) != "" {
			if err := writeScriptHook(scriptPath, normalizedContent); err != nil {
				return err
			}
			patch[def.Option] = scriptPath
		} else {
			patch[def.Option] = ""
		}
	}

	if len(patch) == 0 {
		return nil
	}
	if _, err := s.managed.SaveOptions(patch); err != nil {
		return fmt.Errorf("脚本已保存，但绑定到 aria2 失败。")
	}
	return nil
}

func scriptHookContentForResponse(content string, enabled bool) string {
	if enabled {
		return content
	}
	if strings.TrimSpace(content) == "" {
		return ""
	}
	return ""
}

func writeScriptHook(scriptPath, content string) error {
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0o700); err != nil {
		return fmt.Errorf("创建脚本目录失败。")
	}
	tmp := scriptPath + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o700); err != nil {
		return fmt.Errorf("写入脚本失败。")
	}
	if err := os.Rename(tmp, scriptPath); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("保存脚本失败。")
	}
	return nil
}

func normalizeScriptHookContent(content string) string {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalized = strings.TrimSpace(normalized)
	if normalized == "" {
		return ""
	}
	if !strings.HasPrefix(normalized, "#!") {
		normalized = "#!/usr/bin/env bash\n" + normalized
	}
	return normalized + "\n"
}
