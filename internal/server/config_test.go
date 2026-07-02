package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigCreatesDefaultFile(t *testing.T) {
	t.Setenv("ARIA2MX_ADMIN_PASSWORD", "change-me")
	path := filepath.Join(t.TempDir(), "aria2mx.json")
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
	if cfg.Panel.Theme != "aria2mx" {
		t.Fatalf("expected default theme aria2mx, got %q", cfg.Panel.Theme)
	}
	if cfg.Panel.ColorMode != "system" {
		t.Fatalf("expected default color mode system, got %q", cfg.Panel.ColorMode)
	}
	if cfg.Panel.SkinEnabled {
		t.Fatal("expected skin to be disabled by default")
	}
	if cfg.Panel.SkinName != "default" {
		t.Fatalf("expected default skin name default, got %q", cfg.Panel.SkinName)
	}
	if cfg.Panel.SkinAPITemplate != "" {
		t.Fatalf("expected empty skin api template, got %q", cfg.Panel.SkinAPITemplate)
	}
	if cfg.Panel.RPCOriginCheckMode != panelRPCOriginModeSameOrigin {
		t.Fatalf("expected default rpc origin check mode same_origin, got %q", cfg.Panel.RPCOriginCheckMode)
	}
	if cfg.Admin.PasswordScheme != passwordSchemeClientSHA256PBKDF2 {
		t.Fatalf("expected default password scheme %q, got %q", passwordSchemeClientSHA256PBKDF2, cfg.Admin.PasswordScheme)
	}
	if len(cfg.Panel.RPCOriginWhitelist) != 0 {
		t.Fatalf("expected empty rpc origin whitelist, got %#v", cfg.Panel.RPCOriginWhitelist)
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
	if !VerifyPassword(SHA256Hex("change-me"), cfg.Admin.PasswordSalt, cfg.Admin.PasswordHash, cfg.Admin.PasswordScheme) {
		t.Fatal("expected configured password to verify")
	}
}

func TestLoadConfigMigratesManagedRPCPortFromOptions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aria2mx.json")
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
	if cfg.Panel.Theme != "aria2mx" {
		t.Fatalf("expected migrated theme aria2mx, got %q", cfg.Panel.Theme)
	}
	if cfg.Panel.ColorMode != "system" {
		t.Fatalf("expected migrated color mode system, got %q", cfg.Panel.ColorMode)
	}
	if cfg.Panel.SkinName != "default" {
		t.Fatalf("expected migrated skin name default, got %q", cfg.Panel.SkinName)
	}
}

func TestLoadConfigDisablesSkinWithoutTemplate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aria2mx.json")
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
    "theme": "aria2mx",
    "colorMode": "light",
    "skinEnabled": true,
    "skinName": "  aurora  ",
    "skinApiTemplate": "   "
  }
}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Panel.SkinEnabled {
		t.Fatal("expected skin to be disabled when template is empty")
	}
	if cfg.Panel.SkinName != "aurora" {
		t.Fatalf("expected trimmed skin name aurora, got %q", cfg.Panel.SkinName)
	}
	if cfg.Panel.SkinAPITemplate != "" {
		t.Fatalf("expected trimmed empty skin template, got %q", cfg.Panel.SkinAPITemplate)
	}
}

func TestLoadConfigMigratesClassicThemeToDarkMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aria2mx.json")
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
	if cfg.Panel.Theme != "aria2mx" {
		t.Fatalf("expected migrated theme aria2mx, got %q", cfg.Panel.Theme)
	}
	if cfg.Panel.ColorMode != "system" {
		t.Fatalf("expected migrated color mode system, got %q", cfg.Panel.ColorMode)
	}
}

func TestLoadConfigKeepsSystemColorMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aria2mx.json")
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
    "theme": "aria2mx",
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
	path := filepath.Join(t.TempDir(), "aria2mx.json")
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
    "theme": "aria2mx",
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
	path := filepath.Join(t.TempDir(), "aria2mx.json")
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
    "theme": "aria2mx",
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

func TestLoadConfigDropsUnsupportedTrackerSubscriptionSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aria2mx.json")
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
    "theme": "aria2mx",
    "colorMode": "light",
    "trackerSubscriptionEnabled": true,
    "trackerSubscriptionSource": "invalid-source"
  }
}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Panel.TrackerSubscriptionEnabled {
		t.Fatal("expected unsupported tracker subscription to be disabled")
	}
	if cfg.Panel.TrackerSubscriptionSource != "" {
		t.Fatalf("expected tracker subscription source cleared, got %q", cfg.Panel.TrackerSubscriptionSource)
	}
}

func TestLoadConfigKeepsRPCOriginSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aria2mx.json")
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
    "rpcOriginCheckMode": "whitelist",
    "rpcOriginWhitelist": ["ariang.example.com", "https://panel.example.com", "ariang.example.com", "  "],
    "mcpEnabled": true,
    "theme": "aria2mx",
    "colorMode": "light"
  }
}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Panel.RPCOriginCheckMode != panelRPCOriginModeWhitelist {
		t.Fatalf("expected whitelist origin mode, got %q", cfg.Panel.RPCOriginCheckMode)
	}
	if len(cfg.Panel.RPCOriginWhitelist) != 2 {
		t.Fatalf("expected normalized whitelist length 2, got %#v", cfg.Panel.RPCOriginWhitelist)
	}
	if cfg.Panel.RPCOriginWhitelist[0] != "ariang.example.com" || cfg.Panel.RPCOriginWhitelist[1] != "panel.example.com" {
		t.Fatalf("unexpected normalized whitelist %#v", cfg.Panel.RPCOriginWhitelist)
	}
}

func TestLoadConfigKeepsPeerGuardSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aria2mx.json")
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
    "theme": "aria2mx",
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
