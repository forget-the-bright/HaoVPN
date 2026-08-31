package clientapp

import (
	"fmt"
	"os"
	"strings"
	"time"

	"haovpn/internal/logger"
	"haovpn/internal/netstack"
	"haovpn/internal/netutil"
	"haovpn/internal/winnet"
)

// viaExit 客户端作托管路由 via 时的出口（复用 netstack.Stack）。
//
// 仅当 local_lans 非空且曾 Setup 成功时持有 stack；Stop/策略失败时 Teardown。
// 临时断线重连若 via 指纹未变则保留，避免重复 ICS。
type viaExit struct {
	stack *netstack.Stack
}

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
func (rt *runtime) setupViaExitLocked(vpnSubnet, tunName, tunIP string, localLANs []string) (viaDidSetup bool, err error) {
	fp := viaFingerprint(localLANs, vpnSubnet, tunIP)

	// 已成功应用过且指纹相同：via 开启且 stack 在，或 via 关闭 —— 均跳过
	if rt.viaFPKnown && fp == rt.viaFP {
		if fp == "" {
			logger.Info("via_exit skipped unchanged (off)")
			return false, nil
		}
		if rt.via != nil && rt.via.stack != nil {
			logger.Info("via_exit skipped unchanged fingerprint")
			return false, nil
		}
		// 指纹非空但 stack 丢失（异常）：落下落到重建
		logger.Warn("via_exit fingerprint set but stack missing，将重建")
	}

	rt.teardownViaExitLocked()
	rt.viaFP = ""
	rt.viaFPKnown = false

	lans := netutil.ValidLANCIDRs(localLANs)
	if len(lans) == 0 {
		// 关闭 via：拆 ICS 残留，去掉 192.168.137.x，只留 VPN IP
		cleanupTUNAfterViaDisabled(tunName, tunIP)
		rt.viaFP = ""
		rt.viaFPKnown = true
		logger.Info("via_exit skipped local_lans empty")
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
	// 家庭版走 ICS：保留 ICS，但须在 Setup 后把 ICS 地址 SkipAsSource，否则本机访 AllowedIPs 会错源
	st := netstack.New(netstack.Config{
		TunName:     tunName,
		TunIP:       ip.To4(),
		VPNSubnet:   vpnSubnet,
		LanCIDRs:    lans,
		ForwardOnly: true,
		Enabled:     true,
	})
	if err := st.Setup(); err != nil {
		logger.Error("via_exit_setup fail lans=%v subnet=%s: %v", lans, vpnSubnet, err)
		return false, err
	}
	// Setup 内 ICS 已 PreferVPNSource；此处再加固一次（WinNAT 成功路径无 ICS 时无害）
	if err := winnet.PreferVPNSourceWithICS(tunName, tunIP); err != nil {
		logger.Warn("via_exit PreferVPNSource: %v", err)
	}
	rt.via = &viaExit{stack: st}
	rt.viaFP = fp
	rt.viaFPKnown = true
	logger.Info("via_exit_setup ok snat=%v lans=%v subnet=%s tun=%s", st.SNATEnabled(), lans, vpnSubnet, tunName)
	return true, nil
}

// cleanupTUNAfterViaDisabled via 关闭时关掉 ICS 并清掉 ICS 私网地址。
func cleanupTUNAfterViaDisabled(tunName, vpnIP string) {
	winnet.DisableAllICS()
	if err := winnet.RemoveICSAddressesKeepVPN(tunName, vpnIP); err != nil {
		logger.Warn("via_exit 清理 ICS 地址失败: %v", err)
		return
	}
	logger.Info("via_exit 已清理 ICS 残留 tun=%s keep=%s", tunName, vpnIP)
}

func (rt *runtime) teardownViaExit() {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.teardownViaExitLocked()
	rt.viaFP = ""
	rt.viaFPKnown = false
}

func (rt *runtime) teardownViaExitLocked() {
	if rt.via == nil || rt.via.stack == nil {
		rt.via = nil
		return
	}
	start := time.Now()
	rt.via.stack.Teardown()
	logger.Info("via_exit_teardown done elapsed=%s", time.Since(start))
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
