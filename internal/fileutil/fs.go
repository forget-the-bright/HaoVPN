package fileutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Exists 判断路径是否存在（文件或目录均可）。
//
// 用途：TLS 证书探测、健康检查、自启状态；替代各包私有 fileExists。
// 注意：跟随符号链接；权限不足等 Stat 错误视为不存在（与历史私有 helper 一致）。
func Exists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

// AbsPair 将可执行文件与可选配置路径解析为绝对路径。
//
// 参数：exe — 必填；cfg — 可空（空则返回空 cfgAbs）。
// 返回：绝对路径；任一 Abs 失败时带中文包装错误。
// 用途：autostart linux/darwin 登录自启与服务注册共用，避免三端复制 Abs 样板。
func AbsPair(exe, cfg string) (exeAbs, cfgAbs string, err error) {
	exeAbs, err = filepath.Abs(exe)
	if err != nil {
		return "", "", fmt.Errorf("解析可执行文件路径: %w", err)
	}
	cfg = strings.TrimSpace(cfg)
	if cfg == "" {
		return exeAbs, "", nil
	}
	cfgAbs, err = filepath.Abs(cfg)
	if err != nil {
		return "", "", fmt.Errorf("解析配置路径: %w", err)
	}
	return exeAbs, cfgAbs, nil
}

// EnsureDir 确保目录自身存在（MkdirAll）；与 EnsureParentDir（父目录）互补。
func EnsureDir(dir string, perm os.FileMode) error {
	if strings.TrimSpace(dir) == "" {
		return nil
	}
	return os.MkdirAll(dir, perm)
}
