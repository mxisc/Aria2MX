package server

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type taskRemovalResult struct {
	DeletedPaths []string `json:"deletedPaths,omitempty"`
}

func (s *Server) removeTask(gid string) (taskRemovalResult, error) {
	statusResult, err := s.aria2.Call(Aria2CallRequest{
		Method: "aria2.tellStatus",
		Params: []interface{}{gid, []string{"gid", "status", "files"}},
	})
	if err != nil {
		return taskRemovalResult{}, errors.New(userFacingAria2Error(err))
	}
	status, ok := statusResult.(map[string]interface{})
	if !ok {
		return taskRemovalResult{}, errors.New("任务读取失败，请刷新后重试。")
	}

	taskStatus, _ := status["status"].(string)
	files, _ := status["files"].([]interface{})

	method := "aria2.remove"
	switch taskStatus {
	case "active":
		method = "aria2.forceRemove"
	case "waiting", "paused":
		method = "aria2.remove"
	case "complete":
		method = "aria2.removeDownloadResult"
	default:
		method = "aria2.removeDownloadResult"
	}

	if _, err := s.aria2.Call(Aria2CallRequest{
		Method: method,
		Params: []interface{}{gid},
	}); err != nil {
		return taskRemovalResult{}, errors.New(userFacingAria2Error(err))
	}

	if taskStatus == "complete" {
		return taskRemovalResult{}, nil
	}

	deleted, err := deleteTaskFiles(files)
	if err != nil {
		return taskRemovalResult{DeletedPaths: deleted}, errors.New("任务已移除，但文件删除失败，请检查下载目录权限。")
	}
	return taskRemovalResult{DeletedPaths: deleted}, nil
}

func deleteTaskFiles(files []interface{}) ([]string, error) {
	paths := taskFilePaths(files)
	withControl := make([]string, 0, len(paths)*2)
	for _, path := range paths {
		withControl = append(withControl, path)
		withControl = append(withControl, path+".aria2")
	}

	deleted := make([]string, 0, len(withControl))
	seen := map[string]struct{}{}
	for _, path := range withControl {
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		deletedNow, err := deleteTaskPath(path)
		if err != nil {
			return deleted, err
		}
		if deletedNow {
			deleted = append(deleted, path)
		}
	}
	return deleted, nil
}

func deleteTaskPath(path string) (bool, error) {
	cleaned, err := validateTaskPath(path)
	if err != nil {
		return false, err
	}
	return deletePath(cleaned)
}

func validateTaskPath(path string) (string, error) {
	cleaned := strings.TrimSpace(path)
	if cleaned == "" {
		return "", errors.New("任务文件路径无效。")
	}
	if !filepath.IsAbs(cleaned) {
		return "", errors.New("任务文件路径无效。")
	}
	base := filepath.Base(cleaned)
	if base == "." || base == string(filepath.Separator) {
		return "", errors.New("任务文件路径无效。")
	}
	resolved := filepath.Clean(cleaned)
	if resolved == "/" {
		return "", errors.New("任务文件路径无效。")
	}
	info, err := os.Stat(resolved)
	if errors.Is(err, os.ErrNotExist) {
		return resolved, nil
	}
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", errors.New("任务文件路径无效。")
	}
	return resolved, nil
}

func deletePath(path string) (bool, error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	if err := os.RemoveAll(path); err != nil {
		return false, err
	}
	return true, nil
}
