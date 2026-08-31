//go:build windows

package winnet

import (
	"strings"

	"haovpn/internal/logger"
	"haovpn/internal/platform"
)

// RunPS 执行 PowerShell 脚本（NoProfile、Bypass 执行策略、无控制台窗口）。
//
// 参数：script — 完整 -Command 脚本体。
// 返回：CombinedOutput；失败时 error 含 stderr 摘要（含 stdout/stderr 裁剪）。
// 副作用：启动 powershell.exe 子进程。
//
// 为何统一：netstack/winnet 曾混用「无 Bypass」与「有 Bypass」两种调用，
// 组策略 Restricted 环境下后者可跑、前者失败；所有 PS 必须经本函数或 RunPSBestEffort。
// 关联：platform.Command（无窗口）、CommandOutputError；调用方含 netstack NAT/ICS、本包 ICS。
func RunPS(script string) ([]byte, error) {
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
//
// 用途：清理类操作（关 ICS、删旧 NetNat、开网卡 Forwarding）——失败不应阻断主流程，
// 但必须可观测，禁止 `_ = powershell.Run()` 静默吞错。
// 关联：RunPS（同一 Bypass 策略）；DisableAllICS / netstack enableIPForward / teardownNAT。
func RunPSBestEffort(script, opName string) {
	out, err := RunPS(script)
	if err != nil {
		logger.Warn("powershell 尽力操作失败 op=%s: %v out=%s", opName, err, strings.TrimSpace(string(out)))
	}
}

// RunNetsh 执行 netsh 子命令并在失败时格式化错误信息。
//
// 参数：args — netsh 后续参数（如 interface ipv4 set address ...）。
// 返回：netsh 非零退出或输出含错误时 error。
// 副作用：启动 netsh.exe 子进程，可能修改网络配置（依子命令而定）。
func RunNetsh(args ...string) error {
	out, err := platform.Command("netsh", args...).CombinedOutput()
	if err != nil {
		return platform.CommandOutputError("netsh "+strings.Join(args, " "), out, err)
	}
	return nil
}
