//go:build windows

package netstack

// ics_enable_windows.go：ICS EnableSharing + PreferVPN 加固 + Teardown 关共享。
// 出站挑选见 ics_egress_windows.go；多 LAN 规划见 ics_plan.go。
//
// 决策：
//   有活 137 → 复用（不拆、不 Restart、不 Enable）；Go iphlp PreferVPN
//   无 137 → 冷启 Restart SharedAccess → Enable（PS）→ Go iphlp PreferVPN（禁嵌 PS Prefer）

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"haovpn/internal/logger"
	"haovpn/internal/safeutil"
	"haovpn/internal/winnet"
)

// setupICSForLANs 多 LAN 时只 Enable 一次 ICS：同首网卡网段生效，异网卡跳过并提示。
func setupICSForLANs(ctx context.Context, tunName string, lanCIDRs []string, outboundIf string, tunIP net.IP) (NATSetupOutcome, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := safeutil.Check(ctx); err != nil {
		return NATSetupOutcome{}, err
	}
	bindings, err := resolveICSBindings(ctx, lanCIDRs, outboundIf)
	if err != nil {
		return NATSetupOutcome{}, err
	}
	plan := PlanICSByOutboundPreferred(outboundIf, bindings)
	if plan.PrimaryIf == "" {
		return NATSetupOutcome{}, fmt.Errorf("ICS 回退: 无法解析任何出站网卡（可配置 nat.outbound_interface）")
	}
	if hint := FormatICSLocalLANsHint(plan); hint != "" {
		if len(plan.Skipped) > 0 {
			logger.Warn("ics_multi_nic\n%s", hint)
		} else {
			logger.Info("ics_enable once\n%s", hint)
		}
	} else {
		logger.Info("ics_enable once public=%s active_lans=%d", plan.PrimaryIf, len(plan.Active))
	}
	if err := setupICSWithPublicIf(ctx, tunName, plan.PrimaryIf, tunIP); err != nil {
		return NATSetupOutcome{}, err
	}
	return NATSetupOutcome{UsedICS: true, Plan: plan}, nil
}

// setupICSWithPublicIf 在已选定的公网侧网卡上启用或复用 ICS（TUN 为私网侧）。
func setupICSWithPublicIf(ctx context.Context, tunName, lanIf string, tunIP net.IP) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := safeutil.Check(ctx); err != nil {
		return err
	}
	lanIf = strings.TrimSpace(lanIf)
	if lanIf == "" {
		return fmt.Errorf("ICS: 出站网卡名为空")
	}
	logger.Info("windows: ICS 回退 lan_if=%s tun=%s", lanIf, tunName)

	tunIfIndex := 0
	if idx, err := winnet.InterfaceIndex(tunName); err == nil {
		tunIfIndex = idx
	}

	vpn := ""
	if tunIP != nil {
		if v4 := tunIP.To4(); v4 != nil {
			vpn = v4.String()
		}
	}

	// 快路径：活 ICS（137）→ 不拆、不 Restart、不 Enable COM
	if winnet.HasICSResidue(tunName) {
		return reuseLiveICS(ctx, tunName, lanIf, vpn, tunIfIndex)
	}

	// 无 137：冷启。pre_clear 不必再探（有残留会走 reuse_live）。
	return enableICSCold(ctx, tunName, lanIf, vpn, tunIfIndex)
}

// reuseLiveICS HardRestart/指纹重建：共享仍在，只刷 SkipAsSource + 清 TUN 默认路由。
func reuseLiveICS(ctx context.Context, tunName, lanIf, vpn string, tunIfIndex int) error {
	start := time.Now()
	logger.Info("ics_refresh action=reuse_live reason=has_137 public=%s private=%s", lanIf, tunName)
	winnet.RememberICSPair(lanIf, tunName)
	if err := applyPreferVPNAfterICS(ctx, tunName, vpn, tunIfIndex); err != nil {
		return err
	}
	logger.Info("ics_refresh action=reuse_live elapsed=%s", time.Since(start))
	return nil
}

