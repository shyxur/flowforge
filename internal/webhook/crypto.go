package webhook

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const ciphertextVersion = "v1:"

type SecretCipher struct {
	aead cipher.AEAD
}

func NewSecretCipher(keyMaterial string) *SecretCipher {
	key := sha256.Sum256([]byte(keyMaterial))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		panic("AES-256 initialization failed: " + err.Error())
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		panic("AES-GCM initialization failed: " + err.Error())
	}
	return &SecretCipher{aead: aead}
}

func (cipher *SecretCipher) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, cipher.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate webhook secret nonce: %w", err)
	}
	encrypted := cipher.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return ciphertextVersion + base64.RawStdEncoding.EncodeToString(encrypted), nil
}

func (cipher *SecretCipher) Decrypt(ciphertext string) (string, error) {
	if !strings.HasPrefix(ciphertext, ciphertextVersion) {
		return "", errors.New("unsupported webhook secret ciphertext")
	}
	encrypted, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(ciphertext, ciphertextVersion))
	if err != nil {
		return "", fmt.Errorf("decode webhook secret ciphertext: %w", err)
	}
	nonceSize := cipher.aead.NonceSize()
	if len(encrypted) < nonceSize {
		return "", errors.New("invalid webhook secret ciphertext")
	}
	plaintext, err := cipher.aead.Open(nil, encrypted[:nonceSize], encrypted[nonceSize:], nil)
	if err != nil {
		return "", fmt.Errorf("decrypt webhook secret: %w", err)
	}
	return string(plaintext), nil
}

type HMACSigner struct{}

func (HMACSigner) Sign(secret, timestamp string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(payload)
	return "v1=" + hex.EncodeToString(mac.Sum(nil))
}

func (signer HMACSigner) Verify(secret, timestamp string, payload []byte, signature string) bool {
	expected := signer.Sign(secret, timestamp, payload)
	return hmac.Equal([]byte(expected), []byte(signature))
}
