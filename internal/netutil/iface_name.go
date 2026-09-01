package netutil

import "strings"

// IsVirtualInterfaceName 判断是否应跳过的虚拟/隧道网卡名（出站解析、ICS egress PS 与 iphlp 共用）。
func IsVirtualInterfaceName(name string) bool {
	n := strings.ToLower(name)
	for _, p := range []string{"haovpn", "loopback", "tap-win", "wintun", "openvpn", "tailscale"} {
		if strings.Contains(n, p) {
			return true
		}
	}
	if strings.Contains(n, "tun ") || strings.HasPrefix(n, "tun") || strings.Contains(n, " tunnel") {
		return true
	}
	return false
}

// InterfaceNameLooksLikeTUN 判断 net.Interface.Name 是否为本产品 TUN（yaml configName 或虚拟网卡名）。
func InterfaceNameLooksLikeTUN(name, configName string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	if configName != "" && (name == configName || strings.HasPrefix(name, configName+" ")) {
		return true
	}
	return IsVirtualInterfaceName(name)
}

// VirtualInterfaceSkipPattern 返回 PowerShell -notmatch 用正则片段（与 IsVirtualInterfaceName 语义对齐）。
func VirtualInterfaceSkipPattern() string {
	return "HaoVPN|Loopback|TAP-WIN|TUN|OpenVPN|Tailscale"
}
