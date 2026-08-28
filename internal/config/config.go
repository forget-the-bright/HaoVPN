package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"haovpn/internal/fileutil"
)

// loadYAML 通用 YAML 加载：不存在则写模板，解析后调用 validate。
func loadYAML[T any](path, template string, validate func(*T) error) (*T, bool, error) {
	created, err := ensureFile(path, template)
	if err != nil {
		return nil, false, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, created, fmt.Errorf("读取配置 %s: %w", path, err)
	}
	var cfg T
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, created, fmt.Errorf("解析配置 %s: %w", path, err)
	}
	if err := validate(&cfg); err != nil {
		return nil, created, err
	}
	return &cfg, created, nil
}

// LoadServer 从路径加载服务端配置；文件不存在则生成默认配置后返回。
func LoadServer(path string) (*ServerConfig, bool, error) {
	return loadYAML(path, serverYAMLTemplate, func(c *ServerConfig) error { return c.Validate() })
}

// LoadClient 从路径加载客户端配置；文件不存在则生成默认配置后返回。
func LoadClient(path string) (*ClientConfig, bool, error) {
	return loadYAML(path, clientYAMLTemplate, func(c *ClientConfig) error { return c.Validate() })
}

// ensureFile 若配置文件不存在，创建父目录并以原子写写入带注释的默认模板。
func ensureFile(path, template string) (created bool, err error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	if err := fileutil.WriteFileAtomic(path, []byte(template), 0o600); err != nil {
		return false, fmt.Errorf("写入默认配置 %s: %w", path, err)
	}
	return true, nil
}
