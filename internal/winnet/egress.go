package winnet

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"haovpn/internal/netutil"
)

// EgressAddr 本机一块非虚拟网卡上的 IPv4 地址条目（出站发现用）。
type EgressAddr struct {
	IfIndex   int
	Name      string // FriendlyName（ICS/COM 匹配用）
	IP        net.IP
	PrefixLen int
}

// EgressRoute 一条 IPv4 转发路由（出站发现兜底）。
type EgressRoute struct {
	IfIndex   int
	Dest      net.IP
	PrefixLen int
	Metric    uint32
	IsDefault bool
}

// EgressSnapshot 一次采集的地址表+路由表，供多 LAN 内存匹配（避免 N 次 PS 冷启）。
type EgressSnapshot struct {
	Addrs   []EgressAddr
	Routes  []EgressRoute
	ByIndex map[int]string // ifIndex → FriendlyName
}

var (
	egressSnapMu    sync.Mutex
	egressSnapCache *EgressSnapshot
	egressSnapAt    time.Time
)

// ResolveOutboundNatural 在快照中解析某 LAN 的自然出站网卡。
//
// 顺序：本机同网段 IP → 最长前缀专用路由 → 默认网关。
// viaDefault=true 表示落到 0.0.0.0/0。
func (s *EgressSnapshot) ResolveOutboundNatural(lanCIDR string) (name string, viaDefault bool, err error) {
	if s == nil {
		return "", false, fmt.Errorf("egress snapshot nil")
	}
	_, ipnet, err := netutil.ParseCIDR(lanCIDR)
	if err != nil {
		return "", false, err
	}
	for _, a := range s.Addrs {
		if a.IP != nil && ipnet.Contains(a.IP) {
			return a.Name, false, nil
		}
	}
	probeStr := netutil.ProbeIPForCIDR(lanCIDR)
	probe := net.ParseIP(probeStr).To4()
	if probe == nil {
		return "", false, fmt.Errorf("probe IP 无效")
	}
	bestPlen := -1
	var best *EgressRoute
	var def *EgressRoute
	for i := range s.Routes {
		r := &s.Routes[i]
		if r.IsDefault {
			if def == nil || r.Metric < def.Metric {
				cp := r
				def = cp
			}
			continue
		}
		if r.Dest == nil {
			continue
		}
		mask := net.CIDRMask(r.PrefixLen, 32)
		network := r.Dest.Mask(mask)
		if !probe.Mask(mask).Equal(network) {
			continue
		}
		if r.PrefixLen > bestPlen || (r.PrefixLen == bestPlen && best != nil && r.Metric < best.Metric) {
			bestPlen = r.PrefixLen
			cp := r
			best = cp
		}
	}
	if best != nil {
		return s.ByIndex[best.IfIndex], false, nil
	}
	if def != nil {
		return s.ByIndex[def.IfIndex], true, nil
	}
	return "", false, fmt.Errorf("未找到至 %s 的出站网卡", lanCIDR)
}

// InterfaceExistsInSnapshot 检查友好名是否在快照中（Up 且非跳过）。
func (s *EgressSnapshot) InterfaceExistsInSnapshot(name string) bool {
	if s == nil || strings.TrimSpace(name) == "" {
		return false
	}
	for _, n := range s.ByIndex {
		if strings.EqualFold(n, name) {
			return true
		}
	}
	return false
}

// probeIPForCIDRLocal 已迁至 netutil.ProbeIPForCIDR。
