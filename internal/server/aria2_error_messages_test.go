package server

import "testing"

func TestUserFacingAria2TaskMessage(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		message string
		want    string
	}{
		{
			name:    "disk full",
			code:    "9",
			message: "fallocate failed. cause: No space left on device",
			want:    "磁盘空间不足，无法创建或继续下载文件。请清理下载目录或更换到剩余空间更大的磁盘后重试。",
		},
		{
			name:    "permission denied",
			code:    "16",
			message: "Operation not permitted",
			want:    "当前下载目录不可写，无法创建或写入文件。请检查下载目录权限后重试。",
		},
		{
			name:    "tls issuer",
			code:    "1",
			message: "SSL/TLS handshake failure: unable to get local issuer certificate",
			want:    "TLS 握手失败：无法验证下载源站的证书链。请检查目标站点证书，或为 aria2 配置正确的 CA 证书后重试。",
		},
		{
			name:    "infohash duplicate",
			code:    "12",
			message: "InfoHash dafc8c076ca2f3ed376eeae7c76a0d6be2415c45 is already registered.",
			want:    "该 BT 任务已存在，不能重复添加同一个种子。",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := userFacingAria2TaskMessage(tt.code, tt.message); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestLocalizeAria2ResultTellStopped(t *testing.T) {
	raw := []interface{}{
		map[string]interface{}{
			"gid":          "gid-stopped-1",
			"status":       "error",
			"errorCode":    "16",
			"errorMessage": "Operation not permitted",
		},
	}

	localized, ok := localizeAria2Result("aria2.tellStopped", raw).([]interface{})
	if !ok || len(localized) != 1 {
		t.Fatalf("unexpected localized payload %#v", localized)
	}
	task, ok := localized[0].(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected task payload %#v", localized[0])
	}
	want := "当前下载目录不可写，无法创建或写入文件。请检查下载目录权限后重试。"
	if got, _ := task["errorMessage"].(string); got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
