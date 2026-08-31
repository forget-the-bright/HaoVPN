package netutil

import (
	"fmt"
	"net"
	"strings"
)

// HostFromAddr 从 "host:port" 或裸 IP/主机名中提取主机部分。
//
// 参数：addr — 如 "203.0.113.1:8443"、"[2001:db8::1]:443" 或 "10.0.0.1"。
// 返回：去掉方括号的主机字符串；SplitHostPort 失败时原样返回 addr。
// 用途：TLS ServerName 推断、隧道来源 IP 日志、白名单前的 host 提取。
func HostFromAddr(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return strings.Trim(host, "[]")
}

// ParseHostIP 从地址字符串解析 IP（支持 host:port 与裸 IP）。
//
// 参数：addr — 远端地址或配置中的 server.address。
// 返回：解析成功的 net.IP；无法解析时 error。
// 用途：隧道来源白名单、审计日志中的客户端 IP。
func ParseHostIP(addr string) (net.IP, error) {
	host := HostFromAddr(addr)
	ip := net.ParseIP(host)
	if ip == nil {
		return nil, fmt.Errorf("无法解析远端地址: %s", addr)
	}
	return ip, nil
}

// NormalizeIPv4 将 IPv4 字符串规范化为点分十进制形式。
//
// 参数：ip — 如 "10.88.0.2" 或带前导零的变体。
// 返回：规范化的 IPv4 字符串；非 IPv4 时 error。
// 用途：VPN IP 入库、IP 池键、API 手动指定 IP 校验后统一格式。
func NormalizeIPv4(ip string) (string, error) {
	ip = strings.TrimSpace(ip)
	parsed := net.ParseIP(ip)
	if parsed == nil || parsed.To4() == nil {
		return "", fmt.Errorf("无效 IPv4: %s", ip)
	}
	return parsed.To4().String(), nil
}

// DedupTrimNonEmpty 对字符串列表去首尾空白、去空项、保序去重。
//
// 参数：items — 如 AllowedIPs 前缀、DNS 列表；原切片不被修改。
// 返回：新切片；空输入返回 nil 或空切片均可接受。
// 用途：杀开关 WFP 前缀、配置项规范化、LAN 注册表。
func DedupTrimNonEmpty(items []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

// MergeDedupTrimNonEmpty 合并两个字符串列表后 Trim、去空、保序去重。
//
// 用途：封禁豁免 yaml + DB 合并；避免 probedefense/api 各写一套 map 去重。
func MergeDedupTrimNonEmpty(a, b []string) []string {
	return DedupTrimNonEmpty(append(append([]string(nil), a...), b...))
}
//
// 用途：sessionmgr / clientapp 过滤 TUN 噪声，避免与组播/链路本地判断散落两套实现。
func IsLimitedBroadcast(ip net.IP) bool {
	if ip == nil {
		return false
	}
	v4 := ip.To4()
	return v4 != nil && v4[0] == 255 && v4[1] == 255 && v4[2] == 255 && v4[3] == 255
}

// NormalizeRemoteHost 从 "host:port" 或裸主机提取并规范化远端主机。
//
// 规则：去掉 []；::1 → 127.0.0.1；IPv4 用 To4().String()；非 IP 主机名转小写。
// 用途：sessionmgr 重连 grace 同主机判定；与 HostFromAddr / SplitRemoteAddr 互补（本函数含 loopback 归一）。
func NormalizeRemoteHost(addr string) string {
	host, _ := SplitRemoteAddr(addr)
	host = strings.Trim(host, "[]")
	if host == "" {
		return ""
	}
	if host == "::1" || host == "0:0:0:0:0:0:0:1" {
		return "127.0.0.1"
	}
	if ip := net.ParseIP(host); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			return v4.String()
		}
		return ip.String()
	}
	return strings.ToLower(host)
}

// IsRFC1918 判断 IPv4 是否落在私有地址（10/8、172.16/12、192.168/16）。
//
// 用途：客户端上报 local_lans / ExitLAN 信任边界，禁止把公网前缀挂成出口网段。
func IsRFC1918(ip net.IP) bool {
	v4 := ip.To4()
	if v4 == nil {
		return false
	}
	if v4[0] == 10 {
		return true
	}
	if v4[0] == 172 && v4[1] >= 16 && v4[1] <= 31 {
		return true
	}
	if v4[0] == 192 && v4[1] == 168 {
		return true
	}
	return false
}

// MinAdvertisedLANPrefix 客户端广告 local_lans 允许的最短前缀长度（含）。
// 短于 /16（如 /8）过宽，易被滥用于绕过横向隔离。
const MinAdvertisedLANPrefix = 16

