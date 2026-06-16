package server

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func userFacingAria2Error(err error) string {
	if err == nil {
		return "aria2 暂时不可用，请检查连接设置。"
	}
	text := strings.TrimSpace(err.Error())
	if isPieceLengthConflictError(text) {
		return "任务操作失败：检测到同名下载残留的 .aria2 控制文件与当前任务分片信息不一致。请使用“重新开始”自动清理后再试。"
	}
	if isPermissionDeniedOpenFileError(text) {
		return "任务操作失败：当前下载目录不可写，请检查下载目录权限后重试。"
	}
	if strings.HasPrefix(text, "aria2 error ") {
		detail := strings.TrimPrefix(text, "aria2 error ")
		parts := strings.SplitN(detail, ": ", 2)
		if len(parts) == 2 {
			return fmt.Sprintf("任务操作失败：%s（错误码 %s）。", strings.TrimSpace(parts[1]), strings.TrimSpace(parts[0]))
		}
	}
	if strings.HasPrefix(text, "aria2 unreachable:") {
		return "aria2 暂时不可用，请检查连接设置。"
	}
	if text == "" {
		return "aria2 暂时不可用，请检查连接设置。"
	}
	return fmt.Sprintf("任务操作失败：%s", text)
}

func (s *Server) restartTask(gid string) (string, error) {
	statusResult, err := s.aria2.Call(Aria2CallRequest{
		Method: "aria2.tellStatus",
		Params: []interface{}{gid, []string{"gid", "dir", "files", "status"}},
	})
	if err != nil {
		return "", errors.New(userFacingAria2Error(err))
	}
	status, ok := statusResult.(map[string]interface{})
	if !ok {
		return "", errors.New("任务读取失败，请刷新后重试。")
	}

	taskStatus, _ := status["status"].(string)
	switch taskStatus {
	case "complete", "error", "removed":
	default:
		return "", errors.New("只有已停止或失败的任务才可以重新开始。")
	}

	files, _ := status["files"].([]interface{})
	uris := uniqueTaskURIs(files)
	if len(uris) == 0 {
		return "", errors.New("当前任务没有可复用的下载地址，暂时无法直接重新开始。")
	}

	optionResult, err := s.aria2.Call(Aria2CallRequest{
		Method: "aria2.getOption",
		Params: []interface{}{gid},
	})
	if err != nil {
		return "", errors.New(userFacingAria2Error(err))
	}
	options := taskOptionsMap(optionResult)

	newResult, err := s.aria2.Call(Aria2CallRequest{
		Method: "aria2.addUri",
		Params: []interface{}{uris, options},
	})
	if err != nil {
		if isPieceLengthConflictError(err.Error()) {
			if archiveErr := archiveTaskControlFiles(files); archiveErr != nil {
				return "", errors.New("检测到旧续传控制文件冲突，但自动清理失败，请检查下载目录权限后重试。")
			}
			newResult, err = s.aria2.Call(Aria2CallRequest{
				Method: "aria2.addUri",
				Params: []interface{}{uris, options},
			})
		}
	}
	if err != nil {
		return "", errors.New(userFacingAria2Error(err))
	}
	newGID, _ := newResult.(string)
	if newGID == "" {
		return "", errors.New("重新开始失败，请稍后重试。")
	}

	_, _ = s.aria2.Call(Aria2CallRequest{
		Method: "aria2.removeDownloadResult",
		Params: []interface{}{gid},
	})
	return newGID, nil
}

func uniqueTaskURIs(files []interface{}) []string {
	seen := map[string]struct{}{}
	uris := make([]string, 0)
	for _, fileItem := range files {
		fileMap, ok := fileItem.(map[string]interface{})
		if !ok {
			continue
		}
		fileURIs, _ := fileMap["uris"].([]interface{})
		for _, uriItem := range fileURIs {
			uriMap, ok := uriItem.(map[string]interface{})
			if !ok {
				continue
			}
			uri, _ := uriMap["uri"].(string)
			if uri == "" {
				continue
			}
			if _, exists := seen[uri]; exists {
				continue
			}
			seen[uri] = struct{}{}
			uris = append(uris, uri)
		}
	}
	return uris
}

func taskOptionsMap(result interface{}) map[string]string {
	raw, ok := result.(map[string]interface{})
	if !ok {
		return map[string]string{}
	}
	options := make(map[string]string, len(raw))
	for key, value := range raw {
		text, ok := value.(string)
		if ok {
			options[key] = text
		}
	}
	delete(options, "pause")
	return options
}

func isPieceLengthConflictError(text string) bool {
	return strings.Contains(text, "Detected a change in piece length")
}

func isPermissionDeniedOpenFileError(text string) bool {
	return strings.Contains(text, "Failed to open the file") && strings.Contains(text, "Operation not permitted")
}

func archiveTaskControlFiles(files []interface{}) error {
	for _, path := range taskFilePaths(files) {
		controlFile := path + ".aria2"
		if err := archiveControlFile(controlFile); err != nil {
			return err
		}
	}
	return nil
}

func taskFilePaths(files []interface{}) []string {
	seen := map[string]struct{}{}
	paths := make([]string, 0, len(files))
	for _, fileItem := range files {
		fileMap, ok := fileItem.(map[string]interface{})
		if !ok {
			continue
		}
		path, _ := fileMap["path"].(string)
		if path == "" {
			continue
		}
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	return paths
}

func archiveControlFile(path string) error {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	archivedPath := fmt.Sprintf("%s.conflict-%s", path, time.Now().Format("20060102150405"))
	if err := os.MkdirAll(filepath.Dir(archivedPath), 0o755); err != nil {
		return err
	}
	return os.Rename(path, archivedPath)
}
