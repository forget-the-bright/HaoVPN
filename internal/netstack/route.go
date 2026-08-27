// Package netstack 提供跨平台路由与 NAT（服务端网关 / 客户端分流）。
//
// 服务端职责（Enabled=true）：
//  1. 打开 IP 转发；
//  2. 对 VPN 子网做 SNAT/MASQUERADE，使客户端可访问 allowed_lan_cidrs。
// 客户端职责：
//  将 AllowedIPs 路由到 VPN 网关（服务端 TUN IP）。
package netstack

import (
	"fmt"
	"net"
	"strings"

	"haovpn/internal/logger"
)

// Config 服务端 NAT / 转发参数。
type Config struct {
	TunName     string   // TUN 网卡名，如 haovpn0
	TunIP       net.IP   // 服务端 TUN IP（网关）
	VPNSubnet   string   // VPN 地址池，如 10.88.0.0/24（SNAT 源）
	LanCIDRs    []string // 允许访问的工控/局域网网段
	OutboundIf  string   // 可选：ICS/出站网卡名
	ForwardOnly bool     // SNAT 失败时仅转发、服务继续（health nat_ok=false）
	Enabled     bool     // 是否启用转发+NAT
}

// Stack 管理服务端路由与 NAT 生命周期。
type Stack struct {
	cfg         Config
	snatEnabled bool // 是否已成功配置 SNAT/MASQUERADE
}

// New 创建 netstack 管理器。
func New(cfg Config) *Stack {
	return &Stack{cfg: cfg}
}

// SNATEnabled 是否已成功配置 SNAT（forward_only 且无 SNAT 时为 false）。
func (s *Stack) SNATEnabled() bool {
	return s.snatEnabled
}

// Setup 启用转发并为 VPN→LAN 配置 NAT。
func (s *Stack) Setup() error {
	if !s.cfg.Enabled {
		logger.Info("netstack: NAT/转发已关闭（nat.enabled=false）")
		return nil
	}
	if s.cfg.VPNSubnet == "" {
		return fmt.Errorf("netstack: VPNSubnet 为空，无法配置 SNAT")
	}
	if err := enableIPForwardPlatform(); err != nil {
		return fmt.Errorf("启用 IP 转发失败: %w", err)
	}
	logger.Info("netstack: IP 转发已开启")

	var errs []string
	okCount := 0
	for _, lan := range s.cfg.LanCIDRs {
		if err := setupNATPlatform(s.cfg.VPNSubnet, lan, s.cfg.TunName, s.cfg.TunIP, s.cfg.OutboundIf); err != nil {
			errs = append(errs, fmt.Sprintf("NAT %s→%s: %v", s.cfg.VPNSubnet, lan, err))
			logger.Error("netstack NAT 失败: %v", err)
			continue
		}
		okCount++
		logger.Info("netstack: NAT 已配置 VPN %s → LAN %s", s.cfg.VPNSubnet, lan)
	}
	s.snatEnabled = okCount > 0
	if len(s.cfg.LanCIDRs) == 0 {
		logger.Warn("netstack: allowed_lan_cidrs 为空，仅开启转发、未加 SNAT 规则")
	}
	if len(errs) > 0 && len(errs) == len(s.cfg.LanCIDRs) {
		if s.cfg.ForwardOnly {
			logger.Warn("netstack: 全部 SNAT 失败，forward_only=true，仅 IP 转发继续（health nat_ok=false）")
			logger.Warn("netstack: 可测隧道/ ping 网关 10.88.0.1；访问其它 LAN 需 WinNAT(Hyper-V)/Linux，或在「网络连接→WLAN→共享」手工启用 ICS 后重启")
			logger.Info("netstack setup 完成（无 SNAT） lan_count=%d", len(s.cfg.LanCIDRs))
			return nil
		}
		return fmt.Errorf("全部 NAT 规则失败: %s", strings.Join(errs, "; "))
	}
	logger.Info("netstack setup 完成 lan_count=%d snat=%v", len(s.cfg.LanCIDRs), s.snatEnabled)
	return nil
}

// ProbeIPForCIDR 取 LAN 网段内用于路由探测的 IP（通常为 network+1）。
func ProbeIPForCIDR(lanCIDR string) string {
	ip, ipnet, err := net.ParseCIDR(lanCIDR)
	if err != nil {
		return "192.168.1.1"
	}
	probe := ip.Mask(ipnet.Mask).To4()
	if probe == nil {
		return ip.String()
	}
	if probe[3] < 254 {
		probe[3]++
	}
	return probe.String()
}

// Teardown 拆除 NAT 规则（尽力而为）。
func (s *Stack) Teardown() error {
	if !s.cfg.Enabled {
		return nil
	}
	for _, lan := range s.cfg.LanCIDRs {
		if err := teardownNATPlatform(s.cfg.VPNSubnet, lan, s.cfg.TunName); err != nil {
			logger.Warn("netstack teardown NAT %s: %v", lan, err)
		}
	}
	logger.Info("netstack teardown 完成")
	return nil
}

// AddClientRoute 在客户端把 cidr 导入 VPN 隧道网卡。
// Windows：on-link（0.0.0.0 IF）；Linux/macOS：via gateway。
func AddClientRoute(cidr, tunName, gateway string) error {
	if cidr == "" || tunName == "" || gateway == "" {
		return fmt.Errorf("AddClientRoute: cidr/tunName/gateway 均不能为空")
	}
	if err := addClientRoutePlatform(cidr, tunName, gateway); err != nil {
		return err
	}
	logger.Info("客户端路由已添加: %s on-link (%s)", cidr, tunName)
	return nil
}

// DelClientRoute 删除客户端分流路由。
func DelClientRoute(cidr, tunName, gateway string) error {
	return delClientRoutePlatform(cidr, tunName, gateway)
}

// ParseCIDRs 校验 CIDR 列表。
func ParseCIDRs(cidrs []string) ([]*net.IPNet, error) {
	var out []*net.IPNet
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q: %w", c, err)
		}
		out = append(out, n)
	}
	return out, nil
}

// WindowsOnLinkRouteArgs 构造 Windows on-link 路由参数（route ADD … 0.0.0.0 IF n）。
// 放在无 build tag 文件中便于单测；仅 Windows 平台 addClientRoute 调用。
func WindowsOnLinkRouteArgs(dest, mask string, ifIndex int) []string {
	return []string{"ADD", dest, "MASK", mask, "0.0.0.0", "IF", fmt.Sprintf("%d", ifIndex)}
}

// SplitCIDR 将 CIDR 拆成目标网络与掩码字符串（Windows route 命令用）。
func SplitCIDR(cidr string) (dest, mask string, err error) {
	ip, n, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", "", err
	}
	ones, bits := n.Mask.Size()
	if bits != 32 {
		return "", "", fmt.Errorf("仅支持 IPv4: %s", cidr)
	}
	_ = ones
	return ip.Mask(n.Mask).String(), net.IP(n.Mask).String(), nil
}
