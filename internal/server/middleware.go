package server

import (
	"context"
	"net/http"
)

type sessionContextKey struct{}

func (s *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			writeAPIError(w, http.StatusUnauthorized, "unauthorized", "请先登录。")
			return
		}
		current, ok := s.sessions.Get(cookie.Value)
		if !ok {
			clearSessionCookie(w, r.TLS != nil)
			writeAPIError(w, http.StatusUnauthorized, "unauthorized", "登录状态已过期，请重新登录。")
			return
		}
		ctx := context.WithValue(r.Context(), sessionContextKey{}, current)
		next(w, r.WithContext(ctx))
	}
}
