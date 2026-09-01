//go:build linux

package netstack

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"

	"haovpn/internal/logger"
	"haovpn/internal/platform"
)

func enableIPForwardPlatform() error {
	if err := os.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte("1"), 0o644); err != nil {
		return fmt.Errorf("写 ip_forward: %w", err)
	}
	return nil
}

// setupNATPlatform：VPN 子网访问 LAN 时做 MASQUERADE（源必须是 VPN 池，不是 LAN）。
func setupNATPlatform(ctx context.Context, vpnSubnet, lanCIDR, tunName string, tunIP net.IP, outboundIf string) error {
	_ = ctx
	_ = outboundIf
	_ = tunName
	_ = tunIP
	// FORWARD 放行
	fwd := platform.Command("iptables", "-A", "FORWARD", "-s", vpnSubnet, "-d", lanCIDR, "-j", "ACCEPT")
	if out, err := fwd.CombinedOutput(); err != nil {
		logger.Warn("iptables FORWARD: %s %v", strings.TrimSpace(string(out)), err)
	}
	fwdBack := platform.Command("iptables", "-A", "FORWARD", "-s", lanCIDR, "-d", vpnSubnet, "-j", "ACCEPT")
	if out, err := fwdBack.CombinedOutput(); err != nil {
		logger.Warn("iptables FORWARD back: %s %v", strings.TrimSpace(string(out)), err)
	}
	// SNAT/MASQUERADE：来自 VPN 去往 LAN
	cmd := platform.Command("iptables", "-t", "nat", "-A", "POSTROUTING",
		"-s", vpnSubnet, "-d", lanCIDR, "-j", "MASQUERADE")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return platform.CommandOutputError("iptables MASQUERADE", out, err)
	}
	return nil
}

// setupNATForLANs Linux：逐条 LAN 配置 MASQUERADE（无 ICS 多网卡限制）。
func setupNATForLANs(ctx context.Context, vpnSubnet string, lanCIDRs []string, tunName string, tunIP net.IP, outboundIf string) (NATSetupOutcome, error) {
	var errs []string
	for _, lan := range lanCIDRs {
		if err := setupNATPlatform(ctx, vpnSubnet, lan, tunName, tunIP, outboundIf); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", lan, err))
			logger.Error("netstack NAT 失败: %v", err)
			continue
		}
		logger.Info("netstack: NAT 已配置 VPN %s → LAN %s", vpnSubnet, lan)
	}
	if len(errs) > 0 && len(errs) == len(lanCIDRs) {
		return NATSetupOutcome{}, fmt.Errorf("全部 NAT 规则失败: %s", strings.Join(errs, "; "))
	}
	// 部分失败：已有成功规则，与历史 Setup 行为一致（snatEnabled=true）
	return NATSetupOutcome{}, nil
}

func teardownNATPlatform(vpnSubnet, lanCIDR, tunName string) error {
	_ = tunName
	_ = platform.Command("iptables", "-D", "FORWARD", "-s", vpnSubnet, "-d", lanCIDR, "-j", "ACCEPT").Run()
	_ = platform.Command("iptables", "-D", "FORWARD", "-s", lanCIDR, "-d", vpnSubnet, "-j", "ACCEPT").Run()
	_ = platform.Command("iptables", "-t", "nat", "-D", "POSTROUTING",
		"-s", vpnSubnet, "-d", lanCIDR, "-j", "MASQUERADE").Run()
	return nil
}

// disableICSPlatform Linux 无 ICS。
func disableICSPlatform(ctx context.Context) { _ = ctx }

func addClientRoutePlatform(cidr, tunName, gateway string) error {
	var out []byte
	var err error
	if gateway != "" {
		out, err = platform.Command("ip", "route", "replace", cidr, "via", gateway, "dev", tunName).CombinedOutput()
	} else {
		out, err = platform.Command("ip", "route", "replace", cidr, "dev", tunName).CombinedOutput()
	}
	if err != nil {
		return platform.CommandOutputError("ip route replace "+cidr, out, err)
	}
	return nil
}

func delClientRoutePlatform(cidr, tunName, gateway string) error {
	_ = gateway
	_ = platform.Command("ip", "route", "del", cidr, "dev", tunName).Run()
	return nil
}
