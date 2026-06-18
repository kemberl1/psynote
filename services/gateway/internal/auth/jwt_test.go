package auth

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func newTestTS(t *testing.T) *TokenService {
	t.Helper()
	ts, err := NewTokenService("super-secret-test-key", 15*time.Minute, 24*time.Hour)
	if err != nil {
		t.Fatalf("NewTokenService: %v", err)
	}
	return ts
}

func TestNewTokenService_EmptySecret(t *testing.T) {
	if _, err := NewTokenService("  ", time.Minute, time.Hour); err == nil {
		t.Error("expected error for empty secret")
	}
}

func TestIssueAndParseAccessToken(t *testing.T) {
	ts := newTestTS(t)
	tok, err := ts.IssueAccessToken("doc-123", "doctor")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if strings.Count(tok, ".") != 2 {
		t.Fatalf("not a 3-part JWT: %q", tok)
	}
	claims, err := ts.ParseAccessToken(tok)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if claims.DoctorID != "doc-123" || claims.Role != "doctor" {
		t.Errorf("claims mismatch: %+v", claims)
	}
}

func TestParseAccessToken_Expired(t *testing.T) {
	ts := newTestTS(t)
	// Issue at a fixed past time so exp is already in the past relative to now.
	past := time.Now().Add(-2 * time.Hour)
	ts.now = func() time.Time { return past }
	tok, _ := ts.IssueAccessToken("doc-1", "doctor")

	// Restore real clock for verification.
	ts.now = time.Now
	_, err := ts.ParseAccessToken(tok)
	if !errors.Is(err, ErrTokenExpired) {
		t.Errorf("want ErrTokenExpired, got %v", err)
	}
}

func TestParseAccessToken_BadSignature(t *testing.T) {
	ts := newTestTS(t)
	tok, _ := ts.IssueAccessToken("doc-1", "doctor")

	// A different secret must reject the signature.
	other, _ := NewTokenService("a-totally-different-secret", 15*time.Minute, time.Hour)
	if _, err := other.ParseAccessToken(tok); !errors.Is(err, ErrTokenSignature) {
		t.Errorf("want ErrTokenSignature, got %v", err)
	}

	// Tampering with the payload also breaks the signature.
	parts := strings.Split(tok, ".")
	tampered := parts[0] + "." + parts[1] + "x." + parts[2]
	if _, err := ts.ParseAccessToken(tampered); err == nil {
		t.Error("tampered token accepted")
	}
}

func TestParseAccessToken_Malformed(t *testing.T) {
	ts := newTestTS(t)
	for _, bad := range []string{"", "abc", "a.b", "a.b.c.d"} {
		if _, err := ts.ParseAccessToken(bad); err == nil {
			t.Errorf("malformed token accepted: %q", bad)
		}
	}
}

func TestRefreshToken_HashStableAndOpaque(t *testing.T) {
	tok, hash, err := GenerateRefreshToken()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if tok == "" || hash == "" {
		t.Fatal("empty token or hash")
	}
	if tok == hash {
		t.Error("token equals its hash — hash not applied")
	}
	// HashRefreshToken must be deterministic for lookup.
	if HashRefreshToken(tok) != hash {
		t.Error("hash not stable for the same token")
	}
	// Different tokens → different hashes.
	tok2, hash2, _ := GenerateRefreshToken()
	if tok2 == tok || hash2 == hash {
		t.Error("refresh token generation not unique")
	}
}

func TestTTLAccessors(t *testing.T) {
	ts, _ := NewTokenService("k", 12*time.Minute, 48*time.Hour)
	if ts.AccessTTL() != 12*time.Minute || ts.RefreshTTL() != 48*time.Hour {
		t.Errorf("TTL accessors wrong: %v / %v", ts.AccessTTL(), ts.RefreshTTL())
	}
}
