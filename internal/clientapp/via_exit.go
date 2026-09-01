package clientapp

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"haovpn/internal/logger"
	"haovpn/internal/netstack"
	"haovpn/internal/netutil"
	"haovpn/internal/safeutil"
)

// viaExit 客户端作托管路由 via 时的出口（复用 netstack.Stack）。
//
// 仅当 local_lans 非空且曾 Setup 成功时持有 stack；Stop/策略失败时 Teardown。
// 临时断线重连若 via 指纹未变则保留，避免重复 ICS。
//
// ICS 多网卡提示：Setup 成功后经 takeICSHint → 主窗 LastError；登录页不做预检。
// 注册表：仅握手上报 local_lans（不发 post-auth sync）。
type viaExit struct {
	stack *netstack.Stack
}

// ICS 清理钩子（可测注入；生产默认指向 netstack 门面 → winnet）。
//
// 为何可替换：单测不得真跑十几秒 DisableAllICS COM；断言「无残留不清理 / hadVia 只清地址」。
// 为何不直接 import winnet：clientapp 只依赖 netstack 编排层。
// CleanupICSResidue 须带 ctx：Stop/登出取消时可 Kill 慢 PowerShell（勿再用无 Context 变体作默认）。
var (
	hasICSResidueFn      = netstack.HasICSResidue
	cleanupICSResidueFn  = netstack.CleanupICSResidueContext
	removeICSAddressesFn = netstack.RemoveICSAddressesKeepVPN
)

// willViaSetupLocked 预判本次 setupViaExitLocked 是否会真正执行 Stack.Setup。
//
// 若将 Setup（含 ICS），ICS 常改坏路由表，调用方应推迟装分流路由，只在 Setup 后再装一次。
// 调用方须已持 rt.mu。
func (rt *runtime) willViaSetupLocked(vpnSubnet, tunIP string, localLANs []string) bool {
	fp := viaFingerprint(localLANs, vpnSubnet, tunIP)
	if fp == "" {
		return false
	}
	if rt.viaFPKnown && fp == rt.viaFP && rt.via != nil && rt.via.stack != nil {
		return false
	}
	return true
}

// setupViaExitLocked 按 local_lans 配置 VPN→LAN 转发/SNAT；指纹未变则跳过。
//
// 返回：viaDidSetup — 本次是否真正执行了 Stack.Setup（ICS 可能改路由表，调用方须重装分流路由）。
// 调用方须已持有 rt.mu。
func (rt *runtime) setupViaExitLocked(ctx context.Context, vpnSubnet, tunName, tunIP string, localLANs []string) (viaDidSetup bool, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := safeutil.Check(ctx); err != nil {
		return false, err
	}
	fp := viaFingerprint(localLANs, vpnSubnet, tunIP)

	// 已成功应用过且指纹相同：via 开启且 stack 在，或 via 关闭 —— 均跳过（热路径 Debug，避免刷屏）
	if rt.viaFPKnown && fp == rt.viaFP {
		if fp == "" {
			logger.Debug("via_exit skipped unchanged (off)")
			return false, nil
		}
		if rt.via != nil && rt.via.stack != nil {
			logger.Debug("via_exit skipped unchanged fingerprint")
			return false, nil
		}
		// 指纹非空但 stack 丢失（异常）：落下落到重建
		logger.Warn("via_exit fingerprint set but stack missing，将重建")
	}

	// teardown：即将重建 via → ICSPreserve；关 via → ICSDisable。
	hadVia := rt.via != nil && rt.via.stack != nil
	lans := netutil.ValidLANCIDRs(localLANs)
	if len(lans) > 0 {
		rt.teardownViaExitLocked(netstack.ICSPreserve)
	} else {
		rt.teardownViaExitLocked(netstack.ICSDisable)
	}
	rt.viaFP = ""
	rt.viaFPKnown = false

	if len(lans) == 0 {
		// 关闭 via / 从未开 via：仅有 ICS 残留时才慢清理（公司机无 local_lans 常见无残留）
		cleanupTUNAfterViaDisabled(ctx, tunName, tunIP, hadVia)
		rt.viaFP = ""
		rt.viaFPKnown = true
		logger.Debug("via_exit skipped local_lans empty")
		return false, nil
	}
	vpnSubnet = strings.TrimSpace(vpnSubnet)
	if vpnSubnet == "" {
		logger.Warn("via_exit skipped vpn_subnet empty（握手未下发）")
		return false, fmt.Errorf("via 出口需要握手下发 vpn_subnet")
	}
	ipStr := strings.TrimSpace(tunIP)
	norm, err := netutil.NormalizeIPv4(ipStr)
	if err != nil {
		return false, fmt.Errorf("via 出口 tunIP 无效")
	}
	ip := netutil.ParseHostIPOrNil(norm)
	if ip == nil {
		return false, fmt.Errorf("via 出口 tunIP 无效")
	}
	// OutboundIf 仅 ICS 使用（WinNAT 忽略）；来自 client.yaml windows.outbound_interface
	outboundIf := ""
	if rt.cfg != nil {
		outboundIf = strings.TrimSpace(rt.cfg.Windows.OutboundInterface)
	}
	st := netstack.New(netstack.Config{
		TunName:     tunName,
		TunIP:       ip.To4(),
		VPNSubnet:   vpnSubnet,
		LanCIDRs:    lans,
		OutboundIf:  outboundIf,
		ForwardOnly: true,
		Enabled:     true,
	})
	// Setup(ctx)：Stop/HardRestart 取消时可 Kill ICS PowerShell（勿再塞 AbortCtx 进 Config）
	if err := st.Setup(ctx); err != nil {
		if safeutil.IsCanceled(err) {
			logger.Info("via_exit_setup aborted lans=%v err=%v", lans, err)
		} else {
			logger.Error("via_exit_setup fail lans=%v subnet=%s: %v", lans, vpnSubnet, err)
		}
		return false, err
	}
	// PreferVPN：ICS 路径在 Setup 内已做一次（等 137 挂上后 SkipAsSource）。
	// 勿在此处或装路由后再调（重复 PS 无益）。
	rt.via = &viaExit{stack: st}
	rt.viaFP = fp
	rt.viaFPKnown = true

	active := lans
	skipped := []string(nil)
	if st.UsedICS() {
		active = st.ICSActiveCIDRs()
		skipped = st.ICSSkippedCIDRs()
		// 多 LAN 有 skipped 时注册表仍保留握手全量；用户提示靠 icsHint，不发 post-auth sync。
		if hint := st.ICSLocalLANsHint(); hint != "" {
			rt.icsHint = hint
		}
	}
	logger.Info("via_exit_setup ok snat=%v active_lans=%v skipped_lans=%v ics=%v subnet=%s tun=%s",
		st.SNATEnabled(), active, skipped, st.UsedICS(), vpnSubnet, tunName)
	return true, nil
}

