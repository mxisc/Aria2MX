package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"ariamx/internal/version"
)

type Options struct {
	ConfigPath string
	Config     *Config
	Assets     embed.FS
}

type Server struct {
	configPath              string
	cfg                     *Config
	cfgMu                   sync.RWMutex
	sessions                *SessionStore
	aria2                   *Aria2Client
	managed                 *ManagedAria2
	assets                  embed.FS
	peerGuard               peerGuardRuntime
	peerGuardStop           chan struct{}
	peerGuardDone           chan struct{}
	trackerSubscriptionStop chan struct{}
	trackerSubscriptionDone chan struct{}
}

type apiResponse struct {
	OK    bool        `json:"ok"`
	Data  interface{} `json:"data,omitempty"`
	Error *apiError   `json:"error,omitempty"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func New(opts Options) (*Server, error) {
	s := &Server{
		configPath: opts.ConfigPath,
		cfg:        opts.Config,
		sessions:   NewSessionStore(),
		assets:     opts.Assets,
	}
	s.aria2 = NewAria2Client(func() Aria2Config {
		s.cfgMu.RLock()
		defer s.cfgMu.RUnlock()
		return s.cfg.Aria2
	})
	if s.cfg.Aria2.Managed {
		s.managed = NewManagedAria2(opts.ConfigPath, s.cfg, &s.cfgMu, s.aria2)
		if err := s.managed.Start(); err != nil {
			return nil, err
		}
	}
	s.syncTrackerSubscription()
	s.startTrackerSubscriptionLoop()
	s.startPeerGuardLoop()
	if len(s.cfg.PeerGuard.BlockedPeers) > 0 {
		_ = s.applyPeerGuardFirewall(nil, s.cfg.PeerGuard.BlockedPeers)
	}
	return s, nil
}

func (s *Server) Close() error {
	s.stopTrackerSubscriptionLoop()
	s.stopPeerGuardLoop()
	if s.managed == nil {
		return nil
	}
	return s.managed.Stop()
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/login", s.handleLogin)
	mux.HandleFunc("/api/auth/logout", s.withAuth(s.handleLogout))
	mux.HandleFunc("/api/auth/me", s.withAuth(s.handleMe))
	mux.HandleFunc("/api/about", s.withAuth(s.handleAbout))
	mux.HandleFunc("/api/panel-style", s.handlePanelStyle)
	mux.HandleFunc("/api/config", s.withAuth(s.handleConfig))
	mux.HandleFunc("/api/aria2/options", s.withAuth(s.handleAria2Options))
	mux.HandleFunc("/api/aria2/options/reset", s.withAuth(s.handleAria2OptionsReset))
	mux.HandleFunc("/api/aria2/call", s.withAuth(s.handleAria2Call))
	mux.HandleFunc("/api/aria2/restart", s.withAuth(s.handleAria2Restart))
	mux.HandleFunc("/api/aria2/remove", s.withAuth(s.handleAria2Remove))
	mux.HandleFunc("/api/aria2/upload-torrent", s.withAuth(s.handleTorrentUpload))
	mux.HandleFunc("/api/tracker-subscription", s.withAuth(s.handleTrackerSubscription))
	mux.HandleFunc("/api/scripts", s.withAuth(s.handleScriptHooks))
	mux.HandleFunc("/api/peer-guard", s.withAuth(s.handlePeerGuard))
	mux.HandleFunc("/api/peer-guard/ban", s.withAuth(s.handlePeerGuardBan))
	mux.HandleFunc("/api/peer-guard/unban", s.withAuth(s.handlePeerGuardUnban))
	mux.HandleFunc("/api/peer-guard/settings", s.withAuth(s.handlePeerGuardSettings))
	mux.HandleFunc("/jsonrpc", s.handlePanelRPC)
	mux.HandleFunc("/mcp", s.handleMCP)
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/", s.handleStatic)
	return securityHeaders(mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{"status": "ok"}
	if s.managed != nil {
		data["aria2Managed"] = true
	}
	writeJSON(w, http.StatusOK, apiResponse{OK: true, Data: data})
}

func (s *Server) handlePanelStyle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	s.cfgMu.RLock()
	data := map[string]interface{}{
		"theme":           s.cfg.Panel.Theme,
		"colorMode":       s.cfg.Panel.ColorMode,
		"skinEnabled":     s.cfg.Panel.SkinEnabled,
		"skinName":        s.cfg.Panel.SkinName,
		"skinApiTemplate": s.cfg.Panel.SkinAPITemplate,
	}
	s.cfgMu.RUnlock()
	writeJSON(w, http.StatusOK, apiResponse{OK: true, Data: data})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var payload struct {
		Username       string `json:"username"`
		PasswordSHA256 string `json:"passwordSha256"`
	}
	if err := readJSON(r, &payload); err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad_request", "请检查登录信息后重试。")
		return
	}
	s.cfgMu.RLock()
	admin := s.cfg.Admin
	ttl := time.Duration(s.cfg.Panel.SessionTTLSeconds) * time.Second
	s.cfgMu.RUnlock()

	passwordVerified := IsSHA256Hex(payload.PasswordSHA256) && VerifyPassword(payload.PasswordSHA256, admin.PasswordSalt, admin.PasswordHash, admin.PasswordScheme)

	if payload.Username != admin.Username || !passwordVerified {
		writeAPIError(w, http.StatusUnauthorized, "invalid_login", "用户名或密码不正确。")
		return
	}
	token, expiresAt, err := s.sessions.Create(admin.Username, ttl)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "session_failed", "登录暂时不可用，请稍后重试。")
		return
	}
	setSessionCookie(w, token, expiresAt, r.TLS != nil)
	writeJSON(w, http.StatusOK, apiResponse{OK: true, Data: map[string]interface{}{
		"username":         admin.Username,
		"sessionExpiresAt": expiresAt.Format(time.RFC3339),
	}})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		s.sessions.Delete(cookie.Value)
	}
	clearSessionCookie(w, r.TLS != nil)
	writeJSON(w, http.StatusOK, apiResponse{OK: true})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	current := r.Context().Value(sessionContextKey{}).(session)
	writeJSON(w, http.StatusOK, apiResponse{OK: true, Data: map[string]interface{}{
		"username":         current.Username,
		"sessionExpiresAt": current.ExpiresAt.Format(time.RFC3339),
	}})
}

func (s *Server) handleAbout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	aria2Version := ""
	if result, err := s.aria2.Call(Aria2CallRequest{Method: "aria2.getVersion"}); err == nil {
		if payload, ok := result.(map[string]interface{}); ok {
			if value, ok := payload["version"].(string); ok {
				aria2Version = value
			}
		}
	}
	httpURL, wsURL, mcpURL := publicPanelRPCURLs(r)
	s.cfgMu.RLock()
	panelRPCSecret := s.cfg.Panel.RPCSecret
	mcpEnabled := s.cfg.Panel.MCPEnabled
	s.cfgMu.RUnlock()
	if !mcpEnabled {
		mcpURL = ""
	}
	writeJSON(w, http.StatusOK, apiResponse{OK: true, Data: map[string]interface{}{
		"panelVersion":   version.PanelVersion,
		"aria2Version":   aria2Version,
		"rpcPath":        "/jsonrpc",
		"httpRpcUrl":     httpURL,
		"wsRpcUrl":       wsURL,
		"mcpHttpUrl":     mcpURL,
		"mcpEnabled":     mcpEnabled,
		"panelRpcSecret": panelRPCSecret,
	}})
}

func publicPanelRPCURLs(r *http.Request) (string, string, string) {
	host := r.Host
	if host == "" {
		host = "127.0.0.1"
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		scheme = strings.TrimSpace(parts[0])
	}
	wsScheme := "ws"
	if scheme == "https" {
		wsScheme = "wss"
	}
	return scheme + "://" + host + "/jsonrpc", wsScheme + "://" + host + "/jsonrpc", scheme + "://" + host + "/mcp"
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.cfgMu.RLock()
		cfg := s.cfg
		data := map[string]interface{}{
			"aria2RpcUrl":                cfg.Aria2.RPCURL,
			"hasAria2Secret":             cfg.Aria2.RPCSecret != "",
			"aria2Managed":               cfg.Aria2.Managed,
			"rpcOriginCheckMode":         cfg.Panel.RPCOriginCheckMode,
			"rpcOriginWhitelist":         append([]string{}, cfg.Panel.RPCOriginWhitelist...),
			"trackerSubscriptionEnabled": cfg.Panel.TrackerSubscriptionEnabled,
			"trackerSubscriptionSource":  cfg.Panel.TrackerSubscriptionSource,
			"mcpEnabled":                 cfg.Panel.MCPEnabled,
			"refreshIntervalMs":          cfg.Panel.RefreshIntervalMs,
			"defaultDownloadDir":         cfg.Panel.DefaultDownloadDir,
			"theme":                      cfg.Panel.Theme,
			"colorMode":                  cfg.Panel.ColorMode,
			"skinEnabled":                cfg.Panel.SkinEnabled,
			"skinName":                   cfg.Panel.SkinName,
			"skinApiTemplate":            cfg.Panel.SkinAPITemplate,
		}
		s.cfgMu.RUnlock()
		writeJSON(w, http.StatusOK, apiResponse{OK: true, Data: data})
	case http.MethodPut:
		var payload struct {
			Aria2RPCURL                *string   `json:"aria2RpcUrl"`
			Aria2Secret                *string   `json:"aria2Secret"`
			RefreshIntervalMs          *int      `json:"refreshIntervalMs"`
			DefaultDownloadDir         *string   `json:"defaultDownloadDir"`
			RPCOriginCheckMode         *string   `json:"rpcOriginCheckMode"`
			RPCOriginWhitelist         *[]string `json:"rpcOriginWhitelist"`
			TrackerSubscriptionEnabled *bool     `json:"trackerSubscriptionEnabled"`
			TrackerSubscriptionSource  *string   `json:"trackerSubscriptionSource"`
			MCPEnabled                 *bool     `json:"mcpEnabled"`
			Theme                      *string   `json:"theme"`
			ColorMode                  *string   `json:"colorMode"`
			SkinEnabled                *bool     `json:"skinEnabled"`
			SkinName                   *string   `json:"skinName"`
			SkinAPITemplate            *string   `json:"skinApiTemplate"`
			NewPasswordSHA256          *string   `json:"newPasswordSha256"`
		}
		if err := readJSON(r, &payload); err != nil {
			writeAPIError(w, http.StatusBadRequest, "bad_request", "请检查设置内容后重试。")
			return
		}
		s.cfgMu.Lock()
		if !s.cfg.Aria2.Managed {
			if payload.Aria2RPCURL != nil && *payload.Aria2RPCURL != "" {
				s.cfg.Aria2.RPCURL = *payload.Aria2RPCURL
			}
			if payload.Aria2Secret != nil {
				s.cfg.Aria2.RPCSecret = *payload.Aria2Secret
			}
		}
		if payload.RefreshIntervalMs != nil && *payload.RefreshIntervalMs >= 500 {
			s.cfg.Panel.RefreshIntervalMs = *payload.RefreshIntervalMs
		}
		if payload.DefaultDownloadDir != nil {
			s.cfg.Panel.DefaultDownloadDir = *payload.DefaultDownloadDir
		}
		if payload.RPCOriginCheckMode != nil {
			switch *payload.RPCOriginCheckMode {
			case panelRPCOriginModeDisabled, panelRPCOriginModeSameOrigin, panelRPCOriginModeWhitelist:
				s.cfg.Panel.RPCOriginCheckMode = *payload.RPCOriginCheckMode
			}
		}
		if payload.RPCOriginWhitelist != nil {
			s.cfg.Panel.RPCOriginWhitelist = append([]string(nil), (*payload.RPCOriginWhitelist)...)
		}
		if payload.TrackerSubscriptionEnabled != nil {
			s.cfg.Panel.TrackerSubscriptionEnabled = *payload.TrackerSubscriptionEnabled
		}
		if payload.TrackerSubscriptionSource != nil && isSupportedTrackerSubscriptionSource(*payload.TrackerSubscriptionSource) {
			s.cfg.Panel.TrackerSubscriptionSource = *payload.TrackerSubscriptionSource
		}
		if payload.MCPEnabled != nil {
			s.cfg.Panel.MCPEnabled = *payload.MCPEnabled
		}
		if payload.Theme != nil && *payload.Theme == "ariamx" {
			s.cfg.Panel.Theme = *payload.Theme
		}
		if payload.ColorMode != nil && (*payload.ColorMode == "system" || *payload.ColorMode == "light" || *payload.ColorMode == "dark") {
			s.cfg.Panel.ColorMode = *payload.ColorMode
		}
		if payload.SkinName != nil {
			s.cfg.Panel.SkinName = strings.TrimSpace(*payload.SkinName)
		}
		if payload.SkinAPITemplate != nil {
			s.cfg.Panel.SkinAPITemplate = strings.TrimSpace(*payload.SkinAPITemplate)
		}
		if payload.SkinEnabled != nil {
			s.cfg.Panel.SkinEnabled = *payload.SkinEnabled
		}
		if payload.NewPasswordSHA256 != nil && IsSHA256Hex(*payload.NewPasswordSHA256) {
			salt, err := randomHex(16)
			if err != nil {
				s.cfgMu.Unlock()
				writeAPIError(w, http.StatusInternalServerError, "save_failed", "设置暂时无法保存，请稍后重试。")
				return
			}
			s.cfg.Admin.PasswordSalt = salt
			s.cfg.Admin.PasswordHash = HashPasswordFromClientSHA256(*payload.NewPasswordSHA256, salt)
			s.cfg.Admin.PasswordScheme = passwordSchemeClientSHA256PBKDF2
		}
		err := SaveConfig(s.configPath, s.cfg)
		s.cfgMu.Unlock()
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "save_failed", "设置暂时无法保存，请稍后重试。")
			return
		}
		writeJSON(w, http.StatusOK, apiResponse{OK: true})
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleAria2Options(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if s.managed == nil {
		writeAPIError(w, http.StatusBadRequest, "managed_disabled", "当前 aria2 不是由面板托管，无法保存内置全局选项。")
		return
	}

	var payload struct {
		Patch map[string]string `json:"patch"`
	}
	if err := readJSON(r, &payload); err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad_request", "请检查选项内容后重试。")
		return
	}
	if err := validateManagedAria2Patch(payload.Patch, s.cfg.Panel.TrackerSubscriptionEnabled); err != nil {
		writeAPIError(w, http.StatusBadRequest, "managed_locked", err.Error())
		return
	}

	result, err := s.managed.SaveOptions(payload.Patch)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, "aria2_save_failed", "aria2 选项保存失败，请稍后重试。")
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{OK: true, Data: result})
}

func validateManagedAria2Patch(patch map[string]string, trackerSubscriptionEnabled bool) error {
	if !trackerSubscriptionEnabled {
		return nil
	}
	if _, ok := patch["bt-tracker"]; ok {
		return fmt.Errorf("节点订阅开启时，BT Tracker 服务器由订阅源自动维护，请先关闭节点订阅后再修改。")
	}
	return nil
}

func (s *Server) handleAria2OptionsReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if s.managed == nil {
		writeAPIError(w, http.StatusBadRequest, "managed_disabled", "当前 aria2 不是由面板托管，无法重置内置全局选项。")
		return
	}

	result, err := s.managed.ResetOptions()
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, "aria2_reset_failed", "aria2 选项重置失败，请稍后重试。")
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{OK: true, Data: result})
}

func (s *Server) handleAria2Call(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var payload Aria2CallRequest
	if err := readJSON(r, &payload); err != nil || payload.Method == "" {
		writeAPIError(w, http.StatusBadRequest, "bad_request", "请检查任务请求后重试。")
		return
	}
	result, err := s.aria2.Call(payload)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, "aria2_failed", userFacingAria2Error(err))
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{OK: true, Data: result})
}

func (s *Server) handleAria2Restart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var payload struct {
		GID string `json:"gid"`
	}
	if err := readJSON(r, &payload); err != nil || payload.GID == "" {
		writeAPIError(w, http.StatusBadRequest, "bad_request", "请检查任务后重试。")
		return
	}
	newGID, err := s.restartTask(payload.GID)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, "aria2_restart_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{OK: true, Data: map[string]string{"gid": newGID}})
}

func (s *Server) handleAria2Remove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var payload struct {
		GID string `json:"gid"`
	}
	if err := readJSON(r, &payload); err != nil || payload.GID == "" {
		writeAPIError(w, http.StatusBadRequest, "bad_request", "请检查任务后重试。")
		return
	}
	result, err := s.removeTask(payload.GID)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, "aria2_remove_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{OK: true, Data: result})
}

func (s *Server) handleTorrentUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if err := r.ParseMultipartForm(40 << 20); err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad_upload", "请重新选择种子文件后上传。")
		return
	}
	file, _, err := r.FormFile("torrent")
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad_upload", "请重新选择种子文件后上传。")
		return
	}
	defer file.Close()
	options := torrentOptionsFromForm(r.MultipartForm.Value)
	result, err := s.aria2.AddTorrent(file, options)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, "aria2_failed", "种子任务创建失败，请检查 aria2 连接设置。")
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{OK: true, Data: result})
}

func torrentOptionsFromForm(values map[string][]string) map[string]string {
	options := map[string]string{}
	copyFormValue(options, values, "dir", "dir")
	copyFormValue(options, values, "out", "out")
	copyFormValue(options, values, "split", "split")
	copyFormValue(options, values, "max-connection-per-server", "maxConnectionPerServer")
	copyFormValue(options, values, "max-download-limit", "downloadLimit")
	copyFormValue(options, values, "seed-ratio", "seedRatio")
	copyFormValue(options, values, "seed-time", "seedTime")

	headerLines := values["header"]
	headers := make([]string, 0, len(headerLines))
	for _, header := range headerLines {
		header = strings.TrimSpace(header)
		if header != "" {
			headers = append(headers, header)
		}
	}
	if len(headers) > 0 {
		options["header"] = strings.Join(headers, "\n")
	}
	return options
}

func copyFormValue(target map[string]string, values map[string][]string, optionKey, formKey string) {
	formValues := values[formKey]
	if len(formValues) == 0 {
		return
	}
	value := strings.TrimSpace(formValues[0])
	if value == "" {
		return
	}
	target[optionKey] = value
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		http.NotFound(w, r)
		return
	}
	dist, err := fs.Sub(s.assets, "dist")
	if err != nil {
		http.Error(w, "frontend assets are not built", http.StatusServiceUnavailable)
		return
	}
	requestPath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if requestPath == "." || requestPath == "" {
		requestPath = "index.html"
	}
	if _, err := fs.Stat(dist, requestPath); err != nil {
		requestPath = "index.html"
	}
	http.ServeFileFS(w, r, dist, requestPath)
}

func readJSON(r *http.Request, target interface{}) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func writeJSON(w http.ResponseWriter, status int, payload apiResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, apiResponse{OK: false, Error: &apiError{Code: code, Message: message}})
}

func methodNotAllowed(w http.ResponseWriter) {
	writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不可用。")
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}
