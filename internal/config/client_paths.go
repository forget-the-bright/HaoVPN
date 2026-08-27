package config

import (
	"os"
	"path/filepath"
)

// ResolveClientConfigPath 未指定 -c 时：exe 同目录 client.yaml → 当前目录 → 默认写到 exe 旁。
func ResolveClientConfigPath() string {
	if exe, err := os.Executable(); err == nil {
		cand := filepath.Join(filepath.Dir(exe), "client.yaml")
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			return cand
		}
	}
	if st, err := os.Stat("client.yaml"); err == nil && !st.IsDir() {
		if abs, err := filepath.Abs("client.yaml"); err == nil {
			return abs
		}
		return "client.yaml"
	}
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), "client.yaml")
	}
	return "client.yaml"
}

// LoadClientOrDefaults GUI 用：加载失败时返回带默认值的内存配置（不写盘）。
func LoadClientOrDefaults(path string) *ClientConfig {
	cfg, _, err := LoadClient(path)
	if err == nil && cfg != nil {
		return cfg
	}
	cfg = &ClientConfig{}
	cfg.Server.Address = "REPLACE_WITH_SERVER_IP:8443"
	cfg.Server.TLS.InsecureSkipVerify = false
	cfg.ApplyDefaults()
	return cfg
}
