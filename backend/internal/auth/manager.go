package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/urestic/urestic/backend/internal/config"
)

const CookieName = "urestic_session"

var ErrAuthPasswordMissing = errors.New("admin password is required when auth is enabled")

type Manager struct {
	cfg          config.Config
	key          []byte
	passwordHash string
}

type User struct {
	Username string `json:"username"`
}

type LoginResult struct {
	User      User      `json:"user"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type tokenPayload struct {
	Subject string `json:"sub"`
	Expires int64  `json:"exp"`
}

func Open(cfg config.Config) (*Manager, error) {
	manager := &Manager{cfg: cfg}
	if !cfg.AuthEnabled {
		return manager, nil
	}
	overrideHash := loadPasswordHash(cfg.DataDir)
	manager.passwordHash = overrideHash
	if strings.TrimSpace(cfg.AdminPassword) == "" && strings.TrimSpace(cfg.AdminPasswordHash) == "" && overrideHash == "" {
		return nil, ErrAuthPasswordMissing
	}

	key, err := openKey(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	manager.key = key
	return manager, nil
}

func (m *Manager) Enabled() bool {
	return m.cfg.AuthEnabled
}

func (m *Manager) Username() string {
	return m.cfg.AdminUsername
}

func (m *Manager) TTL() time.Duration {
	hours := m.cfg.SessionTTLHours
	if hours <= 0 {
		hours = 12
	}
	return time.Duration(hours) * time.Hour
}

func (m *Manager) Login(_ context.Context, username string, password string) (LoginResult, bool) {
	if !m.validCredentials(username, password) {
		return LoginResult{}, false
	}

	expiresAt := time.Now().UTC().Add(m.TTL())
	token, err := m.issueToken(username, expiresAt)
	if err != nil {
		return LoginResult{}, false
	}

	return LoginResult{User: User{Username: username}, Token: token, ExpiresAt: expiresAt}, true
}

func (m *Manager) ChangePassword(currentPassword string, newPassword string) error {
	if !m.validCredentials(m.cfg.AdminUsername, currentPassword) {
		return errors.New("current password is invalid")
	}
	newPassword = strings.TrimSpace(newPassword)
	if len(newPassword) < 8 {
		return errors.New("new password must be at least 8 characters")
	}

	hash := sha256.Sum256([]byte(newPassword))
	encoded := "sha256:" + hex.EncodeToString(hash[:])
	if err := os.MkdirAll(m.cfg.DataDir, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(passwordHashPath(m.cfg.DataDir), []byte(encoded), 0o600); err != nil {
		return err
	}
	m.passwordHash = encoded
	return nil
}

func (m *Manager) VerifyToken(token string) (User, bool) {
	if !m.cfg.AuthEnabled {
		return User{Username: m.cfg.AdminUsername}, true
	}

	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return User{}, false
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return User{}, false
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return User{}, false
	}

	expected := m.sign(parts[0])
	if subtle.ConstantTimeCompare(signature, expected) != 1 {
		return User{}, false
	}

	var payload tokenPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return User{}, false
	}
	if payload.Subject != m.cfg.AdminUsername {
		return User{}, false
	}
	if time.Now().UTC().Unix() >= payload.Expires {
		return User{}, false
	}

	return User{Username: payload.Subject}, true
}

func (m *Manager) issueToken(username string, expiresAt time.Time) (string, error) {
	payloadBytes, err := json.Marshal(tokenPayload{Subject: username, Expires: expiresAt.Unix()})
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(payloadBytes)
	signature := base64.RawURLEncoding.EncodeToString(m.sign(payload))
	return payload + "." + signature, nil
}

func (m *Manager) sign(payload string) []byte {
	hash := hmac.New(sha256.New, m.key)
	_, _ = hash.Write([]byte(payload))
	return hash.Sum(nil)
}

func (m *Manager) validCredentials(username string, password string) bool {
	username = strings.TrimSpace(username)
	if subtle.ConstantTimeCompare([]byte(username), []byte(m.cfg.AdminUsername)) != 1 {
		return false
	}

	if hash := strings.TrimSpace(m.passwordHash); hash != "" {
		return verifySHA256Hash(password, hash)
	}
	if hash := strings.TrimSpace(m.cfg.AdminPasswordHash); hash != "" {
		return verifySHA256Hash(password, hash)
	}
	return subtle.ConstantTimeCompare([]byte(password), []byte(m.cfg.AdminPassword)) == 1
}

func loadPasswordHash(dataDir string) string {
	value, err := os.ReadFile(passwordHashPath(dataDir))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(value))
}

func passwordHashPath(dataDir string) string {
	return filepath.Join(dataDir, "admin_password.sha256")
}

func verifySHA256Hash(password string, expected string) bool {
	expected = strings.TrimPrefix(expected, "sha256:")
	expectedBytes, err := hex.DecodeString(expected)
	if err != nil || len(expectedBytes) != sha256.Size {
		return false
	}

	sum := sha256.Sum256([]byte(password))
	return subtle.ConstantTimeCompare(sum[:], expectedBytes) == 1
}

func openKey(dataDir string) ([]byte, error) {
	path := filepath.Join(dataDir, "auth.key")
	if value, err := os.ReadFile(path); err == nil {
		decoded, err := base64.RawStdEncoding.DecodeString(strings.TrimSpace(string(value)))
		if err != nil {
			return nil, err
		}
		return decoded, nil
	}

	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	encoded := base64.RawStdEncoding.EncodeToString(key)
	if err := os.WriteFile(path, []byte(encoded), 0o600); err != nil {
		return nil, err
	}
	return key, nil
}
