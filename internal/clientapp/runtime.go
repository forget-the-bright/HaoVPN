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
	"haovpn/internal/tunnel"
	"haovpn/internal/tun"
)

// runtime 客户端 TUN 设备与系统路由/DNS 的运行时状态（Engine 内聚，不对外导出）。
//
// 字段：
//   mu — 保护 tunDev、routes、allowedCIDRs、vpnIP、policyVer、gateway、via、appliedDNS。
//   cfg — 客户端配置引用，用于 TUN 名、MTU、DNS 开关等。
//   tunDev — 已打开的 TUN 设备；vpnIP 变化时可能关闭并重建。
//   routes — 已通过 netstack 添加的路由 CIDR 列表（规范化），断线临时重连时保留。
//   allowedCIDRs — 最近一次握手策略中的 AllowedIPs，供杀开关前缀使用。
//   vpnIP — 当前 TUN 绑定的虚拟 IP。
//   policyVer — 服务端策略版本号，变更时打日志。
//   gateway — 当前用于 AddClientRoute 的下一跳网关 IP。
//   via — local_lans 非空时的 via 出口 Stack；空配置时为 nil。
//   viaFP — 当前已 Setup 的 via 指纹；与握手配置相同则跳过 ICS 重建。
//   appliedDNS — 最近一次成功写入的 DNS 列表（客户端侧缓存）。
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
	viaFP        string
	viaFPKnown   bool // 是否已成功应用过 via 状态（区分「从未应用」与「via 关闭」）
	appliedDNS   []string
	exitLANNets  []*net.IPNet
}

// allowedIPs 返回 AllowedIPs 副本，供杀开关 Enable 使用。
func (rt *runtime) allowedIPs() []string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return append([]string{}, rt.allowedCIDRs...)
}

// applyPolicy 按握手策略创建/更新 TUN、路由与 DNS（增量：仅改差异部分）。
//
// 参数：policy — 服务端 handshake_ok 下发的权威策略；VPNIP 非空。
// 返回：err 为 TUN 创建失败或 via Setup 失败；路由/DNS 失败仅 Warn 不阻断。
// 副作用：可能关闭并重建 TUN、增删系统路由、修改网卡 DNS、Setup/Teardown via。
// 并发：调用方须持 Engine 锁或单 goroutine 调用；内部持 rt.mu。
//
// 装路由顺序：若预判本次会跑 via/ICS Setup，则先不装分流路由（ICS 会冲掉），
// Setup 成功后再清一次并全量安装；避免「装路由 → ICS → 再装路由」的重复开销。
func (rt *runtime) applyPolicy(policy tunnel.HandshakePolicy) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	// --- 阶段 1：校验 vpn_ip ---
	if policy.VPNIP == "" {
		return fmt.Errorf("握手未下发 vpn_ip")
	}

	mtu := netutil.ResolveMTU(policy.MTU, rt.cfg.Tun.MTU)
	gw := netutil.ResolveGateway(policy.GatewayIP, "", policy.VPNIP)
	desired := desiredClientRoutes(gw, policy.AllowedIPs)
	deferRoutes := rt.willViaSetupLocked(policy.VPNSubnet, policy.VPNIP, rt.cfg.LocalLANs)

	needRecreate := rt.tunDev == nil || rt.vpnIP != policy.VPNIP
	mode := "noop"
	addN, delN := 0, 0

	// --- 阶段 2：按需创建或重建 TUN；路由可推迟到 via 之后 ---
	if needRecreate {
		logger.Info("dataplane_clear reason=vpn_ip_change old=%s new=%s", rt.vpnIP, policy.VPNIP)
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
		rt.viaFP = ""
		rt.viaFPKnown = false
		rt.appliedDNS = nil
		rt.gateway = gw
		mode = "tun_recreate"
		if !deferRoutes {
			addN = rt.installRouteListLocked(desired, gw, rt.tunDev.Name())
		} else {
			logger.Info("policy_apply defer_routes reason=via_setup_pending")
		}
	} else {
		tunName := rt.tunDev.Name()
		oldGw := rt.gateway
		if oldGw == "" {
			oldGw = netutil.ResolveGateway("", "", rt.vpnIP)
		}
		gwChanged := oldGw != gw
		if gwChanged {
			// 下一跳变化：先用旧网关删掉；若将 via Setup 则新路由延后安装
			for _, cidr := range rt.routes {
				_ = netstack.DelClientRoute(cidr, tunName, oldGw)
			}
			delN = len(rt.routes)
			rt.routes = nil
			rt.gateway = gw
			mode = "routes_diff"
			if !deferRoutes {
				addN = rt.installRouteListLocked(desired, gw, tunName)
			} else {
				logger.Info("policy_apply defer_routes reason=via_setup_pending after_gw_change")
			}
		} else {
			rt.gateway = gw
			if !deferRoutes {
				a, d := rt.syncRoutesDiffLocked(desired, gw, tunName)
				addN, delN = a, d
				if addN > 0 || delN > 0 {
					mode = "routes_diff"
				}
			} else {
				// 将跑 ICS：不必先差分（随后全量重装）；保留现有条目供 Setup 期间尽量黑洞
				logger.Info("policy_apply defer_routes reason=via_setup_pending keep_existing_until_ics")
			}
		}
	}

	tunName := rt.tunDev.Name()

	// --- 阶段 3：更新策略版本与杀开关前缀 ---
	if policy.PolicyVer != rt.policyVer && rt.policyVer > 0 {
		logger.Info("策略已更新 policy_ver %d -> %d", rt.policyVer, policy.PolicyVer)
	}
	rt.policyVer = policy.PolicyVer
	rt.allowedCIDRs = append([]string{}, policy.AllowedIPs...)

	// --- 阶段 4：按配置应用 DNS（列表未变则跳过）---
	dnsChanged := false
	if rt.cfg.Tun.DNSFromPolicyEnabled() && len(policy.DNSServers) > 0 {
		if !dnsServersEqual(rt.appliedDNS, policy.DNSServers) {
			if err := netstack.ApplyDNS(tunName, policy.DNSServers); err != nil {
				logger.Warn("DNS 设置失败（未应用）adapter=%s: %v", tunName, err)
			} else {
				rt.appliedDNS = append([]string{}, policy.DNSServers...)
				dnsChanged = true
				logger.Info("dns_applied servers=%v adapter=%s", policy.DNSServers, tunName)
			}
		}
	}

	// --- 阶段 5：via 出口；Setup 成功后再装/重装分流路由 ---
	rt.cacheExitLANNetsLocked()
	viaDidSetup, err := rt.setupViaExitLocked(policy.VPNSubnet, tunName, policy.VPNIP, rt.cfg.LocalLANs)
	if err != nil {
		// via 失败：若曾推迟装路由，补装一次以便排障/短暂可用，随后 dataplaneFailed 仍会全清
		if deferRoutes && len(rt.routes) == 0 && rt.tunDev != nil {
			addN = rt.installRouteListLocked(desired, gw, tunName)
			logger.Warn("policy_apply via_fail_install_deferred_routes add=%d", addN)
		}
		return err
	}
	if viaDidSetup {
		rt.clearRoutesOnlyLocked()
		addN = rt.installRouteListLocked(desired, gw, tunName)
		delN = 0
		if mode == "noop" {
			mode = "via_rebuild"
		}
		logger.Info("via_exit 后已安装客户端分流路由（仅此一次）")
	} else if deferRoutes {
		// 预判与实际不一致（极少）：补做差分/安装
		logger.Warn("policy_apply defer_routes mismatch，补装路由")
		a, d := rt.syncRoutesDiffLocked(desired, gw, tunName)
		addN, delN = a, d
		if addN > 0 || delN > 0 {
			mode = "routes_diff"
		}
	}

	if mode == "noop" && dnsChanged {
		mode = "dns_only"
	}

	logger.Info("policy_apply mode=%s vpn_ip=%s add=%d del=%d defer_routes=%v policy_ver=%d gateway=%s mtu=%d allowed_ips=%v",
		mode, policy.VPNIP, addN, delN, deferRoutes, policy.PolicyVer, gw, mtu, policy.AllowedIPs)
	return nil
}

