package server

import (
	"testing"
	"time"
)

func TestPasswordHashAndVerify(t *testing.T) {
	salt := "test-salt"
	passwordSHA256 := SHA256Hex("secret-pass")
	hash := HashPasswordFromClientSHA256(passwordSHA256, salt)
	if hash == "" {
		t.Fatal("expected hash")
	}
	if !VerifyPassword(passwordSHA256, salt, hash, passwordSchemeClientSHA256PBKDF2) {
		t.Fatal("expected client sha256 password to verify")
	}
	if VerifyPassword(SHA256Hex("wrong-pass"), salt, hash, passwordSchemeClientSHA256PBKDF2) {
		t.Fatal("wrong client sha256 password verified")
	}
}

func TestSessionStoreExpiry(t *testing.T) {
	store := NewSessionStore()
	token, _, err := store.Create("admin", time.Millisecond)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, ok := store.Get(token); !ok {
		t.Fatal("expected fresh session")
	}
	time.Sleep(2 * time.Millisecond)
	if _, ok := store.Get(token); ok {
		t.Fatal("expected expired session to be rejected")
	}
}
