package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Admin     AdminConfig     `json:"admin"`
	Aria2     Aria2Config     `json:"aria2"`
	Panel     PanelConfig     `json:"panel"`
	PeerGuard PeerGuardConfig `json:"peerGuard,omitempty"`
}

type AdminConfig struct {
	Username       string `json:"username"`
	PasswordHash   string `json:"passwordHash"`
	PasswordSalt   string `json:"passwordSalt"`
	PasswordScheme string `json:"passwordScheme,omitempty"`
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
	RefreshIntervalMs          int      `json:"refreshIntervalMs"`
	DefaultDownloadDir         string   `json:"defaultDownloadDir"`
	SessionTTLSeconds          int      `json:"sessionTTLSeconds"`
	RPCSecret                  string   `json:"rpcSecret,omitempty"`
	RPCOriginCheckMode         string   `json:"rpcOriginCheckMode,omitempty"`
	RPCOriginWhitelist         []string `json:"rpcOriginWhitelist,omitempty"`
	TrackerSubscriptionEnabled bool     `json:"trackerSubscriptionEnabled,omitempty"`
	TrackerSubscriptionSource  string   `json:"trackerSubscriptionSource,omitempty"`
	MCPEnabled                 bool     `json:"mcpEnabled"`
	Theme                      string   `json:"theme"`
	ColorMode                  string   `json:"colorMode,omitempty"`
	SkinEnabled                bool     `json:"skinEnabled,omitempty"`
	SkinName                   string   `json:"skinName,omitempty"`
	SkinAPITemplate            string   `json:"skinApiTemplate,omitempty"`
}

type PeerGuardConfig struct {
	AutoBanEnabled  bool            `json:"autoBanEnabled"`
	AutoBanMinScore int             `json:"autoBanMinScore,omitempty"`
	BlockedPeers    []PeerBanRecord `json:"blockedPeers,omitempty"`
}

type PeerBanRecord struct {
	IP        string `json:"ip"`
	Reason    string `json:"reason,omitempty"`
	CreatedAt string `json:"createdAt,omitempty"`
	ExpiresAt string `json:"expiresAt,omitempty"`
}

const defaultManagedRPCPort = 16800

