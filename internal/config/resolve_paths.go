package config

import (
	"path/filepath"
	"strings"

	"haovpn/internal/fileutil"
)

// ResolveFilePath 解析配置中的文件路径（客户端/服务端共用）。
//
// 优先级：
//  1. 已是绝对路径 → 原样（最高）
//  2. 相对路径：先试「应用/exe 同目录」下的相对路径，文件存在则用
//  3. 否则用「配置文件所在目录」下的相对路径（无论该文件是否已存在，便于新建 log/db/自签证书）
//
// 空串原样返回。不依赖进程 CWD，避免服务/计划任务工作目录错位。
func ResolveFilePath(exeDir, cfgDir, p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return p
	}
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	exeDir = strings.TrimSpace(exeDir)
	cfgDir = strings.TrimSpace(cfgDir)
	if abs, err := filepath.Abs(cfgDir); err == nil {
		cfgDir = abs
	}
	if exeDir != "" {
		if abs, err := filepath.Abs(exeDir); err == nil {
			exeDir = abs
		}
		cand := filepath.Clean(filepath.Join(exeDir, p))
		if fileutil.Exists(cand) {
			return cand
		}
	}
	if cfgDir == "" {
		cfgDir = "."
	}
	return filepath.Clean(filepath.Join(cfgDir, p))
}

// resolvePathAgainstConfig 用当前进程 exe 目录 + cfgPath 目录解析一条路径。
func resolvePathAgainstConfig(cfgPath, p string) string {
	exeDir, _ := fileutil.ExecutableDir()
	cfgDir := filepath.Dir(cfgPath)
	return ResolveFilePath(exeDir, cfgDir, p)
}
