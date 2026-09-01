//go:build windows

package netstack

// ics_egress_windows.go：ICS 公网侧出站网卡解析（IP Helper 快照优先，失败回退 PS）。
// 规划纯函数见 ics_plan.go；Enable/PreferVPN 见 ics_enable_windows.go。

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"haovpn/internal/logger"
	"haovpn/internal/netutil"
	"haovpn/internal/safeutil"
	"haovpn/internal/winnet"
)

// findOutboundInterface 确定 ICS 公网侧网卡：配置 > 本机同网段 IP > 专用路由 > 默认网关。
// 优先用 CollectEgressSnapshot（一次 IP Helper）；失败才回退旧 PS 路径。
func findOutboundInterface(ctx context.Context, lanCIDR, configured string) (string, error) {
	if err := safeutil.Check(ctx); err != nil {
		return "", err
	}
	configured = strings.TrimSpace(configured)
	if configured != "" {
		if err := verifyInterfaceExists(ctx, configured); err != nil {
			return "", fmt.Errorf("outbound_interface=%q: %w", configured, err)
		}
		logger.Info("windows: 使用配置的 outbound_interface=%s", configured)
		return configured, nil
	}
	start := time.Now()
	if snap, err := winnet.CollectEgressSnapshot(); err == nil {
		name, viaDefault, err := snap.ResolveOutboundNatural(lanCIDR)
		if err == nil && name != "" {
			if viaDefault {
				logger.Info("lan_egress default_route if=%s cidr=%s method=iphlp elapsed=%s", name, lanCIDR, time.Since(start))
			} else {
				logger.Info("windows: 出站网卡（本机 LAN IP 同网段或专用路由）=%s lan=%s method=iphlp elapsed=%s", name, lanCIDR, time.Since(start))
			}
			logger.Info("ics_egress lan=%s if=%s via_default=%v elapsed=%s method=iphlp", lanCIDR, name, viaDefault, time.Since(start))
			return name, nil
		}
		logger.Warn("ics_egress method=iphlp miss lan=%s err=%v，回退 PS", lanCIDR, err)
	} else {
		logger.Warn("ics_egress snapshot fail: %v，回退 PS", err)
	}
	return findOutboundInterfacePS(ctx, lanCIDR)
}

// findOutboundInterfacePS 旧路径：每 LAN 1～2 次 PowerShell（仅 iphlp 失败时使用）。
func findOutboundInterfacePS(ctx context.Context, lanCIDR string) (string, error) {
	start := time.Now()
	if name, err := findInterfaceWithIPInCIDR(ctx, lanCIDR); err == nil && name != "" {
		logger.Info("windows: 出站网卡（本机 LAN IP 同网段）=%s lan=%s method=ps elapsed=%s", name, lanCIDR, time.Since(start))
		logger.Info("ics_egress lan=%s if=%s elapsed=%s method=ps", lanCIDR, name, time.Since(start))
		return name, nil
	}
	if err := safeutil.Check(ctx); err != nil {
		return "", err
	}
	name, viaDefault, err := findInterfaceByRoute(ctx, lanCIDR)
	if err != nil || name == "" {
		return "", fmt.Errorf("未找到至 %s 的出站网卡（可配置 windows.outbound_interface）", lanCIDR)
	}
	if viaDefault {
		logger.Info("lan_egress default_route if=%s cidr=%s method=ps elapsed=%s", name, lanCIDR, time.Since(start))
	} else {
		logger.Info("windows: 出站网卡（专用路由至 %s）=%s method=ps elapsed=%s", lanCIDR, name, time.Since(start))
	}
	logger.Info("ics_egress lan=%s if=%s via_default=%v elapsed=%s method=ps", lanCIDR, name, viaDefault, time.Since(start))
	return name, nil
}