const (
	panelRPCOriginModeDisabled   = "disabled"
	panelRPCOriginModeSameOrigin = "same_origin"
	panelRPCOriginModeWhitelist  = "whitelist"
)

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		var cfg Config
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("parse config: %w", err)
		}
		if !panelConfigHasField(data, "mcpEnabled") {
			cfg.Panel.MCPEnabled = true
		}
		mutated, err := ensureManagedConfigDefaults(&cfg)
		if err != nil {
			return nil, err
		}
		mutated = normalizeConfig(&cfg) || mutated
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

	password := os.Getenv("ARIA2MX_ADMIN_PASSWORD")
	if password == "" {
		password = "admin"
	}
	salt, err := randomHex(16)
	if err != nil {
		return nil, err
	}
	cfg := &Config{
		Admin: AdminConfig{
			Username:       getenvDefault("ARIA2MX_ADMIN_USER", "admin"),
			PasswordSalt:   salt,
			PasswordHash:   HashPasswordFromRaw(password, salt),
			PasswordScheme: passwordSchemeClientSHA256PBKDF2,
		},
		Aria2: Aria2Config{
			Managed:        true,
			ManagedRPCPort: defaultManagedRPCPort,
		},
		Panel: PanelConfig{
			RefreshIntervalMs:  1500,
			SessionTTLSeconds:  86400,
			RPCOriginCheckMode: panelRPCOriginModeSameOrigin,
			MCPEnabled:         true,
			Theme:              "aria2mx",
			ColorMode:          "system",
			SkinName:           "default",
		},
	}
	if rpcURL := strings.TrimSpace(os.Getenv("ARIA2MX_ARIA2_RPC")); rpcURL != "" {
		cfg.Aria2.Managed = false
		cfg.Aria2.RPCURL = rpcURL
	}
	if secret := strings.TrimSpace(os.Getenv("ARIA2MX_ARIA2_SECRET")); secret != "" {
		cfg.Aria2.RPCSecret = secret
	}
	if secret := strings.TrimSpace(os.Getenv("ARIA2MX_PANEL_RPC_SECRET")); secret != "" {
		cfg.Panel.RPCSecret = secret
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
	if strings.TrimSpace(cfg.Panel.RPCSecret) == "" || cfg.Panel.RPCSecret == cfg.Aria2.RPCSecret {
		secret, err := randomHex(24)
		if err != nil {
			return nil, err
		}
		for secret == cfg.Aria2.RPCSecret {
			secret, err = randomHex(24)
			if err != nil {
				return nil, err
			}
		}
		cfg.Panel.RPCSecret = secret
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

func normalizeConfig(cfg *Config) bool {
	mutated := false
	if cfg.Admin.Username == "" {
		cfg.Admin.Username = "admin"
		mutated = true
	}
	if cfg.Admin.PasswordScheme != passwordSchemeClientSHA256PBKDF2 {
		cfg.Admin.PasswordScheme = passwordSchemeClientSHA256PBKDF2
		mutated = true
	}
	if cfg.Aria2.ManagedRPCPort <= 0 {
		cfg.Aria2.ManagedRPCPort = defaultManagedRPCPort
		mutated = true
	}
	if cfg.Aria2.Managed {
		rpcURL := managedRPCURL(cfg.Aria2.ManagedRPCPort)
		if cfg.Aria2.RPCURL != rpcURL {
			cfg.Aria2.RPCURL = rpcURL
			mutated = true
		}
	} else if cfg.Aria2.RPCURL == "" {
		cfg.Aria2.RPCURL = "http://127.0.0.1:6800/jsonrpc"
		mutated = true
	}
	if cfg.Panel.RefreshIntervalMs <= 0 {
		cfg.Panel.RefreshIntervalMs = 1500
		mutated = true
	}
	if cfg.Panel.SessionTTLSeconds <= 0 {
		cfg.Panel.SessionTTLSeconds = 86400
		mutated = true
	}
	if cfg.Panel.RPCOriginCheckMode != panelRPCOriginModeDisabled && cfg.Panel.RPCOriginCheckMode != panelRPCOriginModeSameOrigin && cfg.Panel.RPCOriginCheckMode != panelRPCOriginModeWhitelist {
		cfg.Panel.RPCOriginCheckMode = panelRPCOriginModeSameOrigin
		mutated = true
	}
	if cfg.Panel.TrackerSubscriptionSource != "" && !isSupportedTrackerSubscriptionSource(cfg.Panel.TrackerSubscriptionSource) {
		cfg.Panel.TrackerSubscriptionEnabled = false
		cfg.Panel.TrackerSubscriptionSource = ""
		mutated = true
	}
	normalizedOrigins := normalizePanelRPCOriginWhitelist(cfg.Panel.RPCOriginWhitelist)
	if len(normalizedOrigins) != len(cfg.Panel.RPCOriginWhitelist) {
		cfg.Panel.RPCOriginWhitelist = normalizedOrigins
		mutated = true
	} else {
		for index := range normalizedOrigins {
			if normalizedOrigins[index] != cfg.Panel.RPCOriginWhitelist[index] {
				cfg.Panel.RPCOriginWhitelist = normalizedOrigins
				mutated = true
				break
			}
		}
	}
	legacyTheme := strings.TrimSpace(cfg.Panel.Theme)
	if cfg.Panel.ColorMode != "light" && cfg.Panel.ColorMode != "dark" && cfg.Panel.ColorMode != "system" {
		cfg.Panel.ColorMode = "system"
		mutated = true
	}
	if legacyTheme != "aria2mx" {
		cfg.Panel.Theme = "aria2mx"
		mutated = true
	}
	skinName := strings.TrimSpace(cfg.Panel.SkinName)
	if skinName == "" {
		skinName = "default"
	}
	if cfg.Panel.SkinName != skinName {
		cfg.Panel.SkinName = skinName
		mutated = true
	}
	skinTemplate := strings.TrimSpace(cfg.Panel.SkinAPITemplate)
	if cfg.Panel.SkinAPITemplate != skinTemplate {
		cfg.Panel.SkinAPITemplate = skinTemplate
		mutated = true
	}
	if cfg.Panel.SkinEnabled && skinTemplate == "" {
		cfg.Panel.SkinEnabled = false
		mutated = true
	}
	normalizedPeers := normalizePeerBanRecords(cfg.PeerGuard.BlockedPeers)
	if len(normalizedPeers) != len(cfg.PeerGuard.BlockedPeers) {
		cfg.PeerGuard.BlockedPeers = normalizedPeers
		mutated = true
	} else {
		for index := range normalizedPeers {
			if normalizedPeers[index] != cfg.PeerGuard.BlockedPeers[index] {
				cfg.PeerGuard.BlockedPeers = normalizedPeers
				mutated = true
				break
			}
		}
	}
	if cfg.PeerGuard.AutoBanMinScore <= 0 {
		cfg.PeerGuard.AutoBanMinScore = 3
		mutated = true
	}
	return mutated
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
	if strings.TrimSpace(cfg.Panel.RPCSecret) == "" || cfg.Panel.RPCSecret == cfg.Aria2.RPCSecret {
		secret, err := randomHex(24)
		if err != nil {
			return false, err
		}
		for secret == cfg.Aria2.RPCSecret {
			secret, err = randomHex(24)
			if err != nil {
				return false, err
			}
		}
		cfg.Panel.RPCSecret = secret
		mutated = true
	}
	return mutated, nil
}

func normalizePeerBanRecords(records []PeerBanRecord) []PeerBanRecord {
	if len(records) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	normalized := make([]PeerBanRecord, 0, len(records))
	for _, record := range records {
		record.IP = strings.TrimSpace(record.IP)
		record.Reason = strings.TrimSpace(record.Reason)
		record.CreatedAt = strings.TrimSpace(record.CreatedAt)
		record.ExpiresAt = strings.TrimSpace(record.ExpiresAt)
		if record.IP == "" {
			continue
		}
		if _, ok := seen[record.IP]; ok {
			continue
		}
		seen[record.IP] = struct{}{}
		normalized = append(normalized, record)
	}
	return normalized
}

func normalizePanelRPCOriginWhitelist(origins []string) []string {
	if len(origins) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	normalized := make([]string, 0, len(origins))
	for _, origin := range origins {
		value := normalizePanelRPCOriginValue(origin)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	return normalized
}

func normalizePanelRPCOriginValue(origin string) string {
	value := strings.ToLower(strings.TrimSpace(origin))
	if value == "" {
		return ""
	}
	if parsed, err := url.Parse(value); err == nil && parsed.Host != "" {
		return strings.ToLower(strings.TrimSpace(parsed.Host))
	}
	value = strings.TrimSuffix(value, "/")
	if slash := strings.IndexByte(value, '/'); slash >= 0 {
		value = value[:slash]
	}
	return strings.TrimSpace(value)
}

func panelConfigHasField(data []byte, field string) bool {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return false
	}
	panelRaw, ok := raw["panel"]
	if !ok {
		return false
	}
	var panel map[string]json.RawMessage
	if err := json.Unmarshal(panelRaw, &panel); err != nil {
		return false
	}
	_, ok = panel[field]
	return ok
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
