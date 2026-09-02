package config

import (
	"os"
	"path/filepath"

	"haovpn/internal/fileutil"
)

// ResolveClientConfigPath 未指定 -c 时：exe 同目录 client.yaml → 当前目录 → 默认写到 exe 旁。
//
// 为何优先 exe 旁：Windows 服务与 GUI 工作目录可能不是用户期望的配置目录；
// ExecutableDir 保证与 wintun.dll / 凭据目录同一锚点。
func ResolveClientConfigPath() string {
	if dir, err := fileutil.ExecutableDir(); err == nil {
		cand := filepath.Join(dir, "client.yaml")
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
	if dir, err := fileutil.ExecutableDir(); err == nil {
		return filepath.Join(dir, "client.yaml")
	}
	return "client.yaml"
}

// ResolveRelativePaths 将配置内相对文件路径按共用规则展开（内存改写，不改 YAML 盘面）。
//
// 见 ResolveFilePath：绝对 > exe 旁（存在）> 配置目录。字段：ca_file、log.file。
func (c *ClientConfig) ResolveRelativePaths(cfgPath string) {
	if c == nil {
		return
	}
	c.Server.TLS.CAFile = resolvePathAgainstConfig(cfgPath, c.Server.TLS.CAFile)
	c.Log.File = resolvePathAgainstConfig(cfgPath, c.Log.File)
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
