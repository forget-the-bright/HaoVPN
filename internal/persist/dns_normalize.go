package persist

import (
	"fmt"
	"strings"

	"haovpn/internal/netutil"
)

// dns_normalize.go：托管 DNS IP / 成员 / 排除列表规范化与校验。

// ValidateDNSIP 校验并规范化 IPv4 DNS 地址（禁止空、禁止非 IPv4）。
func ValidateDNSIP(ip string) (string, error) {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return "", fmt.Errorf("dns_ip 不能为空")
	}
	norm, err := netutil.NormalizeIPv4(ip)
	if err != nil {
		return "", fmt.Errorf("无效 dns_ip（须为 IPv4）: %s", ip)
	}
	return norm, nil
}

// NormalizeExcludeUserIDs 规范化排除列表：去重、仅保留 >0（禁止 0）。
func NormalizeExcludeUserIDs(ids []int64) []int64 {
	seen := map[int64]struct{}{}
	var out []int64
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// validateDNSMemberIDs 校验包含集：0=全部；>0 须存在且为 VPN 账号。
func (s *Store) validateDNSMemberIDs(members []int64) error {
	for _, mid := range members {
		if mid == DNSMemberAll {
			continue
		}
		if mid <= 0 {
			return fmt.Errorf("包含集 user_id 无效")
		}
		u, err := s.GetUserByID(mid)
		if err != nil {
			return fmt.Errorf("查询包含账号失败: %w", err)
		}
		if u == nil {
			return fmt.Errorf("包含账号不存在")
		}
		if !u.HasVPN() {
			return fmt.Errorf("包含账号须为 VPN 账号")
		}
	}
	return nil
}

// validateDNSExcludeIDs 校验排除集：须 >0、存在且为 VPN 账号。
func (s *Store) validateDNSExcludeIDs(excludes []int64) error {
	for _, eid := range excludes {
		if eid <= 0 {
			return fmt.Errorf("排除账号 user_id 无效")
		}
		u, err := s.GetUserByID(eid)
		if err != nil {
			return fmt.Errorf("查询排除账号失败: %w", err)
		}
		if u == nil {
			return fmt.Errorf("排除账号不存在")
		}
		if !u.HasVPN() {
			return fmt.Errorf("排除账号须为 VPN 账号")
		}
	}
	return nil
}

// DNSAppliesToUser 判断一条 DNS 定义是否对指定账号生效（members − excludes）。
//
// 指定成员模式下 ignores excludes（计划：仅 all 时排除有意义）；调用方传入已规范化列表。
func DNSAppliesToUser(members, excludes []int64, userID int64) bool {
	if userID <= 0 {
		return false
	}
	hit := false
	if PeerRouteHasAllMembers(members) {
		hit = true
	} else {
		for _, mid := range members {
			if mid == userID {
				hit = true
				break
			}
		}
	}
	if !hit {
		return false
	}
	// 仅「全部」包含集时应用排除；指定列表时 excludes 忽略
	if !PeerRouteHasAllMembers(members) {
		return true
	}
	for _, eid := range excludes {
		if eid == userID {
			return false
		}
	}
	return true
}
