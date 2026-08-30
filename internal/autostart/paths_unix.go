//go:build linux || darwin

package autostart

import "haovpn/internal/fileutil"

// absExeAndOptionalConfig 解析 GUI/CLI 可执行文件与可选配置为绝对路径。
//
// 委托 fileutil.AbsPair，供 logonEnable / serviceInstall 共用，避免 linux/darwin 复制粘贴。
func absExeAndOptionalConfig(exe, cfg string) (exeAbs, cfgAbs string, err error) {
	return fileutil.AbsPair(exe, cfg)
}
