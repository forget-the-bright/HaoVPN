package clientapp

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"haovpn/internal/config"
	"haovpn/internal/logger"
	"haovpn/internal/netstack"
	"haovpn/internal/netutil"
	"haovpn/internal/persist"
	"haovpn/internal/tunnel"
	"haovpn/internal/tun"
)

// runtime 客户端 TUN 设备与系统路由/DNS 的运行时状态（Engine 内聚，不对外导出）。
//
// 字段：
//   mu — 保护 tunDev、routes、allowedCIDRs、vpnIP、policyVer、gateway。
//   cfg — 客户端配置引用，用于 TUN 名、MTU、DNS 开关等。
//   tunDev — 已打开的 TUN 设备；vpnIP 变化时可能关闭并重建。
//   routes — 已通过 netstack 添加的路由 CIDR 列表，断线或 close 时逐条删除。
//   allowedCIDRs — 最近一次握手策略中的 AllowedIPs，供杀开关前缀使用。
//   vpnIP — 当前 TUN 绑定的虚拟 IP。
//   policyVer — 服务端策略版本号，变更时打日志。
//   gateway — 当前用于 AddClientRoute 的下一跳网关 IP。
//   via — local_lans 非空时的 via 出口 Stack；空配置时为 nil。
//   exitLANNets — 解析后的 local_lans，供 TUN 上送过滤（允许 LAN 回程源）。
type runtime struct {
	mu           sync.Mutex
	cfg          *config.ClientConfig
	tunDev       tun.Device
	routes       []string
	allowedCIDRs []string
	vpnIP        string
	policyVer    int
	gateway      string
	via          *viaExit
	exitLANNets  []*net.IPNet
}

// allowedIPs 返回 AllowedIPs 副本，供杀开关 Enable 使用。
func (rt *runtime) allowedIPs() []string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return append([]string{}, rt.allowedCIDRs...)
}

// applyPolicy 按握手策略创建/更新 TUN、路由与 DNS。
//
// 参数：policy — 服务端 handshake_ok 下发的权威策略；VPNIP 非空。
// 返回：err 为 TUN 创建失败；路由/DNS 失败仅 Warn 不阻断。
// 副作用：可能关闭并重建 TUN、增删系统路由、修改网卡 DNS；更新 rt 内部状态。
// 并发：调用方须持 Engine 锁或单 goroutine 调用；内部持 rt.mu。
func (rt *runtime) applyPolicy(policy tunnel.HandshakePolicy) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	// --- 阶段 1：校验 vpn_ip ---
	if policy.VPNIP == "" {
		return fmt.Errorf("握手未下发 vpn_ip")
	}

	mtu := netutil.ResolveMTU(policy.MTU, rt.cfg.Tun.MTU)

	// --- 阶段 2：按需创建或重建 TUN 设备 ---
	needRecreate := rt.tunDev == nil || rt.vpnIP != policy.VPNIP
	if needRecreate {
		if rt.tunDev != nil {
			rt.clearRoutesLocked()
			_ = rt.tunDev.Close()
			rt.tunDev = nil
		}
		dev, err := tun.Open(tun.Config{
			Name: rt.cfg.Tun.Name,
			MTU:  mtu,
			CIDR: policy.VPNIP + "/32",
		})
		if err != nil {
			return fmt.Errorf("TUN 创建失败: %w", err)
		}
		rt.tunDev = dev
		rt.vpnIP = policy.VPNIP
		rt.routes = nil
	} else {
		rt.clearRoutesOnlyLocked()
	}

	// --- 阶段 3：添加网关路由与 AllowedIPs 分流路由 ---
	gw := netutil.ResolveGateway(policy.GatewayIP, "", policy.VPNIP)
	rt.gateway = gw
	tunName := rt.tunDev.Name()
	rt.installClientRoutesLocked(policy.AllowedIPs, gw, tunName)

	// --- 阶段 4：更新策略版本与杀开关前缀 ---
	if policy.PolicyVer != rt.policyVer && rt.policyVer > 0 {
		logger.Info("策略已更新 policy_ver %d -> %d", rt.policyVer, policy.PolicyVer)
	}
	rt.policyVer = policy.PolicyVer
	rt.allowedCIDRs = append([]string{}, policy.AllowedIPs...)
	logger.Info("已应用服务端策略 vpn_ip=%s allowed_ips=%v policy_ver=%d gateway=%s mtu=%d",
		policy.VPNIP, policy.AllowedIPs, policy.PolicyVer, gw, mtu)

	// --- 阶段 5：按配置应用 DNS ---
	if rt.cfg.Tun.DNSFromPolicyEnabled() && len(policy.DNSServers) > 0 {
		if err := netstack.ApplyDNS(tunName, policy.DNSServers); err != nil {
			logger.Warn("DNS 设置失败（未应用）adapter=%s: %v", tunName, err)
		} else {
			logger.Info("dns_applied servers=%v adapter=%s", policy.DNSServers, tunName)
		}
	}

	// --- 阶段 6：via 出口（local_lans 非空才 Setup；ICS 可能改路由表）---
	rt.cacheExitLANNetsLocked()
	if err := rt.setupViaExitLocked(policy.VPNSubnet, tunName, policy.VPNIP, rt.cfg.LocalLANs); err != nil {
		return err
	}
	// ICS 启用后重装分流路由，避免 AllowedIPs 被冲掉
	if len(rt.cfg.LocalLANs) > 0 {
		rt.clearRoutesOnlyLocked()
		rt.installClientRoutesLocked(policy.AllowedIPs, gw, tunName)
		logger.Info("via_exit 后已重装客户端分流路由")
	}
	return nil
}

