//go:build windows

package winnet

import (
	"context"
	"strings"

	"haovpn/internal/logger"
	"haovpn/internal/platform"
	"haovpn/internal/safeutil"
)

// RunPSOneShot 每次启进程执行脚本（NetNat/ICS/SkipAsSource/清理等唯一路径）。
//
// 参数：script — 完整 -Command 脚本体（须为本包固定模板，勿拼未转义用户输入）。
// 返回：CombinedOutput；失败时 error 含输出摘要。
// 说明：曾可选常驻主机；网卡/ICS/CIM 与常驻不兼容且无加速收益，已移除。
// 无取消需求时用本函数；Stop/HardRestart 路径请用 RunPSOneShotContext。
func RunPSOneShot(script string) ([]byte, error) {
	return RunPSOneShotContext(context.Background(), script)
}

// RunPSOneShotContext 同 RunPSOneShot，ctx 取消时 Kill powershell（Stop 打断 ICS）。
//
// 参数：ctx — 取消则立即返回且尽力 Kill 子进程；script — 固定模板。
// 返回：已取消时优先返回 ctx.Err()（即便已有部分 stdout）；日志键 ps_kill。
// 关联：netstack setupICSPlatform / findOutbound*；clientapp applyPolicy abort。
func RunPSOneShotContext(ctx context.Context, script string) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := safeutil.Check(ctx); err != nil {
		logger.Info("ps_kill stage=before_start err=%v", err)
		return nil, err
	}
	out, err := platform.CommandContext(ctx, "powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script).CombinedOutput()
	if err != nil {
		if e := safeutil.Check(ctx); e != nil {
			logger.Info("ps_kill stage=during_run err=%v", e)
			return out, e
		}
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
	RunPSBestEffortContext(context.Background(), script, opName)
}

// RunPSBestEffortContext 同 RunPSBestEffort，ctx 取消时 Kill 并打 Info（仍不向上返回 error）。
//
// 用途：Teardown/DisableAllICS 等「尽力关闭」路径；取消时勿再空等十余秒 COM。
// 日志：取消 → ps_kill op=…；失败 → 原 Warn。
func RunPSBestEffortContext(ctx context.Context, script, opName string) {
	out, err := RunPSOneShotContext(ctx, script)
	if err != nil {
		if safeutil.IsCanceled(err) {
			logger.Info("ps_kill op=%s err=%v", opName, err)
			return
		}
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
