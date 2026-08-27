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
//
// 字段：
//   aead — 构造时由 32 字节密钥初始化的 GCM 实例；只读，构造后不变。
//
// 线程安全：SealPrivateKey/OpenPrivateKey 无共享可变状态，可多 goroutine 并行调用。
type KeyEnc struct {
	aead cipher.AEAD
}

// NewKeyEnc 从 32 字节原始密钥构造加密器。
//
// 参数：key — 须恰好 32 字节（AES-256）；否则返回错误。
// 返回：*KeyEnc 可用于 Seal/Open；err 为长度不符或 AES/GCM 初始化失败。
// 副作用：无；纯构造。
// 并发：返回后可多 goroutine 共用同一实例。
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
//
// 参数：hexKey — 32 字节密钥的十六进制表示；前后空白会被 Trim。
// 返回：*KeyEnc；err 为 hex 解码失败或 NewKeyEnc 校验失败。
// 副作用：无。
// 并发：同 NewKeyEnc。
func NewKeyEncFromHex(hexKey string) (*KeyEnc, error) {
	key, err := hex.DecodeString(strings.TrimSpace(hexKey))
	if err != nil {
		return nil, fmt.Errorf("decode encryption_key hex: %w", err)
	}
	return NewKeyEnc(key)
}

// IsEncryptedPrivateKey 判断库中私钥字段是否已 AES 加密。
//
// 参数：stored — users.private_key_enc 原始字符串。
// 返回：true 表示以 enc:v1: 前缀密封，须经 KeyEnc.OpenPrivateKey 解密。
// 副作用：无；纯判断。
// 并发：任意 goroutine 可调用。
func IsEncryptedPrivateKey(stored string) bool {
	return strings.HasPrefix(stored, privateKeyEncPrefix)
}

// SealPrivateKey 加密 peer 私钥（base64 明文）后写入 SQLite。
//
// 参数：plainB64 — WireGuard 风格私钥的 Base64 明文；空串直接返回空串。
// 返回：enc:v1: 前缀 + Base64(nonce‖ciphertext)；err 为随机数或 GCM Seal 失败。
// 副作用：无（调用方负责写库）；每次调用生成新随机 nonce。
// 并发：可多 goroutine 并行 Seal 不同明文。
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
//
// 参数：stored — 密封串或历史明文 Base64。
// 返回：plainB64 明文私钥；空串输入返回空串；err 为 Base64/GCM 解密失败。
// 副作用：无。
// 并发：可多 goroutine 并行 Open；未加密历史数据不经 AEAD 直接返回。
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
