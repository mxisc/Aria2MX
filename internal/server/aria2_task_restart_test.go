package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestRestartTaskReaddsURIAndRemovesOldResult(t *testing.T) {
	var calls []observedRPCRequest
	aria2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req observedRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode aria2 request: %v", err)
		}
		calls = append(calls, req)
		switch req.Method {
		case "aria2.tellStatus":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      "ariamx",
				"result": map[string]interface{}{
					"gid":    "old-gid",
					"status": "error",
					"files": []map[string]interface{}{
						{
							"uris": []map[string]interface{}{
								{"uri": "https://example.com/file.iso"},
								{"uri": "https://example.com/file.iso"},
							},
						},
					},
				},
			})
		case "aria2.getOption":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      "ariamx",
				"result": map[string]interface{}{
					"dir":   "/tmp/downloads",
					"split": "16",
				},
			})
		case "aria2.addUri":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      "ariamx",
				"result":  "new-gid",
			})
		case "aria2.removeDownloadResult":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      "ariamx",
				"result":  "OK",
			})
		default:
			t.Fatalf("unexpected method %s", req.Method)
		}
	}))
	defer aria2.Close()

	server := &Server{
		cfg: &Config{
			Aria2: Aria2Config{
				RPCURL:    aria2.URL,
				RPCSecret: "aria-secret",
			},
		},
	}
	server.aria2 = NewAria2Client(func() Aria2Config { return server.cfg.Aria2 })

	newGID, err := server.restartTask("old-gid")
	if err != nil {
		t.Fatalf("restartTask error: %v", err)
	}
	if newGID != "new-gid" {
		t.Fatalf("expected new-gid, got %s", newGID)
	}
	if len(calls) != 4 {
		t.Fatalf("expected 4 aria2 calls, got %d", len(calls))
	}
	if calls[2].Method != "aria2.addUri" {
		t.Fatalf("expected third call addUri, got %s", calls[2].Method)
	}
	params := calls[2].Params
	if len(params) < 2 {
		t.Fatalf("expected addUri params, got %#v", params)
	}
	uris, ok := params[1].([]interface{})
	if !ok || len(uris) != 1 || uris[0] != "https://example.com/file.iso" {
		t.Fatalf("unexpected uris %#v", params[1])
	}
}

func TestUserFacingAria2Error(t *testing.T) {
	got := userFacingAria2Error(assertError("aria2 error 10: Download aborted."))
	want := "任务操作失败：Download aborted.（错误码 10）。"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestUserFacingAria2ErrorPieceLengthConflict(t *testing.T) {
	got := userFacingAria2Error(assertError("aria2 error 10: Detected a change in piece length"))
	want := "任务操作失败：检测到同名下载残留的 .aria2 控制文件与当前任务分片信息不一致。请使用“重新开始”自动清理后再试。"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestRestartTaskArchivesControlFileOnPieceLengthConflict(t *testing.T) {
	tempDir := t.TempDir()
	targetPath := filepath.Join(tempDir, "ubuntu.iso")
	controlPath := targetPath + ".aria2"
	if err := os.WriteFile(controlPath, []byte("control"), 0o600); err != nil {
		t.Fatalf("write control file: %v", err)
	}

	addAttempts := 0
	aria2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req observedRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode aria2 request: %v", err)
		}
		switch req.Method {
		case "aria2.tellStatus":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      "ariamx",
				"result": map[string]interface{}{
					"gid":    "old-gid",
					"status": "error",
					"files": []map[string]interface{}{
						{
							"path": targetPath,
							"uris": []map[string]interface{}{
								{"uri": "https://example.com/file.iso"},
							},
						},
					},
				},
			})
		case "aria2.getOption":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      "ariamx",
				"result":  map[string]interface{}{},
			})
		case "aria2.addUri":
			addAttempts++
			if addAttempts == 1 {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"jsonrpc": "2.0",
					"id":      "ariamx",
					"error": map[string]interface{}{
						"code":    10,
						"message": "Detected a change in piece length",
					},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      "ariamx",
				"result":  "new-gid",
			})
		case "aria2.removeDownloadResult":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      "ariamx",
				"result":  "OK",
			})
		default:
			t.Fatalf("unexpected method %s", req.Method)
		}
	}))
	defer aria2.Close()

	server := &Server{
		cfg: &Config{
			Aria2: Aria2Config{
				RPCURL:    aria2.URL,
				RPCSecret: "aria-secret",
			},
		},
	}
	server.aria2 = NewAria2Client(func() Aria2Config { return server.cfg.Aria2 })

	newGID, err := server.restartTask("old-gid")
	if err != nil {
		t.Fatalf("restartTask error: %v", err)
	}
	if newGID != "new-gid" {
		t.Fatalf("expected new-gid, got %s", newGID)
	}
	if addAttempts != 2 {
		t.Fatalf("expected 2 addUri attempts, got %d", addAttempts)
	}
	if _, err := os.Stat(controlPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected original control file archived, stat err=%v", err)
	}
	matches, err := filepath.Glob(controlPath + ".conflict-*")
	if err != nil {
		t.Fatalf("glob conflict file: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 archived control file, got %d", len(matches))
	}
}

func TestRestartTaskRejectsActiveTask(t *testing.T) {
	aria2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req observedRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode aria2 request: %v", err)
		}
		switch req.Method {
		case "aria2.tellStatus":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      "ariamx",
				"result": map[string]interface{}{
					"gid":    "active-gid",
					"status": "active",
				},
			})
		default:
			t.Fatalf("unexpected method %s", req.Method)
		}
	}))
	defer aria2.Close()

	server := &Server{
		cfg: &Config{
			Aria2: Aria2Config{
				RPCURL:    aria2.URL,
				RPCSecret: "aria-secret",
			},
		},
	}
	server.aria2 = NewAria2Client(func() Aria2Config { return server.cfg.Aria2 })

	_, err := server.restartTask("active-gid")
	if err == nil || err.Error() != "只有已停止或失败的任务才可以重新开始。" {
		t.Fatalf("unexpected error: %v", err)
	}
}

type staticError string

func (e staticError) Error() string { return string(e) }

func assertError(message string) error {
	return staticError(message)
}
