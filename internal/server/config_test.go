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
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected config file: %v", err)
	}
	if !VerifyPassword("change-me", cfg.Admin.PasswordSalt, cfg.Admin.PasswordHash) {
		t.Fatal("expected configured password to verify")
	}
}
