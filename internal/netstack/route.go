package netstack

import (
	"fmt"
	"net"
	"strings"

	"haovpn/internal/logger"
	"haovpn/internal/netutil"
)

// Config 服务端 NAT / 转发所需参数（来自 config.NATSection 与 TUN 信息）。
//
// 字段：
//   TunName — TUN 网卡配置名（如 haovpn0），Windows ICS 回退时定位私网侧接口。
//   TunIP — 服务端 TUN 网关 IPv4，部分平台 NAT 规则引用。
//   VPNSubnet — VPN 地址池 CIDR（SNAT 源网段，如 10.88.0.0/24）。
//   LanCIDRs — 允许 VPN 客户端访问的工控/局域网段列表。
//   OutboundIf — 可选；Windows ICS 公网侧网卡名，空则自动探测。
//   ForwardOnly — SNAT 全部失败时是否仍仅开 IP 转发（health nat_ok=false）。
//   Enabled — 对应 nat.enabled；false 时 Setup/Teardown 均为无操作。
type Config struct {
	TunName     string   // TUN 网卡名，如 haovpn0
	TunIP       net.IP   // 服务端 TUN IP（网关）
	VPNSubnet   string   // VPN 地址池，如 10.88.0.0/24（SNAT 源）
	LanCIDRs    []string // 允许访问的工控/局域网网段
	OutboundIf  string   // 可选：ICS/出站网卡名
	ForwardOnly bool     // SNAT 失败时仅转发、服务继续（health nat_ok=false）
	Enabled     bool     // 是否启用转发+NAT
}

// Stack 管理服务端 IP 转发与 VPN→LAN 的 NAT 规则生命周期。
//
// 由 serverapp 在 TUN 就绪后创建；Setup 写入平台规则，Teardown 尽力拆除。
// 线程安全：实例方法非并发安全，须由单 goroutine（服务端主流程）调用。
type Stack struct {
	cfg         Config
	snatEnabled bool // 是否已成功配置 SNAT/MASQUERADE
}

// New 创建 netstack 管理器，不触发任何系统变更。
//
// 参数：cfg — 来自服务端配置与 TUN 运行时信息。
// 返回：未调用 Setup 的 Stack 指针。
func New(cfg Config) *Stack {
	return &Stack{cfg: cfg}
}

// SNATEnabled 报告最近一次 Setup 是否至少成功配置一条 SNAT/MASQUERADE。
//
// 返回：forward_only 且无 SNAT 时为 false；供 health 上报 nat_ok。
func (s *Stack) SNATEnabled() bool {
	return s.snatEnabled
}

// Setup 启用系统 IP 转发并为每个 LanCIDR 配置 VPN→LAN 的 NAT。
//
// 参数：无（使用 New 时的 Config）。
// 返回：Enabled=false 时 nil；全部 SNAT 失败且非 forward_only 时 error。
// 副作用：写 iptables/WinNAT/ICS、注册表 IPEnableRouter 等（依平台）。
// 并发：非并发安全。
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

// ProbeIPForCIDR 取 LAN 网段内用于路由/ICS 探测的代表性 IPv4（通常为 network+1）。
//
// 参数：lanCIDR — 工控网段 CIDR；解析失败时回退 192.168.1.1。
// 返回：可用于 Find-NetRoute 等探测的目标 IP 字符串。
func ProbeIPForCIDR(lanCIDR string) string {
	ip, ipnet, err := netutil.ParseCIDR(lanCIDR)
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

// Teardown 按 Config 拆除已配置的 NAT 规则（尽力而为，失败仅打日志）。
//
// 返回：Enabled=false 时 nil；个别规则删除失败不阻断整体返回。
// 副作用：删除 iptables/NetNat/ICS 等（依平台）。
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

// AddClientRoute 在客户端把 cidr 导入 VPN 隧道网卡（分流路由）。
//
// 参数：
//   cidr — 目标网段 CIDR，须为 IPv4。
//   tunName — TUN/Wintun 配置名或系统网卡名。
//   gateway — 下一跳；Windows 走 on-link 时仍必填（平台实现可忽略作下一跳）。
// 返回：平台 route/ip 命令失败时 error。
// 副作用：修改系统路由表。
// 平台：Windows 为 on-link（0.0.0.0 IF index）；Linux/macOS 为 via gateway。
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

// DelClientRoute 删除客户端上由 AddClientRoute 添加的分流路由。
//
// 参数：cidr、tunName、gateway — 须与添加时一致（平台实现可能部分忽略后两者）。
// 返回：平台删除失败时 error；路由已不存在时多数平台视为成功。
// 副作用：修改系统路由表。
func DelClientRoute(cidr, tunName, gateway string) error {
	return delClientRoutePlatform(cidr, tunName, gateway)
}

// WindowsOnLinkRouteArgs 构造 Windows route ADD 的 on-link 参数切片。
//
// 参数：
//   dest — 目标网络地址（如 192.168.1.0）。
//   mask — 子网掩码（如 255.255.255.0）。
//   ifIndex — Wintun 接口索引。
// 返回：如 []string{"ADD", dest, "MASK", mask, "0.0.0.0", "IF", "n"}。
// 说明：置于无 build tag 文件便于单测；仅 Windows addClientRoutePlatform 调用。
func WindowsOnLinkRouteArgs(dest, mask string, ifIndex int) []string {
	return []string{"ADD", dest, "MASK", mask, "0.0.0.0", "IF", fmt.Sprintf("%d", ifIndex)}
}
