package netutil

import (
	"net"
	"strings"
)

// IPv4IsICSPrivate 判断是否为 ICS 默认私网地址（192.168.137.0/24）。
func IPv4IsICSPrivate(ip net.IP) bool {
	v4 := ip.To4()
	if v4 == nil {
		return false
	}
	return v4[0] == 192 && v4[1] == 168 && v4[2] == 137
}

// IPv4AddrsToRemove 计算替换式配 IP 时应删除的地址列表。
//
// 参数：have — 网卡当前 IPv4 点分字符串；want — 目标 VPN IP。
// 返回：have 中所有 ≠ want（去空白后）的地址；同 IP 重入时为空切片。
func IPv4AddrsToRemove(have []string, want string) []string {
	want = strings.TrimSpace(want)
	out := make([]string, 0, len(have))
	for _, h := range have {
		h = strings.TrimSpace(h)
		if h == "" || h == want {
			continue
		}
		out = append(out, h)
	}
	return out
}

// IPv4AddrsToRemoveKeepICS 同 IPv4AddrsToRemove，但保留 192.168.137.*（软换 VPN IP、ICS 仍在）。
func IPv4AddrsToRemoveKeepICS(have []string, want string) []string {
	want = strings.TrimSpace(want)
	out := make([]string, 0, len(have))
	for _, h := range have {
		h = strings.TrimSpace(h)
		if h == "" || h == want {
			continue
		}
		if ip := net.ParseIP(h); ip != nil && IPv4IsICSPrivate(ip) {
			continue
		}
		out = append(out, h)
	}
	return out
}

// ICSPrivateIPv4Wildcard 返回 ICS 默认私网 PS -like 模式（192.168.137.*）。
func ICSPrivateIPv4Wildcard() string {
	return "192.168.137.*"
}

// PreferSkipAsSourceNeedsUpdate 判断软换 PreferVPN 是否需改 SkipAsSource（纯函数，winnet/iphlp 共用）。
func PreferSkipAsSourceNeedsUpdate(vpnSkip, has137, skip137 bool) bool {
	if vpnSkip {
		return true
	}
	return has137 && !skip137
}
