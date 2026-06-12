package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	Admin AdminConfig `json:"admin"`
	Aria2 Aria2Config `json:"aria2"`
	Panel PanelConfig `json:"panel"`
}

type AdminConfig struct {
	Username     string `json:"username"`
	PasswordHash string `json:"passwordHash"`
	PasswordSalt string `json:"passwordSalt"`
}

type Aria2Config struct {
	RPCURL    string `json:"rpcUrl"`
	RPCSecret string `json:"rpcSecret"`
}

type PanelConfig struct {
	RefreshIntervalMs  int    `json:"refreshIntervalMs"`
	DefaultDownloadDir string `json:"defaultDownloadDir"`
	SessionTTLSeconds  int    `json:"sessionTTLSeconds"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		var cfg Config
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("parse config: %w", err)
		}
		normalizeConfig(&cfg)
		return &cfg, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	password := os.Getenv("ARIAMX_ADMIN_PASSWORD")
	if password == "" {
		password = "admin"
	}
	salt, err := randomHex(16)
	if err != nil {
		return nil, err
	}
	cfg := &Config{
		Admin: AdminConfig{
			Username:     getenvDefault("ARIAMX_ADMIN_USER", "admin"),
			PasswordSalt: salt,
			PasswordHash: HashPassword(password, salt),
		},
		Aria2: Aria2Config{
			RPCURL: getenvDefault("ARIAMX_ARIA2_RPC", "http://127.0.0.1:6800/jsonrpc"),
		},
		Panel: PanelConfig{
			RefreshIntervalMs: 1500,
			SessionTTLSeconds: 86400,
		},
	}
	if secret := os.Getenv("ARIAMX_ARIA2_SECRET"); secret != "" {
		cfg.Aria2.RPCSecret = secret
	}
	if err := SaveConfig(path, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func SaveConfig(path string, cfg *Config) error {
	normalizeConfig(cfg)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func normalizeConfig(cfg *Config) {
	if cfg.Admin.Username == "" {
		cfg.Admin.Username = "admin"
	}
	if cfg.Aria2.RPCURL == "" {
		cfg.Aria2.RPCURL = "http://127.0.0.1:6800/jsonrpc"
	}
	if cfg.Panel.RefreshIntervalMs <= 0 {
		cfg.Panel.RefreshIntervalMs = 1500
	}
	if cfg.Panel.SessionTTLSeconds <= 0 {
		cfg.Panel.SessionTTLSeconds = 86400
	}
}

func randomHex(size int) (string, error) {
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func getenvDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
