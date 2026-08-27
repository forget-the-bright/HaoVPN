package config

import (
	"fmt"

	"haovpn/internal/logger"
)

// ToLoggerConfig 将 YAML log 段转为 logger 包配置。
func (l LogSection) ToLoggerConfig() logger.Config {
	return logger.Config{
		Level:      l.Level,
		File:       l.File,
		MaxSizeMB:  l.MaxSizeMB,
		MaxBackups: l.MaxBackups,
	}
}

// InitGlobal 初始化全局日志（服务端/客户端入口共用）。
func (l LogSection) InitGlobal() error {
	if err := logger.Init(l.ToLoggerConfig()); err != nil {
		return fmt.Errorf("日志初始化失败: %w", err)
	}
	return nil
}
