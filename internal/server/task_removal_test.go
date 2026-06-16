package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveTaskDeletesUnfinishedFiles(t *testing.T) {
	root := t.TempDir()
	downloadDir := filepath.Join(root, "ariamx-data", "aria2", "downloads")
	if err := os.MkdirAll(downloadDir, 0o755); err != nil {
		t.Fatalf("mkdir download dir: %v", err)
	}
	filePath := filepath.Join(downloadDir, "partial.iso")
	controlPath := filePath + ".aria2"
	if err := os.WriteFile(filePath, []byte("data"), 0o600); err != nil {
		t.Fatalf("write data file: %v", err)
	}
	if err := os.WriteFile(controlPath, []byte("ctrl"), 0o600); err != nil {
		t.Fatalf("write control file: %v", err)
	}

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
					"gid":    "gid-1",
					"status": "active",
					"files": []map[string]interface{}{
						{"path": filePath},
					},
				},
			})
		case "aria2.forceRemove":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      "ariamx",
				"result":  "gid-1",
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

	result, err := server.removeTask("gid-1")
	if err != nil {
		t.Fatalf("removeTask error: %v", err)
	}
	if len(result.DeletedPaths) != 2 {
		t.Fatalf("expected 2 deleted paths, got %#v", result.DeletedPaths)
	}
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatalf("expected file deleted, stat err=%v", err)
	}
	if _, err := os.Stat(controlPath); !os.IsNotExist(err) {
		t.Fatalf("expected control file deleted, stat err=%v", err)
	}
	for _, deleted := range result.DeletedPaths {
		if deleted != filePath && deleted != controlPath {
			t.Fatalf("unexpected deleted path %s", deleted)
		}
	}
}

func TestRemoveTaskKeepsCompletedFilesInPlace(t *testing.T) {
	root := t.TempDir()
	downloadDir := filepath.Join(root, "ariamx-data", "aria2", "downloads")
	if err := os.MkdirAll(downloadDir, 0o755); err != nil {
		t.Fatalf("mkdir download dir: %v", err)
	}
	filePath := filepath.Join(downloadDir, "done.iso")
	if err := os.WriteFile(filePath, []byte("data"), 0o600); err != nil {
		t.Fatalf("write data file: %v", err)
	}

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
					"gid":    "gid-2",
					"status": "complete",
					"files": []map[string]interface{}{
						{"path": filePath},
					},
				},
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

	result, err := server.removeTask("gid-2")
	if err != nil {
		t.Fatalf("removeTask error: %v", err)
	}
	if len(result.DeletedPaths) != 0 {
		t.Fatalf("expected no deleted files, got %#v", result.DeletedPaths)
	}
	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("expected completed file kept in place, err=%v", err)
	}
}

func TestDeleteTaskPathRejectsDirectory(t *testing.T) {
	root := t.TempDir()
	downloadDir := filepath.Join(root, "ariamx-data", "aria2", "downloads")
	if err := os.MkdirAll(downloadDir, 0o755); err != nil {
		t.Fatalf("mkdir download dir: %v", err)
	}

	if _, err := deleteTaskPath(downloadDir); err == nil {
		t.Fatal("expected directory path to be rejected")
	}
}
