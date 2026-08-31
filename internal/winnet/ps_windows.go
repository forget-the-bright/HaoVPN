//go:build windows

package winnet

import (
	"strings"

	"haovpn/internal/logger"
	"haovpn/internal/platform"
)

// RunPS 执行 PowerShell 脚本（NoProfile、Bypass、一进程一脚本）。
//
// 参数：script — 完整 -Command 脚本体（须为本包固定模板，勿拼用户输入）。
// 返回：CombinedOutput；失败时 error 含输出摘要。
// 说明：曾可选常驻主机；网卡/ICS/CIM 与常驻不兼容且无加速收益，已移除，等价于 RunPSOneShot。
func RunPS(script string) ([]byte, error) {
	return RunPSOneShot(script)
}

// RunPSOneShot 每次启进程执行脚本（NetNat/ICS/SkipAsSource/清理等唯一路径）。
//
// 参数：script — 完整 -Command 脚本体（须为本包固定模板）。
// 返回：CombinedOutput；失败时 error 含输出摘要。
func RunPSOneShot(script string) ([]byte, error) {
	out, err := platform.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script).CombinedOutput()
	if err != nil {
		return out, platform.CommandOutputError("powershell", out, err)
	}
	return out, nil
}

// RunPSBestEffort 尽力执行 PowerShell；失败只打 Warn，不向上返回 error。
//
// 参数：
//   script — 完整 -Command 脚本体；
//   opName — 日志操作名（如 "DisableAllICS"、"Remove-NetNat"），便于排障检索。
func RunPSBestEffort(script, opName string) {
	out, err := RunPSOneShot(script)
	if err != nil {
		logger.Warn("powershell 尽力操作失败 op=%s: %v out=%s", opName, err, strings.TrimSpace(string(out)))
	}
}

// RunNetsh 执行 netsh 子命令并在失败时格式化错误信息。
func RunNetsh(args ...string) error {
	out, err := platform.Command("netsh", args...).CombinedOutput()
	if err != nil {
		return platform.CommandOutputError("netsh "+strings.Join(args, " "), out, err)
	}
	return nil
}
