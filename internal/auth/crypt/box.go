// Package crypt seals small secrets (OAuth refresh tokens) at rest with
// AES-256-GCM under a key derived from the ENCRYPTION_KEY secret.
package crypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/scrypt"
)

const (
	version = "v1"
	// Fixed salt: the secret is high-entropy and per-deployment, and a fixed
	// salt keeps derivation deterministic across restarts without extra state.
	salt         = "oido-gcal-mcp/token-store/v1"
	minSecretLen = 16
)

// ErrMalformed reports a sealed value that is not in this package's format.
var ErrMalformed = errors.New("malformed sealed value")

// Box encrypts and decrypts with one derived key.
type Box struct {
	aead cipher.AEAD
}

// New derives the AES key with scrypt (once per process; it is deliberately slow).
func New(secret string) (*Box, error) {
	if len(secret) < minSecretLen {
		return nil, fmt.Errorf("encryption key must be at least %d characters", minSecretLen)
	}
	key, err := scrypt.Key([]byte(secret), []byte(salt), 1<<15, 8, 1, 32)
	if err != nil {
		return nil, fmt.Errorf("derive key: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// Seal encrypts plaintext under a fresh random nonce. The associated data
// binds the ciphertext to its owner (e.g. the account email) so sealed values
// cannot be swapped between records.
func (b *Box) Seal(plaintext, associated string) (string, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("nonce: %w", err)
	}
	sealed := b.aead.Seal(nonce, nonce, []byte(plaintext), []byte(associated))
	return version + ":" + base64.RawStdEncoding.EncodeToString(sealed), nil
}

// Open reverses Seal; it fails if the value, key or associated data differ.
func (b *Box) Open(value, associated string) (string, error) {
	prefix, body, ok := strings.Cut(value, ":")
	if !ok || prefix != version {
		return "", ErrMalformed
	}
	raw, err := base64.RawStdEncoding.DecodeString(body)
	if err != nil || len(raw) < b.aead.NonceSize() {
		return "", ErrMalformed
	}
	nonce, ciphertext := raw[:b.aead.NonceSize()], raw[b.aead.NonceSize():]
	plain, err := b.aead.Open(nil, nonce, ciphertext, []byte(associated))
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(plain), nil
}