func verifyInterfaceExists(ctx context.Context, name string) error {
	if snap, err := winnet.CollectEgressSnapshot(); err == nil && snap.InterfaceExistsInSnapshot(name) {
		return nil
	}
	ps := winnet.PSSnippetVerifyInterfaceExists(name)
	_, err := winnet.RunPSOneShotContext(ctx, ps)
	return err
}

// findInterfaceWithIPInCIDR 本机有该网段 IP 的网卡（服务端与 PLC 同二层/同网段）。
func findInterfaceWithIPInCIDR(ctx context.Context, lanCIDR string) (string, error) {
	_, ipnet, err := netutil.ParseCIDR(lanCIDR)
	if err != nil {
		return "", err
	}
	network := ipnet.IP.Mask(ipnet.Mask).String()
	mask := net.IP(ipnet.Mask).String()

	ps := winnet.PSSnippetFindInterfaceInCIDR(network, mask, netutil.VirtualInterfaceSkipPattern())

	out, err := winnet.RunPSOneShotContext(ctx, ps)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// findInterfaceByRoute 查路由表：优先专用路由至 lanCIDR；否则回退默认网关 0.0.0.0/0。
func findInterfaceByRoute(ctx context.Context, lanCIDR string) (name string, viaDefault bool, err error) {
	probe := netutil.ProbeIPForCIDR(lanCIDR)
	ps := winnet.PSSnippetFindInterfaceByRoute(probe, netutil.VirtualInterfaceSkipPattern())

	out, err := winnet.RunPSOneShotContext(ctx, ps)
	if err != nil {
		return "", false, err
	}
	line := strings.TrimSpace(string(out))
	parts := strings.SplitN(line, "|", 2)
	if len(parts) != 2 {
		if line == "" {
			return "", false, fmt.Errorf("路由表无可用出站网卡")
		}
		return line, false, nil
	}
	name = strings.TrimSpace(parts[1])
	if name == "" {
		return "", false, fmt.Errorf("路由表无可用出站网卡")
	}
	return name, parts[0] == "1", nil
}

// resolveICSBindings 为各 LAN 解析「自然」出站网卡；一次 snapshot 覆盖全部 LAN。
func resolveICSBindings(ctx context.Context, lanCIDRs []string, preferred string) ([]ICSLANBinding, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	totalStart := time.Now()
	preferred = strings.TrimSpace(preferred)
	if preferred != "" {
		if err := verifyInterfaceExists(ctx, preferred); err != nil {
			return nil, fmt.Errorf("outbound_interface=%q: %w", preferred, err)
		}
	}
	// 预热快照：后续 findOutboundInterface 命中缓存
	if _, err := winnet.CollectEgressSnapshot(); err != nil {
		logger.Debug("ics_egress prewarm: %v", err)
	}
	bindings := make([]ICSLANBinding, 0, len(lanCIDRs))
	for _, lan := range lanCIDRs {
		lan = strings.TrimSpace(lan)
		if lan == "" {
			continue
		}
		if err := safeutil.Check(ctx); err != nil {
			return nil, err
		}
		natural, err := findOutboundInterface(ctx, lan, "")
		if preferred == "" {
			if err != nil {
				logger.Warn("ics_plan resolve fail lan=%s err=%v", lan, err)
				bindings = append(bindings, ICSLANBinding{CIDR: lan, IfName: ""})
				continue
			}
			bindings = append(bindings, ICSLANBinding{CIDR: lan, IfName: natural})
			continue
		}
		if err == nil && !strings.EqualFold(natural, preferred) {
			bindings = append(bindings, ICSLANBinding{CIDR: lan, IfName: natural})
			continue
		}
		if err != nil {
			logger.Debug("ics_plan preferred fallback lan=%s preferred=%s natural_err=%v", lan, preferred, err)
		}
		bindings = append(bindings, ICSLANBinding{CIDR: lan, IfName: preferred})
	}
	logger.Info("ics_egress resolve_all elapsed=%s lans=%d preferred=%q", time.Since(totalStart), len(bindings), preferred)
	return bindings, nil
}

