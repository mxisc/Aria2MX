package server

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestApplyManagedOptionPatchSyncsRPCPort(t *testing.T) {
	cfg := &Config{
		Aria2: Aria2Config{
			Managed:        true,
			ManagedRPCPort: defaultManagedRPCPort,
			RPCURL:         managedRPCURL(defaultManagedRPCPort),
			Options:        map[string]string{},
		},
	}

	patch, err := applyManagedOptionPatch(cfg, map[string]string{
		"rpc-listen-port":    "16888",
		"max-download-limit": "1M",
	})
	if err != nil {
		t.Fatalf("apply patch: %v", err)
	}
	if cfg.Aria2.ManagedRPCPort != 16888 {
		t.Fatalf("expected managed rpc port 16888, got %d", cfg.Aria2.ManagedRPCPort)
	}
	if cfg.Aria2.RPCURL != "http://127.0.0.1:16888/jsonrpc" {
		t.Fatalf("unexpected rpc url %q", cfg.Aria2.RPCURL)
	}
	if _, ok := patch["rpc-listen-port"]; ok {
		t.Fatal("expected rpc-listen-port to be handled by manager instead of aria2.changeGlobalOption")
	}
	if patch["max-download-limit"] != "1M" {
		t.Fatalf("expected max-download-limit to remain in patch, got %q", patch["max-download-limit"])
	}
}

func TestApplyManagedOptionPatchRejectsInvalidRPCPort(t *testing.T) {
	cfg := &Config{
		Aria2: Aria2Config{
			Managed:        true,
			ManagedRPCPort: defaultManagedRPCPort,
			RPCURL:         managedRPCURL(defaultManagedRPCPort),
		},
	}

	if _, err := applyManagedOptionPatch(cfg, map[string]string{"rpc-listen-port": "abc"}); err == nil {
		t.Fatal("expected invalid port error")
	}
}

func TestApplyManagedOptionPatchNormalizesTrackerList(t *testing.T) {
	cfg := &Config{
		Aria2: Aria2Config{
			Managed:        true,
			ManagedRPCPort: defaultManagedRPCPort,
			RPCURL:         managedRPCURL(defaultManagedRPCPort),
			Options:        map[string]string{},
		},
	}

	patch, err := applyManagedOptionPatch(cfg, map[string]string{
		"bt-tracker": "http://a/announce\nhttp://b/announce\nhttp://a/announce",
	})
	if err != nil {
		t.Fatalf("apply patch: %v", err)
	}

	want := "http://a/announce,http://b/announce"
	if got := cfg.Aria2.Options["bt-tracker"]; got != want {
		t.Fatalf("expected normalized tracker list %q, got %q", want, got)
	}
	if got := patch["bt-tracker"]; got != want {
		t.Fatalf("expected patch tracker list %q, got %q", want, got)
	}
}

func TestApplyManagedOptionPatchNormalizesUnitOptions(t *testing.T) {
	cfg := &Config{
		Aria2: Aria2Config{
			Managed:        true,
			ManagedRPCPort: defaultManagedRPCPort,
			RPCURL:         managedRPCURL(defaultManagedRPCPort),
			Options:        map[string]string{},
		},
	}

	patch, err := applyManagedOptionPatch(cfg, map[string]string{
		"disk-cache":                 " 64 m ",
		"max-download-limit":         "512kb",
		"max-overall-download-limit": "1.5G",
	})
	if err != nil {
		t.Fatalf("apply patch: %v", err)
	}

	want := map[string]string{
		"disk-cache":                 "64M",
		"max-download-limit":         "512K",
		"max-overall-download-limit": "1610612736",
	}
	for key, value := range want {
		if got := cfg.Aria2.Options[key]; got != value {
			t.Fatalf("expected %s option %q, got %q", key, value, got)
		}
		if got := patch[key]; got != value {
			t.Fatalf("expected %s patch %q, got %q", key, value, got)
		}
	}
}

func TestApplyManagedOptionPatchRejectsInvalidUnitOption(t *testing.T) {
	cfg := &Config{
		Aria2: Aria2Config{
			Managed:        true,
			ManagedRPCPort: defaultManagedRPCPort,
			RPCURL:         managedRPCURL(defaultManagedRPCPort),
			Options:        map[string]string{},
		},
	}

	if _, err := applyManagedOptionPatch(cfg, map[string]string{"max-download-limit": "1Q"}); err == nil {
		t.Fatal("expected invalid unit option error")
	}
	if _, err := applyManagedOptionPatch(cfg, map[string]string{"max-download-limit": "1.5"}); err == nil {
		t.Fatal("expected decimal byte value error")
	}
}

func TestWriteConfigFileKeepsTrackerListOnSingleLine(t *testing.T) {
	cfg := &Config{
		Panel: PanelConfig{
			DefaultDownloadDir: "/tmp/downloads",
		},
	}
	manager := NewManagedAria2("aria2mx.json", cfg, &sync.RWMutex{}, nil)
	root := t.TempDir()

	confPath, err := manager.writeConfigFile(root, Aria2Config{
		Options: map[string]string{
			"bt-tracker": "http://a/announce\nhttp://b/announce",
		},
	})
	if err != nil {
		t.Fatalf("write config: %v", err)
	}

	data, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "bt-tracker=http://a/announce,http://b/announce\n") {
		t.Fatalf("expected single-line bt-tracker entry, got %q", text)
	}
	if strings.Contains(text, "\nhttp://b/announce\n") {
		t.Fatalf("unexpected bare tracker line in config: %q", text)
	}
	if !strings.Contains(text, "disk-cache=64M\n") {
		t.Fatalf("expected reference default disk-cache in config, got %q", text)
	}
	if !strings.Contains(text, "save-session-interval=1\n") {
		t.Fatalf("expected reference default save-session-interval in config, got %q", text)
	}
	if !strings.Contains(text, "auto-save-interval=20\n") {
		t.Fatalf("expected reference default auto-save-interval in config, got %q", text)
	}
	if !strings.Contains(text, "bt-force-encryption=true\n") {
		t.Fatalf("expected reference default bt-force-encryption in config, got %q", text)
	}
	if _, err := os.Stat(filepath.Join(root, "aria2.conf")); err != nil {
		t.Fatalf("expected config file to exist: %v", err)
	}
}