// enableICSCold 无活 137：Restart → Enable（同一次 PS）→ Go iphlp PreferVPN（勿嵌 PS Get-NetIPAddress）。
func enableICSCold(ctx context.Context, tunName, lanIf, vpn string, tunIfIndex int) error {
	tunAlias := winnet.ResolveInterfaceAlias(tunName)

	// 进入本函数时调用方已确认无 137；勿再 HasICSResidue（会双打日志）。
	logger.Info("windows: ics_enable pre_clear=skip reason=no_residue")

	enableStart := time.Now()
	// preferSnippet 留空：Enable 后走 PreferVPNAfterSoftIPReplace（iphlp），与 reuse_live 对齐。
	ps := winnet.PSSnippetICSEnableSharing(
		lanIf, tunName, tunIfIndex, tunAlias,
		"",
		"",
	)

	out, err := winnet.RunPSOneShotContext(ctx, ps)
	enableElapsed := time.Since(enableStart)
	if err != nil {
		logger.Info("ics_enable elapsed=%s err=%v", enableElapsed, err)
		if safeutil.IsCanceled(err) || safeutil.Check(ctx) != nil {
			logger.Info("windows: ICS 启用已取消 err=%v", err)
			if e := safeutil.Check(ctx); e != nil {
				return e
			}
			return err
		}
		return fmt.Errorf("ICS 启用失败: %w（家庭版请确认 LAN 网卡名正确且 SharedAccess 服务可启动）", err)
	}
	sawRestart := false
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		// 脚本内分段耗时（com_init/restart/enable）；不含 powershell.exe 进程冷启与后续 iphlp Prefer。
		if strings.HasPrefix(line, "ics_stage ") {
			logger.Info("windows: %s", line)
			continue
		}
		if strings.HasPrefix(line, "ics_sharedaccess ") {
			logger.Info("windows: %s", line)
			if strings.Contains(line, "action=restart") {
				sawRestart = true
			}
		}
	}
	if !sawRestart {
		logger.Warn("windows: ics_enable 未见到 SharedAccess Restart 日志（脚本应无条件 Restart）")
	}
	logger.Info("ics_enable elapsed=%s public=%s private=%s prefer=iphlp_after sharedaccess_restart=%v",
		enableElapsed, lanIf, tunName, sawRestart)
	logger.Info("windows: ICS 已启用 public=%s private=%s（VPN→LAN NAT 回退）", lanIf, tunName)
	winnet.RememberICSPair(lanIf, tunName)
	logger.Info("windows: ics_link_risk public=%s note=EnableSharing_may_drop_tunnel", lanIf)

	return applyPreferVPNAfterICS(ctx, tunName, vpn, tunIfIndex)
}

// applyPreferVPNAfterICS Enable 或 reuse_live 后的 Prefer：iphlp SkipAsSource（内含清 TUN 默认路由，勿再 Late scrub）。
func applyPreferVPNAfterICS(ctx context.Context, tunName, vpn string, tunIfIndex int) error {
	if vpn == "" {
		return nil
	}
	if tunIfIndex <= 0 {
		if idx, err := winnet.InterfaceIndex(tunName); err == nil {
			tunIfIndex = idx
		}
	}
	if tunIfIndex > 0 {
		if err := winnet.PreferVPNAfterSoftIPReplace(ctx, tunName, tunIfIndex, vpn); err != nil {
			logger.Warn("windows: ICS 后 PreferVPN iphlp: %v", err)
		} else {
			logger.Info("windows: ICS 后已 SkipAsSource 非 VPN 地址，本机发包源优先 %s", vpn)
		}
		return nil
	}
	preferStart := time.Now()
	if err := winnet.PreferVPNSourceWithICSContext(ctx, tunName, vpn); err != nil {
		logger.Warn("windows: ICS 后 PreferVPNSource 失败（本机 AllowedIPs 可能仍异常）: %v", err)
	} else {
		logger.Info("ics_prefer_vpn method=full_fallback elapsed=%s", time.Since(preferStart))
		logger.Info("windows: ICS 后已 SkipAsSource 非 VPN 地址，本机发包源优先 %s", vpn)
	}
	return nil
}

// disableICSPlatform 关闭本会话 ICS（委托 winnet.DisableICSSessionContext）。
func disableICSPlatform(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	winnet.DisableICSSessionContext(ctx)
}
