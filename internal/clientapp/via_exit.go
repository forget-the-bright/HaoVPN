package clientapp

import (
	"fmt"
	"net"
	"os"
	"strings"

	"haovpn/internal/logger"
	"haovpn/internal/netstack"
	"haovpn/internal/persist"
	"haovpn/internal/winnet"
)

// viaExit 客户端作托管路由 via 时的出口（复用 netstack.Stack）。
//
// 仅当 local_lans 非空且曾 Setup 成功时持有 stack；断线 Teardown。
type viaExit struct {
	stack *netstack.Stack
}

// setupViaExitLocked 按 local_lans 配置 VPN→LAN 转发/SNAT；空列表则跳过。
// 调用方须已持有 rt.mu。
func (rt *runtime) setupViaExitLocked(vpnSubnet, tunName, tunIP string, localLANs []string) error {
	rt.teardownViaExitLocked()

	lans := persist.ValidLANCIDRs(localLANs)
	if len(lans) == 0 {
		// 关闭 via：拆 ICS 残留，去掉 192.168.137.x，只留 VPN IP
		cleanupTUNAfterViaDisabled(tunName, tunIP)
		logger.Info("via_exit skipped local_lans empty")
		return nil
	}
	vpnSubnet = strings.TrimSpace(vpnSubnet)
	if vpnSubnet == "" {
		logger.Warn("via_exit skipped vpn_subnet empty（握手未下发）")
		return fmt.Errorf("via 出口需要握手下发 vpn_subnet")
	}
	ip := net.ParseIP(strings.TrimSpace(tunIP))
	if ip == nil {
		return fmt.Errorf("via 出口 tunIP 无效")
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
		return err
	}
	// Setup 内 ICS 已 PreferVPNSource；此处再加固一次（WinNAT 成功路径无 ICS 时无害）
	if err := winnet.PreferVPNSourceWithICS(tunName, tunIP); err != nil {
		logger.Warn("via_exit PreferVPNSource: %v", err)
	}
	rt.via = &viaExit{stack: st}
	logger.Info("via_exit_setup ok snat=%v lans=%v subnet=%s tun=%s", st.SNATEnabled(), lans, vpnSubnet, tunName)
	return nil
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
}

func (rt *runtime) teardownViaExitLocked() {
	if rt.via == nil || rt.via.stack == nil {
		rt.via = nil
		return
	}
	rt.via.stack.Teardown()
	logger.Info("via_exit_teardown done")
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
