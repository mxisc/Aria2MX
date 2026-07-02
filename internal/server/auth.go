package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"sync"
	"time"
)

const sessionCookieName = "aria2mx_session"
const (
	passwordSchemeClientSHA256PBKDF2 = "client_sha256_pbkdf2"
)

type SessionStore struct {
	mu       sync.Mutex
	sessions map[string]session
}

type session struct {
	Username  string
	ExpiresAt time.Time
}

func NewSessionStore() *SessionStore {
	return &SessionStore{sessions: make(map[string]session)}
}

func (s *SessionStore) Create(username string, ttl time.Duration) (string, time.Time, error) {
	token, err := randomToken()
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := time.Now().Add(ttl)
	s.mu.Lock()
	s.sessions[token] = session{Username: username, ExpiresAt: expiresAt}
	s.mu.Unlock()
	return token, expiresAt, nil
}

func (s *SessionStore) Get(token string) (session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.sessions[token]
	if !ok {
		return session{}, false
	}
	if time.Now().After(current.ExpiresAt) {
		delete(s.sessions, token)
		return session{}, false
	}
	return current, true
}

func (s *SessionStore) Delete(token string) {
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
}

func HashPassword(password, salt string) string {
	key := pbkdf2SHA256([]byte(password), []byte(salt), 120000, 32)
	return hex.EncodeToString(key)
}

func HashPasswordFromClientSHA256(passwordSHA256, salt string) string {
	return HashPassword(passwordSHA256, salt)
}

func SHA256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func HashPasswordFromRaw(password, salt string) string {
	return HashPasswordFromClientSHA256(SHA256Hex(password), salt)
}

func IsSHA256Hex(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func VerifyPassword(password, salt, expected, scheme string) bool {
	actual := HashPasswordByScheme(password, salt, scheme)
	return subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) == 1
}

func HashPasswordByScheme(password, salt, scheme string) string {
	switch scheme {
	case "", passwordSchemeClientSHA256PBKDF2:
		return HashPasswordFromClientSHA256(password, salt)
	default:
		return ""
	}
}

func setSessionCookie(w http.ResponseWriter, token string, expiresAt time.Time, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	})
}

func clearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	})
}

func randomToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func pbkdf2SHA256(password, salt []byte, iterations, keyLen int) []byte {
	hashLen := sha256.Size
	numBlocks := (keyLen + hashLen - 1) / hashLen
	var out []byte
	for block := 1; block <= numBlocks; block++ {
		u := pbkdf2F(password, salt, iterations, block)
		out = append(out, u...)
	}
	return out[:keyLen]
}

func pbkdf2F(password, salt []byte, iterations, block int) []byte {
	mac := hmac.New(sha256.New, password)
	mac.Write(salt)
	mac.Write([]byte{byte(block >> 24), byte(block >> 16), byte(block >> 8), byte(block)})
	u := mac.Sum(nil)
	out := make([]byte, len(u))
	copy(out, u)
	for i := 1; i < iterations; i++ {
		mac = hmac.New(sha256.New, password)
		mac.Write(u)
		u = mac.Sum(nil)
		for j := range out {
			out[j] ^= u[j]
		}
	}
	return out
}
