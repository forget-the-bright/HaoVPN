//go:build windows

package netstack

// ics_enable_windows.go：ICS EnableSharing + PreferVPN 加固 + Teardown 关共享。
// 出站挑选见 ics_egress_windows.go；多 LAN 规划见 ics_plan.go。
//
// 原则：每次开共享无条件 Restart-Service SharedAccess -Force → Enable；
// 禁止 Soft/already_paired / 按 137 跳过 Restart。

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

// setupICSWithPublicIf 在已选定的公网侧网卡上启用 ICS（TUN 为私网侧）。
// PreferVPN（SkipAsSource）嵌在同一 PS：省第二次冷启；无 ICS 残留时跳过开头全机 Disable 预清。
// COM EnableSharing 脚本见 winnet.PSSnippetICSEnableSharing（内含无条件 SharedAccess Restart）。
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
	tunAlias := winnet.ResolveInterfaceAlias(tunName)

	vpn := ""
	if tunIP != nil {
		if v4 := tunIP.To4(); v4 != nil {
			vpn = v4.String()
		}
	}

	// 无残留：跳过脚本开头全机 DisableSharing 预清（Try-Enable 前仍会清一次）
	preClear := ""
	if winnet.HasICSResidue(tunName) {
		preClear = winnet.PSSnippetICSDisableSharingLoop()
		logger.Info("windows: ics_enable pre_clear=full_disable reason=residue")
	} else {
		logger.Info("windows: ics_enable pre_clear=skip reason=no_residue")
	}

	preferSnippet := ""
	if vpn != "" {
		preferSnippet = winnet.PSSnippetPreferVPNAfterICS(vpn, tunIfIndex)
	}

	enableStart := time.Now()
	ps := winnet.PSSnippetICSEnableSharing(
		lanIf, tunName, tunIfIndex, tunAlias,
		preClear,
		preferSnippet,
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
	preferEmbedded := false
	preferWaitMS := ""
	sawRestart := false
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ics_sharedaccess ") {
			logger.Info("windows: %s", line)
			if strings.Contains(line, "action=restart") {
				sawRestart = true
			}
		}
		if strings.HasPrefix(line, "ics_src_diag ") {
			logger.Info("windows: %s", line)
			preferEmbedded = true
		}
		if strings.HasPrefix(line, "ics_prefer_vpn ") {
			logger.Info("windows: %s", line)
			preferEmbedded = true
			preferWaitMS = strings.TrimPrefix(line, "ics_prefer_vpn ")
		}
		if strings.HasPrefix(line, "ics_prefix_keep ") || strings.HasPrefix(line, "ics_default_route_scrubbed ") {
			logger.Info("windows: %s", line)
			preferEmbedded = true
		}
	}
	if !sawRestart {
		logger.Warn("windows: ics_enable 未见到 SharedAccess Restart 日志（脚本应无条件 Restart）")
	}
	logger.Info("ics_enable elapsed=%s public=%s private=%s prefer_embedded=%v sharedaccess_restart=%v",
		enableElapsed, lanIf, tunName, preferEmbedded, sawRestart)
	logger.Info("windows: ICS 已启用 public=%s private=%s（VPN→LAN NAT 回退）", lanIf, tunName)
	winnet.RememberICSPair(lanIf, tunName)
	logger.Info("windows: ics_link_risk public=%s note=EnableSharing_may_drop_tunnel", lanIf)

	if vpn != "" && preferEmbedded {
		logger.Info("ics_prefer_vpn embedded=true %s", preferWaitMS)
		logger.Info("windows: ICS 后已 SkipAsSource 非 VPN 地址，本机发包源优先 %s", vpn)
	} else if vpn != "" {
		preferStart := time.Now()
		if err := winnet.PreferVPNSourceWithICSContext(ctx, tunName, vpn); err != nil {
			logger.Warn("windows: ICS 后 PreferVPNSource 失败（本机 AllowedIPs 可能仍异常）: %v", err)
		} else {
			logger.Info("ics_prefer_vpn embedded=false wait=fallback elapsed=%s", time.Since(preferStart))
			logger.Info("windows: ICS 后已 SkipAsSource 非 VPN 地址，本机发包源优先 %s", vpn)
		}
	}
	if tunIfIndex > 0 {
		if _, e := winnet.DeleteDefaultRouteOnInterface(tunIfIndex, winnet.ScrubDefaultRouteLate); e != nil {
			logger.Warn("tun_default_route_scrub after_ics ifIndex=%d: %v", tunIfIndex, e)
		}
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
