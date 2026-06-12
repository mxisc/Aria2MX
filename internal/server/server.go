package server

import (
	"embed"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"
)

type Options struct {
	ConfigPath string
	Config     *Config
	Assets     embed.FS
}

type Server struct {
	configPath string
	cfg        *Config
	cfgMu      sync.RWMutex
	sessions   *SessionStore
	aria2      *Aria2Client
	assets     embed.FS
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

func New(opts Options) *Server {
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
	return s
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/login", s.handleLogin)
	mux.HandleFunc("/api/auth/logout", s.withAuth(s.handleLogout))
	mux.HandleFunc("/api/auth/me", s.withAuth(s.handleMe))
	mux.HandleFunc("/api/config", s.withAuth(s.handleConfig))
	mux.HandleFunc("/api/aria2/call", s.withAuth(s.handleAria2Call))
	mux.HandleFunc("/api/aria2/upload-torrent", s.withAuth(s.handleTorrentUpload))
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/", s.handleStatic)
	return securityHeaders(mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, apiResponse{OK: true, Data: map[string]string{"status": "ok"}})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var payload struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &payload); err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad_request", "请检查登录信息后重试。")
		return
	}
	s.cfgMu.RLock()
	admin := s.cfg.Admin
	ttl := time.Duration(s.cfg.Panel.SessionTTLSeconds) * time.Second
	s.cfgMu.RUnlock()

	if payload.Username != admin.Username || !VerifyPassword(payload.Password, admin.PasswordSalt, admin.PasswordHash) {
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

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.cfgMu.RLock()
		cfg := s.cfg
		data := map[string]interface{}{
			"aria2RpcUrl":        cfg.Aria2.RPCURL,
			"hasAria2Secret":     cfg.Aria2.RPCSecret != "",
			"refreshIntervalMs":  cfg.Panel.RefreshIntervalMs,
			"defaultDownloadDir": cfg.Panel.DefaultDownloadDir,
		}
		s.cfgMu.RUnlock()
		writeJSON(w, http.StatusOK, apiResponse{OK: true, Data: data})
	case http.MethodPut:
		var payload struct {
			Aria2RPCURL        *string `json:"aria2RpcUrl"`
			Aria2Secret        *string `json:"aria2Secret"`
			RefreshIntervalMs  *int    `json:"refreshIntervalMs"`
			DefaultDownloadDir *string `json:"defaultDownloadDir"`
			NewPassword        *string `json:"newPassword"`
		}
		if err := readJSON(r, &payload); err != nil {
			writeAPIError(w, http.StatusBadRequest, "bad_request", "请检查设置内容后重试。")
			return
		}
		s.cfgMu.Lock()
		if payload.Aria2RPCURL != nil && *payload.Aria2RPCURL != "" {
			s.cfg.Aria2.RPCURL = *payload.Aria2RPCURL
		}
		if payload.Aria2Secret != nil {
			s.cfg.Aria2.RPCSecret = *payload.Aria2Secret
		}
		if payload.RefreshIntervalMs != nil && *payload.RefreshIntervalMs >= 500 {
			s.cfg.Panel.RefreshIntervalMs = *payload.RefreshIntervalMs
		}
		if payload.DefaultDownloadDir != nil {
			s.cfg.Panel.DefaultDownloadDir = *payload.DefaultDownloadDir
		}
		if payload.NewPassword != nil && len(*payload.NewPassword) >= 6 {
			salt, err := randomHex(16)
			if err != nil {
				s.cfgMu.Unlock()
				writeAPIError(w, http.StatusInternalServerError, "save_failed", "设置暂时无法保存，请稍后重试。")
				return
			}
			s.cfg.Admin.PasswordSalt = salt
			s.cfg.Admin.PasswordHash = HashPassword(*payload.NewPassword, salt)
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
		writeAPIError(w, http.StatusBadGateway, "aria2_failed", "aria2 暂时不可用，请检查连接设置。")
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
	result, err := s.aria2.AddTorrent(file)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, "aria2_failed", "种子任务创建失败，请检查 aria2 连接设置。")
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{OK: true, Data: result})
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
