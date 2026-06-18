// Package auth — Argon2id password hashing (docs/09 §2).
//
// Argon2id — текущий рекомендованный OWASP алгоритм (победитель Password
// Hashing Competition). Мы используем golang.org/x/crypto/argon2 (уже в
// зависимостях gateway) и кодируем результат в стандартный PHC-формат
//
//	$argon2id$v=19$m=65536,t=3,p=2$<base64-salt>$<base64-hash>
//
// который САМ В СЕБЕ несёт все параметры и уникальную соль. Это позволяет
// верифицировать пароль без отдельного хранения параметров и безопасно менять
// их в будущем (старые хэши проверяются по своим параметрам — backward-compat).
//
// ПРИВАТНОСТЬ (docs/09 §2): пароль в открытом виде НИКОГДА не хранится и не
// логируется; в БД (doctor.password_hash) попадает только PHC-строка.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2Params are the tunable cost parameters (docs/09 §2: memory ~64MB,
// iterations ~3, parallelism ~1–2). Калибруются под сервер; дефолты —
// DefaultArgon2Params.
type Argon2Params struct {
	// Memory in KiB (64*1024 = 64 MiB).
	Memory uint32
	// Iterations (time cost).
	Iterations uint32
	// Parallelism (number of threads / lanes).
	Parallelism uint8
	// SaltLength in bytes (уникальная соль на пароль).
	SaltLength uint32
	// KeyLength of the derived hash in bytes.
	KeyLength uint32
}

// DefaultArgon2Params follows OWASP-recommended Argon2id settings (docs/09 §2).
var DefaultArgon2Params = Argon2Params{
	Memory:      64 * 1024, // 64 MiB
	Iterations:  3,
	Parallelism: 2,
	SaltLength:  16,
	KeyLength:   32,
}

// ErrInvalidHash is returned when a stored hash string is not a valid PHC
// Argon2id encoding (corrupted data / wrong algorithm).
var ErrInvalidHash = errors.New("auth: invalid argon2id hash format")

// ErrIncompatibleVersion is returned when the hash was produced by a different
// Argon2 version than this binary supports.
var ErrIncompatibleVersion = errors.New("auth: incompatible argon2 version")

// HashPassword derives an Argon2id PHC-encoded hash with a fresh random salt.
func HashPassword(plaintext string, p Argon2Params) (string, error) {
	salt := make([]byte, p.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: read salt: %w", err)
	}
	hash := argon2.IDKey([]byte(plaintext), salt, p.Iterations, p.Memory, p.Parallelism, p.KeyLength)

	b64 := base64.RawStdEncoding
	encoded := fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.Memory, p.Iterations, p.Parallelism,
		b64.EncodeToString(salt), b64.EncodeToString(hash),
	)
	return encoded, nil
}

// VerifyPassword reports whether plaintext matches the PHC-encoded Argon2id
// hash. Параметры берутся из самого хэша (backward-compat при их смене).
// Сравнение — constant-time (защита от timing-атак).
func VerifyPassword(plaintext, encoded string) (bool, error) {
	p, salt, want, err := decodeArgon2Hash(encoded)
	if err != nil {
		return false, err
	}
	got := argon2.IDKey([]byte(plaintext), salt, p.Iterations, p.Memory, p.Parallelism, p.KeyLength)
	if subtle.ConstantTimeCompare(got, want) == 1 {
		return true, nil
	}
	return false, nil
}

// decodeArgon2Hash parses a PHC string back into params, salt and the stored
// derived key.
func decodeArgon2Hash(encoded string) (Argon2Params, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	// "", "argon2id", "v=19", "m=..,t=..,p=..", salt, hash
	if len(parts) != 6 || parts[1] != "argon2id" {
		return Argon2Params{}, nil, nil, ErrInvalidHash
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return Argon2Params{}, nil, nil, ErrInvalidHash
	}
	if version != argon2.Version {
		return Argon2Params{}, nil, nil, ErrIncompatibleVersion
	}

	var p Argon2Params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.Memory, &p.Iterations, &p.Parallelism); err != nil {
		return Argon2Params{}, nil, nil, ErrInvalidHash
	}

	b64 := base64.RawStdEncoding
	salt, err := b64.DecodeString(parts[4])
	if err != nil {
		return Argon2Params{}, nil, nil, ErrInvalidHash
	}
	hash, err := b64.DecodeString(parts[5])
	if err != nil {
		return Argon2Params{}, nil, nil, ErrInvalidHash
	}
	p.SaltLength = uint32(len(salt))
	p.KeyLength = uint32(len(hash))
	return p, salt, hash, nil
}
