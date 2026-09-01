//go:build darwin

package netstack

import (
	"context"
	"fmt"
	"net"

	"haovpn/internal/platform"
)

func enableIPForwardPlatform() error {
	cmd := platform.Command("sysctl", "-w", "net.inet.ip.forwarding=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return platform.CommandOutputError("sysctl forwarding", out, err)
	}
	return nil
}

// setupNATPlatform macOS：nat.enabled 时 v1.0 不假装 pf NAT 已成功，返回明确错误。
func setupNATPlatform(ctx context.Context, vpnSubnet, lanCIDR, tunName string, tunIP net.IP, outboundIf string) error {
	_ = ctx
	_ = outboundIf
	_ = tunName
	_ = tunIP
	_ = vpnSubnet
	_ = lanCIDR
	return fmt.Errorf("darwin: nat.enabled 需要手工配置 pf NAT（v1.0 服务端主推 Linux/Windows）；已开启 IP 转发但 SNAT 未配置")
}

// setupNATForLANs darwin：与单 LAN 相同，明确告知须手工 pf。
func setupNATForLANs(ctx context.Context, vpnSubnet string, lanCIDRs []string, tunName string, tunIP net.IP, outboundIf string) (NATSetupOutcome, error) {
	lan := ""
	if len(lanCIDRs) > 0 {
		lan = lanCIDRs[0]
	}
	return NATSetupOutcome{}, setupNATPlatform(ctx, vpnSubnet, lan, tunName, tunIP, outboundIf)
}

func teardownNATPlatform(vpnSubnet, lanCIDR, tunName string) error {
	_ = vpnSubnet
	_ = lanCIDR
	_ = tunName
	return nil
}

// disableICSPlatform 非 Windows 无 ICS。
func disableICSPlatform(ctx context.Context) { _ = ctx }

func addClientRoutePlatform(cidr, tunName, gateway string) error {
	cmd := platform.Command("route", "-n", "add", "-net", cidr, gateway)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// 回退：经接口
		cmd2 := platform.Command("route", "-n", "add", "-net", cidr, "-interface", tunName)
		out2, err2 := cmd2.CombinedOutput()
		if err2 != nil {
			return fmt.Errorf("%w / %w",
				platform.CommandOutputError("route add "+cidr, out, err),
				platform.CommandOutputError("route add -interface "+cidr, out2, err2))
		}
	}
	return nil
}

func delClientRoutePlatform(cidr, tunName, gateway string) error {
	_ = tunName
	_ = gateway
	_ = platform.Command("route", "-n", "delete", "-net", cidr).Run()
	return nil
}
