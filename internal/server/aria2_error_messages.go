package server

import (
	"fmt"
	"strings"
)

func localizeAria2Result(method string, result interface{}) interface{} {
	switch method {
	case "aria2.tellActive", "aria2.tellWaiting", "aria2.tellStopped":
		items, ok := result.([]interface{})
		if !ok {
			return result
		}
		for _, item := range items {
			task, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			localizeAria2Task(task)
		}
	case "aria2.tellStatus":
		task, ok := result.(map[string]interface{})
		if !ok {
			return result
		}
		localizeAria2Task(task)
	}
	return result
}

func localizeAria2Task(task map[string]interface{}) {
	code, _ := task["errorCode"].(string)
	message, _ := task["errorMessage"].(string)
	if strings.TrimSpace(message) == "" {
		return
	}
	task["errorMessage"] = userFacingAria2TaskMessage(code, message)
}

func userFacingAria2TaskMessage(code, message string) string {
	text := strings.TrimSpace(message)
	switch {
	case text == "":
		return ""
	case isPieceLengthConflictError(text):
		return "检测到同名下载残留的 .aria2 控制文件与当前任务分片信息不一致。请使用“重新开始”自动清理后再试。"
	case isPermissionDeniedOpenFileError(text), isPermissionDeniedTaskError(text):
		return "当前下载目录不可写，无法创建或写入文件。请检查下载目录权限后重试。"
	case isNoSpaceLeftError(text):
		return "磁盘空间不足，无法创建或继续下载文件。请清理下载目录或更换到剩余空间更大的磁盘后重试。"
	case isTLSIssuerCertificateError(text):
		return "TLS 握手失败：无法验证下载源站的证书链。请检查目标站点证书，或为 aria2 配置正确的 CA 证书后重试。"
	case isInfoHashAlreadyRegisteredError(text):
		return "该 BT 任务已存在，不能重复添加同一个种子。"
	case strings.Contains(text, "Download aborted"):
		return "下载已中止，请检查任务详情后重试。"
	default:
		if code != "" {
			return fmt.Sprintf("任务失败（错误码 %s）：%s", code, text)
		}
		return fmt.Sprintf("任务失败：%s", text)
	}
}

func isPermissionDeniedTaskError(text string) bool {
	return strings.Contains(text, "Operation not permitted") || strings.Contains(text, "Permission denied")
}

func isNoSpaceLeftError(text string) bool {
	return strings.Contains(text, "No space left on device") || strings.Contains(text, "fallocate failed")
}

func isTLSIssuerCertificateError(text string) bool {
	return strings.Contains(text, "unable to get local issuer certificate")
}

func isInfoHashAlreadyRegisteredError(text string) bool {
	return strings.Contains(text, "InfoHash") && strings.Contains(text, "already registered")
}