// NormalizeLANCIDR 规范化客户端上报的本地网段（RFC1918 + ≥/16 + 禁默认路由）。
//
// 等价于 ValidateAdvertisedLAN；命名强调「广告 LAN」场景，供 persist 注册表与 ValidLANCIDRs 调用。
// 关联：clientapp via 出口、tunnel 握手 ExitLANs；托管路由 dest 仍用 ForbidDefaultRoute（管理员可控）。
func NormalizeLANCIDR(cidr string) (string, error) {
	return ValidateAdvertisedLAN(cidr)
}

// ValidLANCIDRs 过滤并规范化 CIDR 列表（供客户端上报 / 握手 ExitLANs / via 指纹）。
//
// 无效项静默跳过；保序去重。策略见 ValidateAdvertisedLAN。
// 为何放在 netutil：纯校验，避免 clientapp/tunnel 仅为 LAN 校验依赖 persist（分层倒置）。
func ValidLANCIDRs(cidrs []string) []string {
	var out []string
	for _, c := range cidrs {
		n, err := NormalizeLANCIDR(c)
		if err != nil {
			continue
		}
		out = append(out, n)
	}
	return DedupTrimNonEmpty(out)
}

// ValidateAdvertisedLAN 校验并规范化客户端上报的本地网段。
//
// 规则：可解析为 CIDR/单 IP；禁止默认路由；须为 RFC1918；IPv4 前缀长度 ≥ MinAdvertisedLANPrefix。
// 返回：规范化 CIDR 字符串；不满足时 error（含中文原因）。
// 关联：ValidLANCIDRs、握手 ExitLANs；与 VPN 池重叠须再调 ValidateAdvertisedLANNotForbidden。
func ValidateAdvertisedLAN(cidr string) (string, error) {
	n, err := ParseCIDROrHost(cidr)
	if err != nil {
		return "", err
	}
	if err := ForbidDefaultRoute(n.String()); err != nil {
		return "", err
	}
	ip := n.IP.To4()
	if ip == nil {
		return "", fmt.Errorf("local_lans 仅支持 IPv4: %s", n.String())
	}
	ones, bits := n.Mask.Size()
	if bits != 32 {
		return "", fmt.Errorf("local_lans 仅支持 IPv4: %s", n.String())
	}
	if ones < MinAdvertisedLANPrefix {
		return "", fmt.Errorf("local_lans 前缀过宽（须 ≥/%d）: %s", MinAdvertisedLANPrefix, n.String())
	}
	// 网段网络地址须落在 RFC1918（用网络地址判断，避免 /32 主机落在私网边界外）
	network := ip.Mask(n.Mask)
	if !IsRFC1918(network) {
		return "", fmt.Errorf("local_lans 须为 RFC1918 私网: %s", n.String())
	}
	return n.String(), nil
}

// CIDRsOverlap 判断两个 CIDR/单 IP 是否有地址交集（含互相包含）。
//
// 用途：拒绝 local_lans 与 vpn.subnet 重叠，防止 ExitLAN 旁路横向隔离后伪造他户 VPN 源。
// 算法：任一方网络地址落在另一方网段内即视为重叠（相邻等长不重叠）。
func CIDRsOverlap(a, b string) (bool, error) {
	na, err := ParseCIDROrHost(a)
	if err != nil {
		return false, err
	}
	nb, err := ParseCIDROrHost(b)
	if err != nil {
		return false, err
	}
	return na.Contains(nb.IP) || nb.Contains(na.IP), nil
}

// ValidateAdvertisedLANNotForbidden 在 ValidateAdvertisedLAN 基础上拒绝与 forbidden 列表重叠的网段。
//
// 参数：cidr — 客户端广告；forbidden — 通常为 vpn.subnet（及将来其它不可广告前缀）。
// 返回：规范化 CIDR；与任一 forbidden 重叠时中文 error。
// 关联：tunnel.applyLANRegistry；空 forbidden 项跳过。
func ValidateAdvertisedLANNotForbidden(cidr string, forbidden ...string) (string, error) {
	n, err := ValidateAdvertisedLAN(cidr)
	if err != nil {
		return "", err
	}
	for _, f := range forbidden {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		ov, err := CIDRsOverlap(n, f)
		if err != nil {
			// forbidden 自身不可解析时跳过该项（启动配置错误应由别处拦截）
			continue
		}
		if ov {
			return "", fmt.Errorf("local_lans 不得与 VPN 地址池重叠: %s ∩ %s", n, f)
		}
	}
	return n, nil
}

// ValidLANCIDRsNotForbidden 过滤并规范化 CIDR 列表，并剔除与 forbidden 重叠项。
//
// 无效或重叠项静默跳过（调用方逐条校验时可打日志）；保序去重。
func ValidLANCIDRsNotForbidden(cidrs []string, forbidden ...string) []string {
	var out []string
	for _, c := range cidrs {
		n, err := ValidateAdvertisedLANNotForbidden(c, forbidden...)
		if err != nil {
			continue
		}
		out = append(out, n)
	}
	return DedupTrimNonEmpty(out)
}
