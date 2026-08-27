package netstack

import (
	"fmt"
	"net"
	"strings"
)

// NormalizeKillPrefixes 规范化 AllowedIPs 为防火墙 remoteip 参数。
func NormalizeKillPrefixes(prefixes []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, p := range prefixes {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

// ParseDNSShowOutput 从 netsh 输出解析 DNS。
func ParseDNSShowOutput(out []byte) []string {
	var servers []string
	for _, line := range strings.Split(string(out), "\n") {
		for _, p := range strings.Fields(strings.TrimSpace(line)) {
			if strings.Count(p, ".") == 3 && net.ParseIP(p) != nil {
				servers = append(servers, p)
			}
		}
	}
	return servers
}

// ParseCIDRToV4Mask 将 CIDR 转为主机序 IPv4 地址与掩码（WFP 条件用）。
func ParseCIDRToV4Mask(cidr string) (addr, mask uint32, err error) {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return 0, 0, err
	}
	v4 := ip.To4()
	if v4 == nil {
		return 0, 0, fmt.Errorf("非 IPv4: %s", cidr)
	}
	m := ipnet.Mask
	if len(m) != 4 {
		return 0, 0, fmt.Errorf("无效掩码: %s", cidr)
	}
	addr = uint32(v4[0])<<24 | uint32(v4[1])<<16 | uint32(v4[2])<<8 | uint32(v4[3])
	mask = uint32(m[0])<<24 | uint32(m[1])<<16 | uint32(m[2])<<8 | uint32(m[3])
	return addr, mask, nil
}

// WFPFilterRef 枚举到的过滤器摘要（纯逻辑测试用）。
type WFPFilterRef struct {
	ID       uint64
	Sublayer [16]byte // GUID 原始字节
}

// SelectProductFilterIDs 选出属于本产品子层的过滤器 ID（崩溃残留清理逻辑）。
func SelectProductFilterIDs(items []WFPFilterRef, productSublayer [16]byte) []uint64 {
	var out []uint64
	for _, it := range items {
		if it.Sublayer == productSublayer {
			out = append(out, it.ID)
		}
	}
	return out
}

// HaoVPNKillSublayerBytes 本产品杀开关子层 GUID 的固定字节（与 Windows 实现一致）。
func HaoVPNKillSublayerBytes() [16]byte {
	// {a1b2c3d4-e5f6-7890-abcd-ef0123456789} 小端 Data1/Data2/Data3 + Data4
	return [16]byte{
		0xd4, 0xc3, 0xb2, 0xa1, 0xf6, 0xe5, 0x90, 0x78,
		0xab, 0xcd, 0xef, 0x01, 0x23, 0x45, 0x67, 0x89,
	}
}
