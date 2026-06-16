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
	if cfg.Panel.RPCSecret == "" {
		t.Fatal("expected panel rpc secret to be generated")
	}
	if cfg.Panel.RPCSecret == cfg.Aria2.RPCSecret {
		t.Fatal("expected panel rpc secret to differ from aria2 rpc secret")
	}
	if cfg.Panel.Theme != "ariamx" {
		t.Fatalf("expected default theme ariamx, got %q", cfg.Panel.Theme)
	}
	if cfg.Panel.ColorMode != "light" {
		t.Fatalf("expected default color mode light, got %q", cfg.Panel.ColorMode)
	}
	if !cfg.Panel.MCPEnabled {
		t.Fatal("expected mcp to be enabled by default")
	}
	if cfg.PeerGuard.AutoBanEnabled {
		t.Fatal("expected peer guard auto ban to be disabled by default")
	}
	if cfg.PeerGuard.AutoBanMinScore != 3 {
		t.Fatalf("expected peer guard auto ban score 3, got %d", cfg.PeerGuard.AutoBanMinScore)
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
	if cfg.Panel.Theme != "ariamx" {
		t.Fatalf("expected migrated theme ariamx, got %q", cfg.Panel.Theme)
	}
	if cfg.Panel.ColorMode != "light" {
		t.Fatalf("expected migrated color mode light, got %q", cfg.Panel.ColorMode)
	}
}

func TestLoadConfigMigratesClassicThemeToDarkMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ariamx.json")
	if err := os.WriteFile(path, []byte(`{
  "admin": {"username":"admin","passwordHash":"hash","passwordSalt":"salt"},
  "aria2": {
    "managed": true,
    "rpcUrl": "http://127.0.0.1:16800/jsonrpc",
    "rpcSecret": "secret",
    "managedRpcPort": 16800
  },
  "panel": {
    "refreshIntervalMs": 1500,
    "sessionTTLSeconds": 86400,
    "theme": "classic"
  }
}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Panel.Theme != "ariamx" {
		t.Fatalf("expected migrated theme ariamx, got %q", cfg.Panel.Theme)
	}
	if cfg.Panel.ColorMode != "dark" {
		t.Fatalf("expected migrated color mode dark, got %q", cfg.Panel.ColorMode)
	}
}

func TestLoadConfigKeepsSystemColorMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ariamx.json")
	if err := os.WriteFile(path, []byte(`{
  "admin": {"username":"admin","passwordHash":"hash","passwordSalt":"salt"},
  "aria2": {
    "managed": true,
    "rpcUrl": "http://127.0.0.1:16800/jsonrpc",
    "rpcSecret": "secret",
    "managedRpcPort": 16800
  },
  "panel": {
    "refreshIntervalMs": 1500,
    "sessionTTLSeconds": 86400,
    "theme": "ariamx",
    "colorMode": "system"
  }
}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Panel.ColorMode != "system" {
		t.Fatalf("expected system color mode to be preserved, got %q", cfg.Panel.ColorMode)
	}
}

func TestLoadConfigRegeneratesDuplicatePanelRPCSecret(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ariamx.json")
	if err := os.WriteFile(path, []byte(`{
  "admin": {"username":"admin","passwordHash":"hash","passwordSalt":"salt"},
  "aria2": {
    "managed": true,
    "rpcUrl": "http://127.0.0.1:16800/jsonrpc",
    "rpcSecret": "same-secret",
    "managedRpcPort": 16800
  },
  "panel": {
    "refreshIntervalMs": 1500,
    "sessionTTLSeconds": 86400,
    "rpcSecret": "same-secret",
    "theme": "ariamx",
    "colorMode": "light"
  }
}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Panel.RPCSecret == "" {
		t.Fatal("expected panel rpc secret to exist")
	}
	if cfg.Panel.RPCSecret == cfg.Aria2.RPCSecret {
		t.Fatal("expected duplicate panel rpc secret to be regenerated")
	}
}

func TestLoadConfigKeepsDisabledMCP(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ariamx.json")
	if err := os.WriteFile(path, []byte(`{
  "admin": {"username":"admin","passwordHash":"hash","passwordSalt":"salt"},
  "aria2": {
    "managed": true,
    "rpcUrl": "http://127.0.0.1:16800/jsonrpc",
    "rpcSecret": "secret",
    "managedRpcPort": 16800
  },
  "panel": {
    "refreshIntervalMs": 1500,
    "sessionTTLSeconds": 86400,
    "rpcSecret": "panel-secret",
    "mcpEnabled": false,
    "theme": "ariamx",
    "colorMode": "light"
  }
}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Panel.MCPEnabled {
		t.Fatal("expected disabled mcp to be preserved")
	}
}

func TestLoadConfigKeepsPeerGuardSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ariamx.json")
	if err := os.WriteFile(path, []byte(`{
  "admin": {"username":"admin","passwordHash":"hash","passwordSalt":"salt"},
  "aria2": {
    "managed": true,
    "rpcUrl": "http://127.0.0.1:16800/jsonrpc",
    "rpcSecret": "secret",
    "managedRpcPort": 16800
  },
  "panel": {
    "refreshIntervalMs": 1500,
    "sessionTTLSeconds": 86400,
    "rpcSecret": "panel-secret",
    "mcpEnabled": true,
    "theme": "ariamx",
    "colorMode": "light"
  },
  "peerGuard": {
    "autoBanEnabled": true,
    "autoBanMinScore": 4,
    "blockedPeers": [{"ip":"203.0.113.10","reason":"manual"}]
  }
}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.PeerGuard.AutoBanEnabled {
		t.Fatal("expected peer guard auto ban enabled to be preserved")
	}
	if cfg.PeerGuard.AutoBanMinScore != 4 {
		t.Fatalf("expected peer guard auto ban min score 4, got %d", cfg.PeerGuard.AutoBanMinScore)
	}
	if len(cfg.PeerGuard.BlockedPeers) != 1 || cfg.PeerGuard.BlockedPeers[0].IP != "203.0.113.10" {
		t.Fatalf("unexpected blocked peers: %#v", cfg.PeerGuard.BlockedPeers)
	}
}
