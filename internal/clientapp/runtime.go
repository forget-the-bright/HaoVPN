package clientapp

import (
	"context"
	"fmt"
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
//   mu — 保护 tunDev、routes、allowedCIDRs、vpnIP、policyVer、gateway。
//   cfg — 客户端配置引用，用于 TUN 名、MTU、DNS 开关等。
//   tunDev — 已打开的 TUN 设备；vpnIP 变化时可能关闭并重建。
//   routes — 已通过 netstack 添加的路由 CIDR 列表，断线或 close 时逐条删除。
//   allowedCIDRs — 最近一次握手策略中的 AllowedIPs，供杀开关前缀使用。
//   vpnIP — 当前 TUN 绑定的虚拟 IP。
//   policyVer — 服务端策略版本号，变更时打日志。
//   gateway — 当前用于 AddClientRoute 的下一跳网关 IP。
type runtime struct {
	mu           sync.Mutex
	cfg          *config.ClientConfig
	tunDev       tun.Device
	routes       []string
	allowedCIDRs []string
	vpnIP        string
	policyVer    int
	gateway      string
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
	if gw != "" {
		gwCIDR := gw + "/32"
		if err := netstack.AddClientRoute(gwCIDR, tunName, gw); err != nil {
			logger.Warn("添加网关路由 %s: %v", gwCIDR, err)
		} else {
			rt.routes = append(rt.routes, gwCIDR)
		}
	}
	for _, cidr := range policy.AllowedIPs {
		if gw != "" && cidr == gw+"/32" {
			continue
		}
		if err := netstack.AddClientRoute(cidr, tunName, gw); err != nil {
			logger.Warn("添加路由 %s: %v", cidr, err)
		} else {
			rt.routes = append(rt.routes, cidr)
		}
	}

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
	return nil
}

func (rt *runtime) clearRoutes() {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.clearRoutesLocked()
}

func (rt *runtime) clearRoutesLocked() {
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
		if err := send(buf[:n]); err != nil {
			logger.Warn("隧道发送失败: %v", err)
		}
	}
}
