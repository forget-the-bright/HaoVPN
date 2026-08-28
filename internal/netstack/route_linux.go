//go:build linux

package netstack

import (
	"fmt"
	"net"
	"os"
	"os/exec"
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
func setupNATPlatform(vpnSubnet, lanCIDR, tunName string, tunIP net.IP, outboundIf string) error {
	_ = outboundIf
	_ = tunName
	_ = tunIP
	// FORWARD 放行
	fwd := exec.Command("iptables", "-A", "FORWARD", "-s", vpnSubnet, "-d", lanCIDR, "-j", "ACCEPT")
	if out, err := fwd.CombinedOutput(); err != nil {
		logger.Warn("iptables FORWARD: %s %v", strings.TrimSpace(string(out)), err)
	}
	fwdBack := exec.Command("iptables", "-A", "FORWARD", "-s", lanCIDR, "-d", vpnSubnet, "-j", "ACCEPT")
	if out, err := fwdBack.CombinedOutput(); err != nil {
		logger.Warn("iptables FORWARD back: %s %v", strings.TrimSpace(string(out)), err)
	}
	// SNAT/MASQUERADE：来自 VPN 去往 LAN
	cmd := exec.Command("iptables", "-t", "nat", "-A", "POSTROUTING",
		"-s", vpnSubnet, "-d", lanCIDR, "-j", "MASQUERADE")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return platform.CommandOutputError("iptables MASQUERADE", out, err)
	}
	return nil
}

func teardownNATPlatform(vpnSubnet, lanCIDR, tunName string) error {
	_ = tunName
	_ = exec.Command("iptables", "-D", "FORWARD", "-s", vpnSubnet, "-d", lanCIDR, "-j", "ACCEPT").Run()
	_ = exec.Command("iptables", "-D", "FORWARD", "-s", lanCIDR, "-d", vpnSubnet, "-j", "ACCEPT").Run()
	_ = exec.Command("iptables", "-t", "nat", "-D", "POSTROUTING",
		"-s", vpnSubnet, "-d", lanCIDR, "-j", "MASQUERADE").Run()
	return nil
}

func addClientRoutePlatform(cidr, tunName, gateway string) error {
	var cmd *exec.Cmd
	if gateway != "" {
		cmd = exec.Command("ip", "route", "replace", cidr, "via", gateway, "dev", tunName)
	} else {
		cmd = exec.Command("ip", "route", "replace", cidr, "dev", tunName)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return platform.CommandOutputError("ip route replace "+cidr, out, err)
	}
	return nil
}

func delClientRoutePlatform(cidr, tunName, gateway string) error {
	_ = gateway
	cmd := exec.Command("ip", "route", "del", cidr, "dev", tunName)
	_ = cmd.Run()
	return nil
}
