package security

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"haovpn/internal/config"
	"haovpn/internal/fileutil"
	"haovpn/internal/logger"
)

const defaultKeyFileName = ".haovpn-key"

// LoadOrCreateDataKey 加载数据库字段加密密钥：优先 server.yaml hex，否则 data/.haovpn-key。
//
// 参数：dbCfg — EncryptionKey 非空时优先；EncryptionKeyFile 为空时用 dataDir/.haovpn-key。
// 返回：*KeyEnc 用于 Seal/Open 私钥；err 为 hex 无效、读写密钥文件失败或随机生成失败。
// 副作用：密钥文件不存在时生成 32 字节随机密钥并写入（0600）；打 Info 日志。
// 并发：服务启动单线程调用；返回的 KeyEnc 可多 goroutine 共用。
func LoadOrCreateDataKey(dbCfg config.DatabaseSection, dataDir string) (*KeyEnc, error) {
	if hexKey := strings.TrimSpace(dbCfg.EncryptionKey); hexKey != "" {
		enc, err := NewKeyEncFromHex(hexKey)
		if err != nil {
			return nil, fmt.Errorf("database.encryption_key: %w", err)
		}
		logger.Info("数据库字段加密：使用 server.yaml 中的 encryption_key")
		return enc, nil
	}

	keyPath := strings.TrimSpace(dbCfg.EncryptionKeyFile)
	if keyPath == "" {
		keyPath = filepath.Join(dataDir, defaultKeyFileName)
	}
	if err := fileutil.EnsureParentDir(keyPath, 0o700); err != nil {
		return nil, fmt.Errorf("create key dir: %w", err)
	}

	raw, err := os.ReadFile(keyPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("read key file: %w", err)
		}
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, fmt.Errorf("generate key: %w", err)
		}
		hexStr := hex.EncodeToString(key)
		if err := os.WriteFile(keyPath, []byte(hexStr+"\n"), 0o600); err != nil {
			return nil, fmt.Errorf("write key file: %w", err)
		}
		logger.Info("已生成数据库字段加密密钥: %s（请备份；丢失将无法解密已存私钥）", keyPath)
		return NewKeyEnc(key)
	}

	hexStr := strings.TrimSpace(string(raw))
	enc, err := NewKeyEncFromHex(hexStr)
	if err != nil {
		return nil, fmt.Errorf("key file %s: %w", keyPath, err)
	}
	logger.Info("数据库字段加密：使用密钥文件 %s", keyPath)
	return enc, nil
}
