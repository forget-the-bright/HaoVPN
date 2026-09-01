package netstack

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"haovpn/internal/logger"
	"haovpn/internal/safeutil"
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
//
// 生命周期：取消上下文不放本结构（易陈旧）；传给 Setup(ctx)/Teardown(ctx)。
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
	snatEnabled bool            // 是否已成功配置 SNAT/MASQUERADE
	icsUsed     bool            // 本次 Setup 是否走了 Windows ICS
	icsPlan     ICSOutboundPlan // ICS 决策（仅 icsUsed 时有意义）
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

// UsedICS 报告最近一次成功 Setup 是否启用了 Windows ICS（非 WinNAT）。
func (s *Stack) UsedICS() bool {
	return s.icsUsed
}

// ICSLocalLANsHint 用户可见 ICS 多网卡提示（有跳过或同网卡多段 Info）；未走 ICS 或无可提示时为空。
func (s *Stack) ICSLocalLANsHint() string {
	if !s.icsUsed {
		return ""
	}
	return FormatICSLocalLANsHint(s.icsPlan)
}

// ICSActiveCIDRs ICS 实际生效的网段；未走 ICS 时返回空切片（调用方应用全部 LanCIDRs）。
func (s *Stack) ICSActiveCIDRs() []string {
	if !s.icsUsed {
		return nil
	}
	out := make([]string, 0, len(s.icsPlan.Active))
	for _, b := range s.icsPlan.Active {
		if c := strings.TrimSpace(b.CIDR); c != "" {
			out = append(out, c)
		}
	}
	return out
}

// ICSSkippedCIDRs ICS 因异网卡等原因未生效的网段。
func (s *Stack) ICSSkippedCIDRs() []string {
	if !s.icsUsed {
		return nil
	}
	out := make([]string, 0, len(s.icsPlan.Skipped))
	for _, b := range s.icsPlan.Skipped {
		if c := strings.TrimSpace(b.CIDR); c != "" {
			out = append(out, c)
		}
	}
	return out
}

// Setup 启用系统 IP 转发并为每个 LanCIDR 配置 VPN→LAN 的 NAT。
//
// 参数：ctx — Stop/HardRestart 取消时 Kill ICS/探测 PowerShell；nil 等同 Background。
// 返回：Enabled=false 时 nil；全部 SNAT 失败且非 forward_only 时 error；
//
//	ctx 取消时返回 context.Canceled（不得当作 forward_only「无 SNAT 成功」）。
//
// 副作用：写 iptables/WinNAT/ICS、注册表 IPEnableRouter 等（依平台）。
// 并发：非并发安全。
func (s *Stack) Setup(ctx context.Context) error {
	if !s.cfg.Enabled {
		logger.Info("netstack: NAT/转发已关闭（nat.enabled=false）")
		return nil
	}
	if s.cfg.VPNSubnet == "" {
		return fmt.Errorf("netstack: VPNSubnet 为空，无法配置 SNAT")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := safeutil.Check(ctx); err != nil {
		logger.Info("netstack setup aborted err=%v", err)
		return err
	}
	if err := enableIPForwardPlatform(); err != nil {
		return fmt.Errorf("启用 IP 转发失败: %w", err)
	}
	logger.Info("netstack: IP 转发已开启")

	if len(s.cfg.LanCIDRs) == 0 {
		logger.Warn("netstack: allowed_lan_cidrs 为空，仅开启转发、未加 SNAT 规则")
		s.snatEnabled = false
		s.icsUsed = false
		s.icsPlan = ICSOutboundPlan{}
		logger.Info("netstack setup 完成 lan_count=0 snat=false")
		return nil
	}

	out, err := setupNATForLANs(ctx, s.cfg.VPNSubnet, s.cfg.LanCIDRs, s.cfg.TunName, s.cfg.TunIP, s.cfg.OutboundIf)
	if err != nil {
		if safeutil.IsCanceled(err) || safeutil.Check(ctx) != nil {
			logger.Info("netstack setup aborted err=%v", err)
			if e := safeutil.Check(ctx); e != nil {
				return e
			}
			return err
		}
		if s.cfg.ForwardOnly {
			logger.Warn("netstack: SNAT 失败，forward_only=true，仅 IP 转发继续（health nat_ok=false）: %v", err)
			logger.Warn("netstack: 可测隧道/ ping 网关；访问其它 LAN 需 WinNAT(Hyper-V)/Linux，或手工启用 ICS 后重启")
			s.snatEnabled = false
			s.icsUsed = false
			s.icsPlan = ICSOutboundPlan{}
			logger.Info("netstack setup 完成（无 SNAT） lan_count=%d", len(s.cfg.LanCIDRs))
			return nil
		}
		return err
	}
	s.snatEnabled = true
	s.icsUsed = out.UsedICS
	s.icsPlan = out.Plan
	if out.UsedICS {
		logger.Info("netstack setup 完成 lan_count=%d snat=%v ics=true active=%d skipped=%d",
			len(s.cfg.LanCIDRs), s.snatEnabled, len(out.Plan.Active), len(out.Plan.Skipped))
	} else {
		logger.Info("netstack setup 完成 lan_count=%d snat=%v", len(s.cfg.LanCIDRs), s.snatEnabled)
	}
	return nil
}

// ProbeIPForCIDR 已迁至 netutil.ProbeIPForCIDR；本包调用方请直接用 netutil。

// Teardown 按 Config 拆除已配置的 NAT 规则（尽力而为，失败仅打日志）。
//
// 参数：ctx — 取消时 Kill 进行中的 Disable* PowerShell；正常 Stop 清数据面应传
//
//	context.Background()，避免因 runCtx 已取消而跳过 ICS 清理留下残留。
//	快速退出/抢占路径可传可取消 ctx。
//
// 返回：Enabled=false 时 nil；个别规则删除失败不阻断整体返回。
// 副作用：删除 iptables/NetNat；Windows 上 ICS 仅关闭一次（避免多 LAN 重复 COM 卡顿）。
func (s *Stack) Teardown(ctx context.Context) error {
	if !s.cfg.Enabled {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	start := time.Now()
	for _, lan := range s.cfg.LanCIDRs {
		if err := teardownNATPlatform(s.cfg.VPNSubnet, lan, s.cfg.TunName); err != nil {
			logger.Warn("netstack teardown NAT %s: %v", lan, err)
		}
	}
	// ICS 关闭与 LAN 条数无关，只做一次（每条 LAN 调一次会卡数秒×N）
	icsStart := time.Now()
	disableICSPlatform(ctx)
	logger.Info("netstack teardown 完成 elapsed=%s ics_elapsed=%s lans=%d",
		time.Since(start), time.Since(icsStart), len(s.cfg.LanCIDRs))
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