func TestEffectiveManagedOptionsAllowsUserOverride(t *testing.T) {
	cfg := &Config{
		Panel: PanelConfig{
			DefaultDownloadDir: "/tmp/downloads",
		},
	}
	cfgMu := &sync.RWMutex{}
	manager := NewManagedAria2("aria2mx.json", cfg, cfgMu, nil)

	options := manager.effectiveManagedOptions("/tmp/aria2mx-data/aria2", Aria2Config{
		Options: map[string]string{
			"disk-cache": "32M",
			"dir":        "/data/custom",
		},
	})

	if got := options["disk-cache"]; got != "32M" {
		t.Fatalf("expected disk-cache override to be preserved, got %q", got)
	}
	if got := options["dir"]; got != "/data/custom" {
		t.Fatalf("expected dir override to be preserved, got %q", got)
	}
	if got := options["split"]; got != "64" {
		t.Fatalf("expected reference default split to exist, got %q", got)
	}
}

func TestManagedCACertificatePathUsesExistingSystemBundle(t *testing.T) {
	cfg := Aria2Config{
		Options: map[string]string{},
	}
	path := managedCACertificatePath(cfg)
	if path == "" {
		t.Skip("no known system CA bundle path on this machine")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected detected CA bundle to exist: %v", err)
	}
}

func TestManagedCACertificatePathRespectsExplicitConfig(t *testing.T) {
	cfg := Aria2Config{
		Options: map[string]string{
			"ca-certificate": "/custom/ca.pem",
		},
	}
	if path := managedCACertificatePath(cfg); path != "" {
		t.Fatalf("expected manager not to override explicit ca-certificate, got %q", path)
	}
}

func TestFindAvailableManagedRPCPortUsesStepTen(t *testing.T) {
	first, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen first: %v", err)
	}
	defer first.Close()
	basePort := first.Addr().(*net.TCPAddr).Port

	second, err := net.Listen("tcp4", "127.0.0.1:"+strconv.Itoa(basePort+10))
	if err != nil {
		t.Fatalf("listen second: %v", err)
	}
	defer second.Close()

	port, err := findAvailableManagedRPCPort(basePort, 10)
	if err != nil {
		t.Fatalf("find available port: %v", err)
	}
	if port != basePort+20 {
		t.Fatalf("expected stepped port %d, got %d", basePort+20, port)
	}
}

func TestCanReuseExistingLocked(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"result": map[string]interface{}{"version": "1.37.0"},
		})
	}))
	defer server.Close()

	port, err := serverPort(server.URL)
	if err != nil {
		t.Fatalf("server port: %v", err)
	}
	cfg := &Config{
		Aria2: Aria2Config{
			Managed:        true,
			RPCURL:         server.URL,
			RPCSecret:      "secret",
			ManagedRPCPort: port,
		},
	}
	manager := NewManagedAria2("aria2mx.json", cfg, &sync.RWMutex{}, NewAria2Client(func() Aria2Config {
		return cfg.Aria2
	}))

	if !manager.canReuseExistingLocked() {
		t.Fatal("expected existing aria2 process to be reusable")
	}
}

func TestStopLockedShutsDownReusedProcess(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	shutdownCh := make(chan struct{}, 1)
	var server *http.Server
	server = &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		method, _ := payload["method"].(string)
		switch method {
		case "aria2.forceShutdown":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"result": "OK"})
			go func() {
				time.Sleep(100 * time.Millisecond)
				_ = server.Close()
				shutdownCh <- struct{}{}
			}()
		default:
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"result": map[string]interface{}{"version": "1.37.0"},
			})
		}
	})}
	go func() {
		_ = server.Serve(listener)
	}()

	cfg := &Config{
		Aria2: Aria2Config{
			Managed:        true,
			RPCURL:         "http://127.0.0.1:" + strconv.Itoa(port),
			RPCSecret:      "secret",
			ManagedRPCPort: port,
		},
	}
	cfgMu := &sync.RWMutex{}
	manager := NewManagedAria2("aria2mx.json", cfg, cfgMu, NewAria2Client(func() Aria2Config {
		cfgMu.RLock()
		defer cfgMu.RUnlock()
		return cfg.Aria2
	}))
	manager.reused = true

	if err := manager.stopLocked(); err != nil {
		t.Fatalf("stop reused aria2: %v", err)
	}
	select {
	case <-shutdownCh:
	case <-time.After(2 * time.Second):
		t.Fatal("expected forceShutdown to close reused aria2 listener")
	}
}

func serverPort(rawURL string) (int, error) {
	_, portText, err := net.SplitHostPort(strings.TrimPrefix(rawURL, "http://"))
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(portText)
}
