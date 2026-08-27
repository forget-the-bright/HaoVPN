package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadServer 从路径加载服务端配置；文件不存在则生成默认配置后返回。
func LoadServer(path string) (*ServerConfig, bool, error) {
	created, err := ensureFile(path, serverYAMLTemplate)
	if err != nil {
		return nil, false, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, created, fmt.Errorf("读取配置 %s: %w", path, err)
	}
	var cfg ServerConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, created, fmt.Errorf("解析配置 %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, created, err
	}
	return &cfg, created, nil
}

// LoadClient 从路径加载客户端配置；文件不存在则生成默认配置后返回。
func LoadClient(path string) (*ClientConfig, bool, error) {
	created, err := ensureFile(path, clientYAMLTemplate)
	if err != nil {
		return nil, false, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, created, fmt.Errorf("读取配置 %s: %w", path, err)
	}
	var cfg ClientConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, created, fmt.Errorf("解析配置 %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, created, err
	}
	return &cfg, created, nil
}

// ensureFile 若配置文件不存在，创建父目录并写入带注释的默认模板。
func ensureFile(path, template string) (created bool, err error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return false, fmt.Errorf("创建配置目录 %s: %w", dir, err)
		}
	}
	if err := os.WriteFile(path, []byte(template), 0o600); err != nil {
		return false, fmt.Errorf("写入默认配置 %s: %w", path, err)
	}
	return true, nil
}

// containsWildcardHost 检查是否含 0.0.0.0 或 :: 全接口绑定。
func containsWildcardHost(hosts []string) bool {
	for _, h := range hosts {
		h = strings.TrimSpace(h)
		if h == "0.0.0.0" || h == "::" {
			return true
		}
	}
	return false
}
