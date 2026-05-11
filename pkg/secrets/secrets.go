// Package secrets is the host-side encryption helper used by anything
// in Flow that persists user-supplied credentials (cloud creds, kv
// secrets, agent PATs). Master key comes from the FLOW_SECRET_KEY env
// var. Format: base64-url(nonce || ciphertext || tag), AES-256-GCM.
//
// Design choices:
//   - Fail loud if the key is missing. We never silently store plaintext
//     in fields the rest of the system thinks are encrypted — that's
//     worse than failing to start.
//   - Master key is hashed with SHA-256 so any reasonable input length
//     works (32-byte hex, base64, or a passphrase).
//   - Each Seal generates a fresh random 12-byte nonce. Ciphertext is
//     authenticated by GCM's tag.
//
// We do NOT version the ciphertext envelope. If we ever need key
// rotation, prepend a 1-byte version + key-id and decode by version.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
)

const envKey = "FLOW_SECRET_KEY"

var (
	keyOnce sync.Once
	keyVal  []byte
	keyErr  error
)

// loadKey reads FLOW_SECRET_KEY once and caches the SHA-256 of its bytes.
// Returns an error (not panic) so callers can surface it cleanly.
func loadKey() ([]byte, error) {
	keyOnce.Do(func() {
		raw := os.Getenv(envKey)
		if raw == "" {
			keyErr = fmt.Errorf("secrets: %s is not set; refusing to encrypt/decrypt", envKey)
			return
		}
		sum := sha256.Sum256([]byte(raw))
		keyVal = sum[:]
	})
	return keyVal, keyErr
}

// SealString encrypts plaintext with AES-256-GCM and returns a
// base64-url-encoded envelope. Empty input returns empty output (so
// "no secret set" round-trips through encryption cleanly).
func SealString(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	key, err := loadKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("aes new: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("gcm new: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("nonce: %w", err)
	}
	ct := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	envelope := append(nonce, ct...)
	return base64.RawURLEncoding.EncodeToString(envelope), nil
}

// OpenString reverses SealString. Empty input returns empty output.
// Any decode/auth failure is reported — never silently fall back to
// returning the input as plaintext.
func OpenString(envelope string) (string, error) {
	if envelope == "" {
		return "", nil
	}
	key, err := loadKey()
	if err != nil {
		return "", err
	}
	raw, err := base64.RawURLEncoding.DecodeString(envelope)
	if err != nil {
		return "", fmt.Errorf("base64: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("aes new: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("gcm new: %w", err)
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	nonce, ct := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("gcm open (key mismatch or tampered): %w", err)
	}
	return string(pt), nil
}

// SealMap encrypts every value of m in place, returning a new map.
// Useful for kv-credential maps without writing a loop at every caller.
func SealMap(m map[string]string) (map[string]string, error) {
	if m == nil {
		return nil, nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		sealed, err := SealString(v)
		if err != nil {
			return nil, fmt.Errorf("seal %q: %w", k, err)
		}
		out[k] = sealed
	}
	return out, nil
}

// OpenMap reverses SealMap.
func OpenMap(m map[string]string) (map[string]string, error) {
	if m == nil {
		return nil, nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		opened, err := OpenString(v)
		if err != nil {
			return nil, fmt.Errorf("open %q: %w", k, err)
		}
		out[k] = opened
	}
	return out, nil
}

// IsConfigured reports whether the master key is set. Useful at startup
// so the API can refuse to mount the credentials endpoints (or warn
// loudly) when secrets won't work.
func IsConfigured() bool {
	_, err := loadKey()
	return err == nil
}
