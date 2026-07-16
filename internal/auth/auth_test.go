package auth

import (
	"testing"
	"time"
)

func TestTokenRoundtrip(t *testing.T) {
	secret := []byte("s3cret")
	tok := NewToken(secret, time.Now().Add(time.Hour))
	if !ValidateToken(secret, tok) {
		t.Fatal("valid token rejected")
	}
}

func TestTokenExpired(t *testing.T) {
	secret := []byte("s3cret")
	tok := NewToken(secret, time.Now().Add(-time.Minute))
	if ValidateToken(secret, tok) {
		t.Fatal("expired token accepted")
	}
}

func TestTokenTampered(t *testing.T) {
	secret := []byte("s3cret")
	tok := NewToken(secret, time.Now().Add(time.Hour))
	if ValidateToken(secret, tok+"x") || ValidateToken([]byte("other"), tok) || ValidateToken(secret, "garbage") {
		t.Fatal("bad token accepted")
	}
}
