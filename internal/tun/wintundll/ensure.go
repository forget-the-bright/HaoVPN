//go:build windows

// Package wintundll 将 Wintun 预编译 DLL 嵌入客户端，首次使用时释放到 exe 同目录供 LoadLibrary 加载。
package wintundll

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"haovpn/internal/fileutil"
)

const dllName = "wintun.dll"

var ensureOnce sync.Once
var ensureErr error

// Ensure 将内嵌 wintun.dll 释放到可执行文件所在目录（与 golang.zx2c4.com/wintun 的加载路径一致）。
// 分发只需单个 exe；首次连 TUN 时若同目录尚无 dll 或内容不一致则写入。
func Ensure() error {
	ensureOnce.Do(func() {
		ensureErr = ensureOnceInner()
	})
	return ensureErr
}

func ensureOnceInner() error {
	if len(embeddedDLL) == 0 {
		return fmt.Errorf("wintun: 内嵌 DLL 为空（构建前请运行 scripts/install-wintun.ps1）")
	}
	dir, err := fileutil.ExecutableDir()
	if err != nil {
		return fmt.Errorf("wintun: 解析 exe 路径: %w", err)
	}
	target := filepath.Join(dir, dllName)

	if fileMatches(target, embeddedDLL) {
		return nil
	}
	// 与敏感配置相同的原子写，避免半截 dll 被 LoadLibrary。
	if err := fileutil.WriteFileAtomic(target, embeddedDLL, 0o644); err != nil {
		return fmt.Errorf("wintun: 安装 %s: %w（请确认 exe 目录可写）", target, err)
	}
	return nil
}

func fileMatches(path string, want []byte) bool {
	got, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return bytes.Equal(got, want)
}
