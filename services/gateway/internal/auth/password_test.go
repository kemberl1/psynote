package auth

import (
	"strings"
	"testing"
)

// fastParams keeps tests quick while exercising the same code path.
var fastParams = Argon2Params{Memory: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32}

func TestHashPassword_FormatAndVerify(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple", fastParams)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$") {
		t.Errorf("unexpected PHC prefix: %q", hash)
	}
	// Plaintext must never appear in the hash.
	if strings.Contains(hash, "correct horse") {
		t.Error("plaintext leaked into hash")
	}

	ok, err := VerifyPassword("correct horse battery staple", hash)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !ok {
		t.Error("correct password failed to verify")
	}
}

func TestVerifyPassword_WrongPassword(t *testing.T) {
	hash, _ := HashPassword("right-password", fastParams)
	ok, err := VerifyPassword("wrong-password", hash)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if ok {
		t.Error("wrong password verified as correct")
	}
}

func TestHashPassword_UniqueSaltPerHash(t *testing.T) {
	h1, _ := HashPassword("same", fastParams)
	h2, _ := HashPassword("same", fastParams)
	if h1 == h2 {
		t.Error("identical passwords produced identical hashes — salt not unique")
	}
	// Both must still verify.
	if ok, _ := VerifyPassword("same", h1); !ok {
		t.Error("h1 failed to verify")
	}
	if ok, _ := VerifyPassword("same", h2); !ok {
		t.Error("h2 failed to verify")
	}
}

func TestVerifyPassword_InvalidHash(t *testing.T) {
	cases := []string{
		"",
		"not-a-hash",
		"$argon2i$v=19$m=8,t=1,p=1$c2FsdA$aGFzaA", // wrong algorithm (argon2i)
		"$argon2id$v=19$bad-params$c2FsdA$aGFzaA",
	}
	for _, c := range cases {
		if _, err := VerifyPassword("x", c); err == nil {
			t.Errorf("expected error for invalid hash %q", c)
		}
	}
}

func TestHashPassword_DefaultParams(t *testing.T) {
	// Sanity: production defaults round-trip (slower, but a single call is fine).
	hash, err := HashPassword("p@ssw0rd-long", DefaultArgon2Params)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if ok, _ := VerifyPassword("p@ssw0rd-long", hash); !ok {
		t.Error("default-params hash failed to verify")
	}
}
