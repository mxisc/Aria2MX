package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	Managed           bool              `json:"managed"`
	RPCURL            string            `json:"rpcUrl"`
	RPCSecret         string            `json:"rpcSecret"`
	ManagedRPCPort    int               `json:"managedRpcPort"`
	ManagedStateDir   string            `json:"managedStateDir,omitempty"`
	ManagedBinaryPath string            `json:"managedBinaryPath,omitempty"`
	Options           map[string]string `json:"options,omitempty"`
}

type PanelConfig struct {
	RefreshIntervalMs  int    `json:"refreshIntervalMs"`
	DefaultDownloadDir string `json:"defaultDownloadDir"`
	SessionTTLSeconds  int    `json:"sessionTTLSeconds"`
	Theme              string `json:"theme"`
}

const defaultManagedRPCPort = 16800

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		var cfg Config
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("parse config: %w", err)
		}
		mutated, err := ensureManagedConfigDefaults(&cfg)
		if err != nil {
			return nil, err
		}
		normalizeConfig(&cfg)
		if mutated {
			if err := SaveConfig(path, &cfg); err != nil {
				return nil, err
			}
		}
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
			Managed:        true,
			ManagedRPCPort: defaultManagedRPCPort,
		},
		Panel: PanelConfig{
			RefreshIntervalMs: 1500,
			SessionTTLSeconds: 86400,
			Theme:             "design",
		},
	}
	if rpcURL := strings.TrimSpace(os.Getenv("ARIAMX_ARIA2_RPC")); rpcURL != "" {
		cfg.Aria2.Managed = false
		cfg.Aria2.RPCURL = rpcURL
	}
	if secret := strings.TrimSpace(os.Getenv("ARIAMX_ARIA2_SECRET")); secret != "" {
		cfg.Aria2.RPCSecret = secret
	}
	if cfg.Aria2.Managed {
		if cfg.Aria2.RPCSecret == "" {
			secret, err := randomHex(24)
			if err != nil {
				return nil, err
			}
			cfg.Aria2.RPCSecret = secret
		}
		cfg.Aria2.RPCURL = managedRPCURL(cfg.Aria2.ManagedRPCPort)
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
	if cfg.Aria2.ManagedRPCPort <= 0 {
		cfg.Aria2.ManagedRPCPort = defaultManagedRPCPort
	}
	if cfg.Aria2.Managed {
		cfg.Aria2.RPCURL = managedRPCURL(cfg.Aria2.ManagedRPCPort)
	} else if cfg.Aria2.RPCURL == "" {
		cfg.Aria2.RPCURL = "http://127.0.0.1:6800/jsonrpc"
	}
	if cfg.Panel.RefreshIntervalMs <= 0 {
		cfg.Panel.RefreshIntervalMs = 1500
	}
	if cfg.Panel.SessionTTLSeconds <= 0 {
		cfg.Panel.SessionTTLSeconds = 86400
	}
	if cfg.Panel.Theme != "classic" && cfg.Panel.Theme != "design" {
		cfg.Panel.Theme = "design"
	}
}

func ensureManagedConfigDefaults(cfg *Config) (bool, error) {
	mutated := false
	if !cfg.Aria2.Managed && (cfg.Aria2.RPCURL == "" || cfg.Aria2.RPCURL == "http://127.0.0.1:6800/jsonrpc") {
		cfg.Aria2.Managed = true
		mutated = true
	}
	if cfg.Aria2.ManagedRPCPort <= 0 {
		cfg.Aria2.ManagedRPCPort = defaultManagedRPCPort
		mutated = true
	}
	if cfg.Aria2.Managed {
		if cfg.Aria2.Options != nil {
			if portText, ok := cfg.Aria2.Options["rpc-listen-port"]; ok {
				if port, err := strconv.Atoi(strings.TrimSpace(portText)); err == nil && port > 0 && port <= 65535 {
					if cfg.Aria2.ManagedRPCPort != port {
						cfg.Aria2.ManagedRPCPort = port
						mutated = true
					}
				}
				delete(cfg.Aria2.Options, "rpc-listen-port")
				mutated = true
			}
			for _, controlledKey := range []string{"enable-rpc", "rpc-listen-all", "rpc-save-upload-metadata", "rpc-secure", "rpc-allow-origin-all"} {
				if _, ok := cfg.Aria2.Options[controlledKey]; ok {
					delete(cfg.Aria2.Options, controlledKey)
					mutated = true
				}
			}
		}
		expectedURL := managedRPCURL(cfg.Aria2.ManagedRPCPort)
		if cfg.Aria2.RPCURL != expectedURL {
			cfg.Aria2.RPCURL = expectedURL
			mutated = true
		}
		if strings.TrimSpace(cfg.Aria2.RPCSecret) == "" {
			secret, err := randomHex(24)
			if err != nil {
				return false, err
			}
			cfg.Aria2.RPCSecret = secret
			mutated = true
		}
	}
	return mutated, nil
}

func managedRPCURL(port int) string {
	return fmt.Sprintf("http://127.0.0.1:%d/jsonrpc", port)
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
