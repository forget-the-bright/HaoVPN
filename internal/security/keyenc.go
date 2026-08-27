// Package security 提供 TLS 策略、脱敏与敏感数据加密等安全能力。
package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
)

const privateKeyEncPrefix = "enc:v1:"

// KeyEnc 使用 AES-256-GCM 加密 SQLite 中的 peer 私钥等敏感字段。
type KeyEnc struct {
	aead cipher.AEAD
}

// NewKeyEnc 从 32 字节原始密钥构造加密器。
func NewKeyEnc(key []byte) (*KeyEnc, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("encryption key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm: %w", err)
	}
	return &KeyEnc{aead: aead}, nil
}

// NewKeyEncFromHex 从 64 字符 hex 字符串加载密钥（server.yaml database.encryption_key）。
func NewKeyEncFromHex(hexKey string) (*KeyEnc, error) {
	key, err := hex.DecodeString(strings.TrimSpace(hexKey))
	if err != nil {
		return nil, fmt.Errorf("decode encryption_key hex: %w", err)
	}
	return NewKeyEnc(key)
}

// IsEncryptedPrivateKey 判断库中私钥字段是否已 AES 加密。
func IsEncryptedPrivateKey(stored string) bool {
	return strings.HasPrefix(stored, privateKeyEncPrefix)
}

// SealPrivateKey 加密 peer 私钥（base64 明文）后写入 SQLite。
func (k *KeyEnc) SealPrivateKey(plainB64 string) (string, error) {
	if plainB64 == "" {
		return "", nil
	}
	nonce := make([]byte, k.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("nonce: %w", err)
	}
	sealed := k.aead.Seal(nil, nonce, []byte(plainB64), nil)
	payload := append(nonce, sealed...)
	return privateKeyEncPrefix + base64.StdEncoding.EncodeToString(payload), nil
}

// OpenPrivateKey 解密库中私钥；若为历史明文则原样返回（由启动迁移逻辑再加密）。
func (k *KeyEnc) OpenPrivateKey(stored string) (string, error) {
	if stored == "" {
		return "", nil
	}
	if !IsEncryptedPrivateKey(stored) {
		return stored, nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, privateKeyEncPrefix))
	if err != nil {
		return "", fmt.Errorf("decode sealed private key: %w", err)
	}
	ns := k.aead.NonceSize()
	if len(raw) < ns {
		return "", fmt.Errorf("sealed private key too short")
	}
	plain, err := k.aead.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return "", fmt.Errorf("decrypt private key: %w", err)
	}
	return string(plain), nil
}