// installRouteListLocked 按期望列表全量安装路由；返回成功添加条数。调用方须已持 rt.mu。
func (rt *runtime) installRouteListLocked(desired []string, gw, tunName string) int {
	rt.routes = nil
	n := 0
	for _, cidr := range desired {
		if err := netstack.AddClientRoute(cidr, tunName, gw); err != nil {
			logger.Warn("添加路由 %s: %v", cidr, err)
			continue
		}
		rt.routes = append(rt.routes, cidr)
		n++
	}
	return n
}

// syncRoutesDiffLocked 对已装路由与期望做集合差分；返回 add/del 数量。调用方须已持 rt.mu。
func (rt *runtime) syncRoutesDiffLocked(desired []string, gw, tunName string) (addN, delN int) {
	add, del := routeSetDiff(rt.routes, desired)
	delSet := make(map[string]struct{}, len(del))
	for _, c := range del {
		delSet[c] = struct{}{}
		if err := netstack.DelClientRoute(c, tunName, gw); err != nil {
			logger.Warn("删除路由 %s: %v", c, err)
		}
	}
	keep := make([]string, 0, len(rt.routes))
	for _, c := range normalizeRouteList(rt.routes) {
		if _, drop := delSet[c]; drop {
			continue
		}
		keep = append(keep, c)
	}
	for _, c := range add {
		if err := netstack.AddClientRoute(c, tunName, gw); err != nil {
			logger.Warn("添加路由 %s: %v", c, err)
			continue
		}
		keep = append(keep, c)
	}
	rt.routes = normalizeRouteList(keep)
	return len(add), len(del)
}

// cacheExitLANNetsLocked 解析 local_lans 供 TUN 上送过滤；调用方须已持 rt.mu。
func (rt *runtime) cacheExitLANNetsLocked() {
	rt.exitLANNets = nil
	if rt.cfg == nil {
		return
	}
	// 与握手/出口一致：先 ValidLANCIDRs，再解析；空则关闭上送放宽
	lans := netutil.ValidLANCIDRs(rt.cfg.LocalLANs)
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
	return !netutil.CIDRListContainsIP(allowed, ip)
}

func (rt *runtime) clearRoutes() {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.clearRoutesLocked()
}

func (rt *runtime) clearRoutesLocked() {
	rt.teardownViaExitLocked()
	rt.viaFP = ""
	rt.viaFPKnown = false
	rt.clearRoutesOnlyLocked()
	rt.appliedDNS = nil
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
	start := time.Now()
	logger.Info("dataplane_clear reason=stop")
	rt.clearRoutesLocked()
	if rt.tunDev != nil {
		_ = rt.tunDev.Close()
		rt.tunDev = nil
	}
	rt.allowedCIDRs = nil
	logger.Info("dataplane_clear done elapsed=%s", time.Since(start))
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
	if dst != nil && (dst.IsMulticast() || dst.IsLinkLocalMulticast() || netutil.IsLimitedBroadcast(dst)) {
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
