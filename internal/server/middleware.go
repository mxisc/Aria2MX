package server

import (
	"context"
	"net/http"
	"strings"
)

type sessionContextKey struct{}

func (s *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		current, ok, cookieMissing := s.authenticatedSession(r)
		if !ok {
			if !cookieMissing {
				clearSessionCookie(w, r.TLS != nil)
				writeAPIError(w, http.StatusUnauthorized, "unauthorized", "登录状态已过期，请重新登录。")
				return
			}
			writeAPIError(w, http.StatusUnauthorized, "unauthorized", "请先登录。")
			return
		}
		ctx := context.WithValue(r.Context(), sessionContextKey{}, current)
		next(w, r.WithContext(ctx))
	}
}

func (s *Server) authenticatedSession(r *http.Request) (session, bool, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return session{}, false, true
	}
	current, ok := s.sessions.Get(cookie.Value)
	return current, ok, false
}

func (s *Server) matchesPanelRPCSecret(r *http.Request) bool {
	for _, candidate := range []string{
		extractBearerToken(r.Header.Get("Authorization")),
		strings.TrimSpace(r.Header.Get("X-Aria2MX-Secret")),
		strings.TrimSpace(r.URL.Query().Get("secret")),
	} {
		if candidate != "" && s.matchesPanelRPCSecretValue(candidate) {
			return true
		}
	}
	return false
}

func (s *Server) matchesPanelRPCSecretValue(candidate string) bool {
	s.cfgMu.RLock()
	expected := s.cfg.Panel.RPCSecret
	s.cfgMu.RUnlock()
	return candidate != "" && expected != "" && candidate == expected
}

func extractBearerToken(value string) string {
	parts := strings.SplitN(strings.TrimSpace(value), " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
