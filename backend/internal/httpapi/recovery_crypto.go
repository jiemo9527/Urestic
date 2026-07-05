package httpapi

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	recoveryPackKind          = "urestic.encryptedRecoveryPack"
	recoveryPackFormatVersion = 3
	recoveryPackKDFIterations = 210000
	recoveryPackSaltBytes     = 16
	recoveryPackNonceBytes    = 12
)

type recoveryPackExportRequest struct {
	Password string          `json:"password"`
	Client   json.RawMessage `json:"client,omitempty"`
}

type recoveryPackImportRequest struct {
	Password string          `json:"password"`
	Pack     json.RawMessage `json:"pack"`
}

type encryptedRecoveryPack struct {
	FormatVersion int                    `json:"formatVersion"`
	Kind          string                 `json:"kind"`
	ExportedAt    string                 `json:"exportedAt"`
	Encryption    recoveryPackEncryption `json:"encryption"`
	Payload       string                 `json:"payload"`
}

type recoveryPackEncryption struct {
	Algorithm  string `json:"algorithm"`
	KDF        string `json:"kdf"`
	Iterations int    `json:"iterations"`
	Salt       string `json:"salt"`
	Nonce      string `json:"nonce"`
}

func encryptRecoveryPack(payload configExport, password string) (encryptedRecoveryPack, error) {
	if err := validateRecoveryPackPassword(password); err != nil {
		return encryptedRecoveryPack{}, err
	}
	plain, err := json.Marshal(payload)
	if err != nil {
		return encryptedRecoveryPack{}, err
	}
	salt, err := randomBytes(recoveryPackSaltBytes)
	if err != nil {
		return encryptedRecoveryPack{}, err
	}
	nonce, err := randomBytes(recoveryPackNonceBytes)
	if err != nil {
		return encryptedRecoveryPack{}, err
	}
	key := pbkdf2SHA256([]byte(password), salt, recoveryPackKDFIterations, 32)
	block, err := aes.NewCipher(key)
	if err != nil {
		return encryptedRecoveryPack{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return encryptedRecoveryPack{}, err
	}
	ciphertext := gcm.Seal(nil, nonce, plain, nil)
	return encryptedRecoveryPack{
		FormatVersion: recoveryPackFormatVersion,
		Kind:          recoveryPackKind,
		ExportedAt:    time.Now().UTC().Format(time.RFC3339Nano),
		Encryption: recoveryPackEncryption{
			Algorithm:  "AES-256-GCM",
			KDF:        "PBKDF2-SHA256",
			Iterations: recoveryPackKDFIterations,
			Salt:       base64.StdEncoding.EncodeToString(salt),
			Nonce:      base64.StdEncoding.EncodeToString(nonce),
		},
		Payload: base64.StdEncoding.EncodeToString(ciphertext),
	}, nil
}

func decryptRecoveryPack(raw json.RawMessage, password string) (configExport, error) {
	if err := validateRecoveryPackPassword(password); err != nil {
		return configExport{}, err
	}
	if len(raw) == 0 {
		return configExport{}, errors.New("请选择恢复包文件。")
	}
	var pack encryptedRecoveryPack
	if err := json.Unmarshal(raw, &pack); err != nil {
		return configExport{}, errors.New("恢复包 JSON 解析失败。")
	}
	if pack.Kind != recoveryPackKind || pack.FormatVersion != recoveryPackFormatVersion {
		return configExport{}, errors.New("不支持的恢复包格式，请导入加密恢复包。")
	}
	if pack.Encryption.Algorithm != "AES-256-GCM" || pack.Encryption.KDF != "PBKDF2-SHA256" || pack.Encryption.Iterations <= 0 {
		return configExport{}, errors.New("不支持的恢复包加密参数。")
	}
	salt, err := base64.StdEncoding.DecodeString(pack.Encryption.Salt)
	if err != nil || len(salt) == 0 {
		return configExport{}, errors.New("恢复包 salt 无效。")
	}
	nonce, err := base64.StdEncoding.DecodeString(pack.Encryption.Nonce)
	if err != nil || len(nonce) == 0 {
		return configExport{}, errors.New("恢复包 nonce 无效。")
	}
	ciphertext, err := base64.StdEncoding.DecodeString(pack.Payload)
	if err != nil || len(ciphertext) == 0 {
		return configExport{}, errors.New("恢复包 payload 无效。")
	}
	key := pbkdf2SHA256([]byte(password), salt, pack.Encryption.Iterations, 32)
	block, err := aes.NewCipher(key)
	if err != nil {
		return configExport{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return configExport{}, err
	}
	if len(nonce) != gcm.NonceSize() {
		return configExport{}, errors.New("恢复包 nonce 长度无效。")
	}
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return configExport{}, errors.New("恢复包密码错误或文件已损坏。")
	}
	var payload configExport
	if err := json.Unmarshal(plain, &payload); err != nil {
		return configExport{}, errors.New("恢复包内容解析失败。")
	}
	return payload, nil
}

func validateRecoveryPackPassword(password string) error {
	if strings.TrimSpace(password) == "" {
		return errors.New("请输入恢复包密码。")
	}
	if len([]rune(password)) < 8 {
		return errors.New("恢复包密码至少 8 个字符。")
	}
	return nil
}

func randomBytes(size int) ([]byte, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return nil, fmt.Errorf("随机数生成失败: %w", err)
	}
	return value, nil
}

func pbkdf2SHA256(password, salt []byte, iterations int, keyLen int) []byte {
	if iterations < 1 {
		iterations = 1
	}
	hLen := sha256.Size
	numBlocks := (keyLen + hLen - 1) / hLen
	derived := make([]byte, 0, numBlocks*hLen)
	for block := 1; block <= numBlocks; block++ {
		mac := hmac.New(sha256.New, password)
		mac.Write(salt)
		var blockIndex [4]byte
		binary.BigEndian.PutUint32(blockIndex[:], uint32(block))
		mac.Write(blockIndex[:])
		u := mac.Sum(nil)
		t := append([]byte(nil), u...)
		for i := 1; i < iterations; i++ {
			mac = hmac.New(sha256.New, password)
			mac.Write(u)
			u = mac.Sum(nil)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		derived = append(derived, t...)
	}
	return derived[:keyLen]
}
