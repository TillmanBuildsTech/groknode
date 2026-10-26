package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestNewPKCE(t *testing.T) {
	p, err := NewPKCE()
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(p.Verifier))
	want := base64.RawURLEncoding.EncodeToString(sum[:])
	if p.Challenge != want {
		t.Fatalf("challenge mismatch: got %q want %q", p.Challenge, want)
	}
	if p.Verifier == "" || p.Challenge == p.Verifier {
		t.Fatal("degenerate PKCE pair")
	}
}

func makeJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	body, _ := json.Marshal(claims)
	seg := func(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
	return seg([]byte(`{"alg":"none"}`)) + "." + seg(body) + "." + seg([]byte("sig"))
}

func TestJWTPayloadAndExpiry(t *testing.T) {
	tok := makeJWT(t, map[string]any{"sub": "user_42", "exp": float64(2_000)})
	claims, err := JWTPayload(tok)
	if err != nil {
		t.Fatal(err)
	}
	if claims["sub"] != "user_42" {
		t.Fatalf("sub = %v", claims["sub"])
	}
	// exp = 2000s => 2_000_000 ms. now=1_000_000ms, margin=500_000ms => not soon.
	if ExpiresSoon(tok, 1_000_000, 500_000) {
		t.Fatal("should not expire soon")
	}
	// now=1_900_000ms, margin=500_000ms => soon.
	if !ExpiresSoon(tok, 1_900_000, 500_000) {
		t.Fatal("should expire soon")
	}
	// Unreadable token => treat as expiring.
	if !ExpiresSoon("garbage", 0, 1) {
		t.Fatal("garbage token should be treated as expiring")
	}
}

func TestAccountScopeStable(t *testing.T) {
	tok := makeJWT(t, map[string]any{"sub": "abc"})
	if got := AccountScope(tok); len(got) != 64 {
		t.Fatalf("expected 64 hex chars, got %d (%q)", len(got), got)
	}
}