// cleanupTUNAfterViaDisabled 空 local_lans 时按残留智能清理 ICS（勿无条件 DisableAllICS）。
//
// 参数：
//   ctx — 可取消；传给 CleanupICSResidueContext（Stop 时可 Kill PS）；nil 视为 Background；
//   tunName / vpnIP — TUN 与须保留的 VPN IP；
//   hadVia — 本轮 teardown 前是否刚关过 via（Teardown 已关全机 ICS）。
// 策略：
//   - 无 192.168.137 残留 → 跳过（公司机主路径，亚秒探测）；
//   - 有残留且 hadVia → 只删 137（避免二次十几秒 COM）；
//   - 有残留且非 hadVia → CleanupICSResidueContext 一次 PS（Disable+清地址）。
func cleanupTUNAfterViaDisabled(ctx context.Context, tunName, vpnIP string, hadVia bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	probeStart := time.Now()
	hasResidue := hasICSResidueFn(tunName)
	logger.Info("via_cleanup HasICSResidue elapsed=%s hit=%v tun=%s hadVia=%v", time.Since(probeStart), hasResidue, tunName, hadVia)
	if !hasResidue {
		logger.Debug("via_exit skipped local_lans empty (no ICS residue) tun=%s", tunName)
		return
	}
	if hadVia {
		if err := removeICSAddressesFn(tunName, vpnIP); err != nil {
			logger.Warn("via_exit 清理 ICS 地址失败 reason=had_via_teardown: %v", err)
			return
		}
		logger.Info("via_exit 已清理 ICS 地址残留 reason=had_via_teardown tun=%s keep=%s", tunName, vpnIP)
		return
	}
	if err := cleanupICSResidueFn(ctx, tunName, vpnIP); err != nil {
		if safeutil.IsCanceled(err) {
			logger.Info("via_exit 清理 ICS 残留 aborted tun=%s err=%v", tunName, err)
			return
		}
		logger.Warn("via_exit 清理 ICS 残留失败 reason=residue: %v", err)
		return
	}
	logger.Info("via_exit 已清理 ICS 残留 reason=residue tun=%s keep=%s", tunName, vpnIP)
}

func (rt *runtime) teardownViaExitLocked(ics netstack.ICSLifecycle) {
	if rt.via == nil || rt.via.stack == nil {
		rt.via = nil
		return
	}
	start := time.Now()
	// 正常清数据面用 Background：runCtx 在 Stop 时已取消，若传入会导致 Disable ICS 被立刻跳过留下残留。
	if ics.Preserve() {
		_ = rt.via.stack.TeardownKeepICS(context.Background())
	} else {
		_ = rt.via.stack.Teardown(context.Background())
	}
	logger.Info("via_exit_teardown done elapsed=%s ics=%s", time.Since(start), ics.LogLabel())
	rt.via = nil
}

// clientHostID 生成可选主机标识（主机名），供注册表排障。
func clientHostID() string {
	h, err := os.Hostname()
	if err != nil {
		return ""
	}
	return h
}
