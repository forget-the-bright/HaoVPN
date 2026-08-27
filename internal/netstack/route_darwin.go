//go:build darwin

package netstack

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
)

func enableIPForwardPlatform() error {
	cmd := exec.Command("sysctl", "-w", "net.inet.ip.forwarding=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sysctl forwarding: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// setupNATPlatform macOS：nat.enabled 时 v1.0 不假装 pf NAT 已成功，返回明确错误。
func setupNATPlatform(vpnSubnet, lanCIDR, tunName string, tunIP net.IP, outboundIf string) error {
	_ = outboundIf
	_ = tunName
	_ = tunIP
	_ = vpnSubnet
	_ = lanCIDR
	return fmt.Errorf("darwin: nat.enabled 需要手工配置 pf NAT（v1.0 服务端主推 Linux/Windows）；已开启 IP 转发但 SNAT 未配置")
}

func teardownNATPlatform(vpnSubnet, lanCIDR, tunName string) error {
	_ = vpnSubnet
	_ = lanCIDR
	_ = tunName
	return nil
}

func addClientRoutePlatform(cidr, tunName, gateway string) error {
	cmd := exec.Command("route", "-n", "add", "-net", cidr, gateway)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// 回退：经接口
		cmd2 := exec.Command("route", "-n", "add", "-net", cidr, "-interface", tunName)
		out2, err2 := cmd2.CombinedOutput()
		if err2 != nil {
			return fmt.Errorf("route add %s: %w / %w: %s %s", cidr, err, err2,
				strings.TrimSpace(string(out)), strings.TrimSpace(string(out2)))
		}
	}
	return nil
}

func delClientRoutePlatform(cidr, tunName, gateway string) error {
	_ = tunName
	_ = gateway
	_ = exec.Command("route", "-n", "delete", "-net", cidr).Run()
	return nil
}
