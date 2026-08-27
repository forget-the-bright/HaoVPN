// Package crypto 封装密钥生成与隧道载荷加解密（基于 X25519 共享密钥 + ChaCha20-Poly1305 + 防重放窗口）。
package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"sync"
	"sync/atomic"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/replay"

	"haovpn/internal/logger"
)

// KeyPair 持有一对 WireGuard 风格密钥（base64）。
type KeyPair struct {
	PrivateKey string
	PublicKey  string
}

// Session 使用双方 X25519 共享密钥派生的 AEAD 加解密隧道载荷，带 RFC6479 防重放。
type Session struct {
	aead        cipherAEAD
	sendCounter atomic.Uint64
	replayMu    sync.Mutex
	replay      replay.Filter
}

type cipherAEAD interface {
	Seal(dst, nonce, plaintext, additionalData []byte) []byte
	Open(dst, nonce, ciphertext, additionalData []byte) ([]byte, error)
	NonceSize() int
	Overhead() int
}

const replayCounterLimit = ^uint64(0)

// GenerateKeyPair 生成新密钥对。
func GenerateKeyPair() (KeyPair, error) {
	priv, err := generatePrivateKey()
	if err != nil {
		return KeyPair{}, fmt.Errorf("generate key: %w", err)
	}
	pub := publicKey(priv)
	return KeyPair{
		PrivateKey: base64.StdEncoding.EncodeToString(priv[:]),
		PublicKey:  base64.StdEncoding.EncodeToString(pub[:]),
	}, nil
}

func generatePrivateKey() (device.NoisePrivateKey, error) {
	var sk device.NoisePrivateKey
	if _, err := rand.Read(sk[:]); err != nil {
		return sk, err
	}
	// WireGuard / X25519 钳制
	sk[0] &= 248
	sk[31] = (sk[31] & 127) | 64
	return sk, nil
}

func publicKey(sk device.NoisePrivateKey) device.NoisePublicKey {
	var pk device.NoisePublicKey
	curve25519.ScalarBaseMult((*[32]byte)(&pk), (*[32]byte)(&sk))
	return pk
}

// ParsePrivateKey 解码 base64 私钥。
func ParsePrivateKey(s string) (device.NoisePrivateKey, error) {
	var key device.NoisePrivateKey
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return key, fmt.Errorf("decode private key: %w", err)
	}
	if len(b) != len(key) {
		return key, fmt.Errorf("invalid private key length: %d", len(b))
	}
	copy(key[:], b)
	return key, nil
}

// ParsePublicKey 解码 base64 公钥。
func ParsePublicKey(s string) (device.NoisePublicKey, error) {
	var key device.NoisePublicKey
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return key, fmt.Errorf("decode public key: %w", err)
	}
	if len(b) != len(key) {
		return key, fmt.Errorf("invalid public key length: %d", len(b))
	}
	copy(key[:], b)
	return key, nil
}

// NewSession 用本地私钥与对端公钥建立会话。
// 双方各自 NewSession(自己私钥, 对方公钥) 必须得到同一 AEAD 密钥。
func NewSession(privateKeyB64, peerPublicKeyB64 string) (*Session, error) {
	priv, err := ParsePrivateKey(privateKeyB64)
	if err != nil {
		return nil, err
	}
	peer, err := ParsePublicKey(peerPublicKeyB64)
	if err != nil {
		return nil, err
	}
	keyMaterial, err := deriveSharedKey(priv, peer)
	if err != nil {
		return nil, err
	}
	aead, err := chacha20poly1305.New(keyMaterial)
	if err != nil {
		return nil, fmt.Errorf("create aead: %w", err)
	}
	return &Session{aead: aead}, nil
}

// PublicKeyFromPrivate 从私钥导出公钥 base64。
func PublicKeyFromPrivate(privateKeyB64 string) (string, error) {
	priv, err := ParsePrivateKey(privateKeyB64)
	if err != nil {
		return "", err
	}
	pub := publicKey(priv)
	return base64.StdEncoding.EncodeToString(pub[:]), nil
}

// deriveSharedKey 计算 X25519 共享密钥并做域分隔哈希，保证 A↔B 对称。
func deriveSharedKey(priv device.NoisePrivateKey, peer device.NoisePublicKey) ([]byte, error) {
	shared, err := curve25519.X25519(priv[:], peer[:])
	if err != nil {
		return nil, fmt.Errorf("x25519: %w", err)
	}
	sum := sha256.Sum256(append([]byte("HaoVPN-tunnel-v1"), shared...))
	return sum[:], nil
}

// Encrypt 加密 IP 包（12 字节 counter nonce || ciphertext+tag）。
func (s *Session) Encrypt(plaintext []byte) ([]byte, error) {
	counter := s.sendCounter.Add(1) - 1
	nonce := counterToNonce(counter)
	sealed := s.aead.Seal(nil, nonce, plaintext, nil)
	out := append(nonce, sealed...)
	logger.Trace("encrypted packet len=%d counter=%d", len(plaintext), counter)
	return out, nil
}

// Decrypt 解密对端载荷并校验防重放窗口。
func (s *Session) Decrypt(ciphertext []byte) ([]byte, error) {
	ns := s.aead.NonceSize()
	if len(ciphertext) < ns+s.aead.Overhead() {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce := ciphertext[:ns]
	sealed := ciphertext[ns:]
	counter := nonceToCounter(nonce)

	s.replayMu.Lock()
	ok := s.replay.ValidateCounter(counter, replayCounterLimit)
	s.replayMu.Unlock()
	if !ok {
		logger.Warn("replay attack detected: counter=%d rejected", counter)
		return nil, fmt.Errorf("replay detected: counter=%d", counter)
	}

	out, err := s.aead.Open(nil, nonce, sealed, nil)
	if err != nil {
		logger.Warn("decrypt failed (possible tamper or key mismatch): %v", err)
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	logger.Trace("decrypted packet len=%d counter=%d", len(out), counter)
	return out, nil
}

func counterToNonce(counter uint64) []byte {
	nonce := make([]byte, 12)
	binary.BigEndian.PutUint64(nonce[4:], counter)
	return nonce
}

func nonceToCounter(nonce []byte) uint64 {
	if len(nonce) < 12 {
		return 0
	}
	return binary.BigEndian.Uint64(nonce[4:12])
}
