package app

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"
)

// Stored hashes use salt:key with this fixed derivation policy. Changing the
// policy requires a versioned format and backward-compatible verification.
const passwordIterations = 600_000

func passwordHash(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, passwordIterations, 32)
	if err != nil {
		return "", fmt.Errorf("derive password key: %w", err)
	}
	return base64.RawStdEncoding.EncodeToString(salt) + ":" + base64.RawStdEncoding.EncodeToString(key), nil
}

func passwordMatches(encoded, password string) bool {
	parts := strings.Split(encoded, ":")
	if len(parts) != 2 {
		return false
	}
	salt, saltErr := base64.RawStdEncoding.DecodeString(parts[0])
	expected, keyErr := base64.RawStdEncoding.DecodeString(parts[1])
	if saltErr != nil || keyErr != nil || len(salt) != 16 || len(expected) != 32 {
		return false
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, passwordIterations, 32)
	return err == nil && subtle.ConstantTimeCompare(key, expected) == 1
}
