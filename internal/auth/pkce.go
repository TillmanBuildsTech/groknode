// Package auth implements the PKCE browser-link login and token handling for
// the Grok Bot / Cursor backend.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// PKCE holds a verifier/challenge pair for an OAuth authorization.
type PKCE struct {
	Verifier  string
	Challenge string
}

// NewPKCE generates a fresh PKCE pair matching the reference client:
// verifier = base64url(32 random bytes), challenge = base64url(sha256(verifier)).
func NewPKCE() (PKCE, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return PKCE{}, fmt.Errorf("random: %w", err)
	}
	verifier := base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	return PKCE{Verifier: verifier, Challenge: challenge}, nil
}

// JWTPayload decodes (without verifying) the claims of a JWT. Mirrors the
// reference client, which only reads `sub` and `exp`.
func JWTPayload(token string) (map[string]any, error) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("not a JWT")
	}
	data, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		// Some tokens use standard padding; tolerate it.
		if d2, e2 := base64.StdEncoding.DecodeString(parts[1]); e2 == nil {
			data = d2
		} else {
			return nil, fmt.Errorf("decode payload: %w", err)
		}
	}
	var claims map[string]any
	if err := json.Unmarshal(data, &claims); err != nil {
		return nil, fmt.Errorf("parse claims: %w", err)
	}
	return claims, nil
}

// AccountScope is sha256(sub || accessToken-fallback) as a hex string, matching
// the reference client's accountScope().
func AccountScope(accessToken string) string {
	claims, _ := JWTPayload(accessToken)
	key := accessToken
	if claims != nil {
		if sub, ok := claims["sub"].(string); ok && sub != "" {
			key = sub
		}
	}
	sum := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", sum)
}

// ExpiresSoon reports whether the access token expires within marginMs, or has
// no readable exp.
func ExpiresSoon(accessToken string, nowMs, marginMs int64) bool {
	claims, err := JWTPayload(accessToken)
	if err != nil {
		return true
	}
	expF, ok := claims["exp"].(float64)
	if !ok || expF == 0 {
		return true
	}
	expMs := int64(expF) * 1000
	return expMs-nowMs < marginMs
}
