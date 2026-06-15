package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigCreatesDefaultFile(t *testing.T) {
	t.Setenv("ARIAMX_ADMIN_PASSWORD", "change-me")
	path := filepath.Join(t.TempDir(), "ariamx.json")
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Admin.Username != "admin" {
		t.Fatalf("unexpected username %q", cfg.Admin.Username)
	}
	if cfg.Aria2.RPCURL == "" {
		t.Fatal("expected default aria2 rpc url")
	}
	if !cfg.Aria2.Managed {
		t.Fatal("expected managed aria2 to be enabled by default")
	}
	if cfg.Aria2.ManagedRPCPort != 16800 {
		t.Fatalf("expected managed rpc port 16800, got %d", cfg.Aria2.ManagedRPCPort)
	}
	if cfg.Aria2.RPCSecret == "" {
		t.Fatal("expected managed aria2 rpc secret to be generated")
	}
	if cfg.Panel.Theme != "design" {
		t.Fatalf("expected default theme design, got %q", cfg.Panel.Theme)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected config file: %v", err)
	}
	if !VerifyPassword("change-me", cfg.Admin.PasswordSalt, cfg.Admin.PasswordHash) {
		t.Fatal("expected configured password to verify")
	}
}

func TestLoadConfigMigratesManagedRPCPortFromOptions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ariamx.json")
	if err := os.WriteFile(path, []byte(`{
  "admin": {"username":"admin","passwordHash":"hash","passwordSalt":"salt"},
  "aria2": {
    "managed": true,
    "rpcUrl": "http://127.0.0.1:16800/jsonrpc",
    "rpcSecret": "secret",
    "managedRpcPort": 16800,
    "options": {
      "rpc-listen-port": "16888",
      "rpc-secure": "true"
    }
  },
  "panel": {
    "refreshIntervalMs": 1500,
    "sessionTTLSeconds": 86400,
    "theme": "design"
  }
}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Aria2.ManagedRPCPort != 16888 {
		t.Fatalf("expected migrated managed rpc port 16888, got %d", cfg.Aria2.ManagedRPCPort)
	}
	if cfg.Aria2.RPCURL != "http://127.0.0.1:16888/jsonrpc" {
		t.Fatalf("unexpected rpc url %q", cfg.Aria2.RPCURL)
	}
	if _, ok := cfg.Aria2.Options["rpc-listen-port"]; ok {
		t.Fatal("expected rpc-listen-port to be removed from managed options")
	}
	if _, ok := cfg.Aria2.Options["rpc-secure"]; ok {
		t.Fatal("expected rpc-secure to be removed from managed options")
	}
}
