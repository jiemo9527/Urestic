package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const keySize = 32

type Manager struct {
	key []byte
}

func Open(dataDir string) (*Manager, error) {
	keyPath := filepath.Join(dataDir, "secret.key")
	key, err := loadOrCreateKey(keyPath)
	if err != nil {
		return nil, err
	}

	return &Manager{key: key}, nil
}

func (m *Manager) Encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(m.key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return "v1:" + base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (m *Manager) Decrypt(ciphertext string) (string, error) {
	encoded, ok := strings.CutPrefix(ciphertext, "v1:")
	if !ok {
		return "", errors.New("unsupported ciphertext format")
	}

	sealed, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(m.key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	if len(sealed) < gcm.NonceSize() {
		return "", errors.New("ciphertext is too short")
	}

	nonce := sealed[:gcm.NonceSize()]
	message := sealed[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, message, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

func loadOrCreateKey(keyPath string) ([]byte, error) {
	content, err := os.ReadFile(keyPath)
	if err == nil {
		key, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(content)))
		if err != nil {
			return nil, fmt.Errorf("decode secret key: %w", err)
		}
		if len(key) != keySize {
			return nil, fmt.Errorf("secret key must be %d bytes", keySize)
		}
		return key, nil
	}

	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil {
		return nil, err
	}

	key := make([]byte, keySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}

	encoded := base64.RawURLEncoding.EncodeToString(key)
	if err := os.WriteFile(keyPath, []byte(encoded), 0o600); err != nil {
		return nil, err
	}

	return key, nil
}