// installClientRoutesLocked 安装网关 /32（按需）与 AllowedIPs；调用方须已持 rt.mu。
func (rt *runtime) installClientRoutesLocked(allowed []string, gw, tunName string) {
	rt.routes = nil
	if gw != "" && gatewayHostRouteNeeded(gw, allowed) {
		gwCIDR := gw + "/32"
		if err := netstack.AddClientRoute(gwCIDR, tunName, gw); err != nil {
			logger.Warn("添加网关路由 %s: %v", gwCIDR, err)
		} else {
			rt.routes = append(rt.routes, gwCIDR)
		}
	}
	for _, cidr := range allowed {
		if gw != "" && cidr == gw+"/32" {
			continue
		}
		if err := netstack.AddClientRoute(cidr, tunName, gw); err != nil {
			logger.Warn("添加路由 %s: %v", cidr, err)
		} else {
			rt.routes = append(rt.routes, cidr)
		}
	}
}

// cacheExitLANNetsLocked 解析 local_lans 供 TUN 上送过滤；调用方须已持 rt.mu。
func (rt *runtime) cacheExitLANNetsLocked() {
	rt.exitLANNets = nil
	if rt.cfg == nil {
		return
	}
	// 与握手/出口一致：先 ValidLANCIDRs，再解析；空则关闭上送放宽
	lans := persist.ValidLANCIDRs(rt.cfg.LocalLANs)
	if len(lans) == 0 {
		return
	}
	nets, err := netutil.ParseCIDRListToNets(lans)
	if err != nil {
		return
	}
	rt.exitLANNets = nets
}

// gatewayHostRouteNeeded 判断是否需单独添加网关主机路由（/32）。
// 若 AllowedIPs 中已有 CIDR 包含网关 IP（如 10.88.0.0/24 含 10.88.0.1），则不必再装。
func gatewayHostRouteNeeded(gw string, allowed []string) bool {
	ip := net.ParseIP(strings.TrimSpace(gw))
	if ip == nil {
		return false
	}
	ip = ip.To4()
	if ip == nil {
		return false
	}
	for _, c := range allowed {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			if hip := net.ParseIP(c); hip != nil && hip.To4() != nil {
				_, n, err = net.ParseCIDR(hip.String() + "/32")
			}
		}
		if err != nil || n == nil {
			continue
		}
		if n.Contains(ip) {
			return false
		}
	}
	return true
}

func (rt *runtime) clearRoutes() {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.clearRoutesLocked()
}

func (rt *runtime) clearRoutesLocked() {
	rt.teardownViaExitLocked()
	rt.clearRoutesOnlyLocked()
	if rt.tunDev == nil {
		return
	}
	_ = netstack.RestoreDNS(rt.tunDev.Name())
}

func (rt *runtime) clearRoutesOnlyLocked() {
	if rt.tunDev == nil {
		return
	}
	tunName := rt.tunDev.Name()
	gw := rt.gateway
	if gw == "" {
		gw = netutil.ResolveGateway("", "", rt.vpnIP)
	}
	for _, cidr := range rt.routes {
		_ = netstack.DelClientRoute(cidr, tunName, gw)
	}
	rt.routes = nil
}

func (rt *runtime) close() {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.clearRoutesLocked()
	if rt.tunDev != nil {
		_ = rt.tunDev.Close()
		rt.tunDev = nil
	}
	rt.allowedCIDRs = nil
}

func (rt *runtime) write(pkt []byte) error {
	rt.mu.Lock()
	dev := rt.tunDev
	rt.mu.Unlock()
	if dev == nil {
		return nil
	}
	_, err := dev.Write(pkt)
	return err
}

func (rt *runtime) readLoop(ctx context.Context, send func([]byte) error, mtu int) {
	buf := make([]byte, netutil.ReadBufferSize(mtu))
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		rt.mu.Lock()
		dev := rt.tunDev
		rt.mu.Unlock()
		if dev == nil {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		n, err := dev.Read(buf)
		if err != nil {
			logger.Warn("TUN 读错误: %v", err)
			time.Sleep(200 * time.Millisecond)
			continue
		}
		pkt := buf[:n]
		if !rt.shouldUploadTUN(pkt) {
			continue
		}
		if err := send(pkt); err != nil {
			logger.Warn("隧道发送失败: %v", err)
		}
	}
}

// shouldUploadTUN 仅上送合法 IPv4：源为本机 VPN IP，或落在 local_lans（via 回程）。
// 过滤 ICS(192.168.137.x)、广播/组播等误注入，避免服务端伪造源刷屏。
func (rt *runtime) shouldUploadTUN(pkt []byte) bool {
	if len(pkt) < 20 || pkt[0]>>4 != 4 {
		return false
	}
	src := net.IP(pkt[12:16])
	dst := net.IP(pkt[16:20])
	if dst != nil && (dst.IsMulticast() || dst.IsLinkLocalMulticast() || isAllOnesBroadcast(dst)) {
		return false
	}
	rt.mu.Lock()
	vpnIP := rt.vpnIP
	lans := rt.exitLANNets
	rt.mu.Unlock()
	if vpnIP != "" && src.String() == vpnIP {
		return true
	}
	for _, n := range lans {
		if n != nil && n.Contains(src) {
			return true
		}
	}
	return false
}

// isAllOnesBroadcast IPv4 受限广播 255.255.255.255。
func isAllOnesBroadcast(ip net.IP) bool {
	v4 := ip.To4()
	return v4 != nil && v4[0] == 255 && v4[1] == 255 && v4[2] == 255 && v4[3] == 255
}
