package server

import (
	"testing"
	"time"
)

func TestPasswordHashAndVerify(t *testing.T) {
	salt := "test-salt"
	hash := HashPassword("secret-pass", salt)
	if hash == "" {
		t.Fatal("expected hash")
	}
	if !VerifyPassword("secret-pass", salt, hash) {
		t.Fatal("expected password to verify")
	}
	if VerifyPassword("wrong-pass", salt, hash) {
		t.Fatal("wrong password verified")
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
