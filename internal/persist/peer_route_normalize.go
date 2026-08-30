package persist

import (
	"fmt"
	"strings"

	"haovpn/internal/netutil"
)

// peer_route_normalize.go：托管路由 dest/成员规范化与校验（入库前与 dirty 合并共用）。

// ValidatePeerRouteDest 校验托管路由目标 CIDR：须合法且禁止默认路由。
//
// 领域包装：错误文案带「托管路由」语境；解析与禁默认路由委托 netutil。
func ValidatePeerRouteDest(cidr string) error {
	cidr = strings.TrimSpace(cidr)
	if cidr == "" {
		return fmt.Errorf("dest_cidr 不能为空")
	}
	if _, err := netutil.ParseCIDROrHost(cidr); err != nil {
		return fmt.Errorf("无效 dest_cidr: %s", cidr)
	}
	if err := netutil.ForbidDefaultRoute(cidr); err != nil {
		return fmt.Errorf("禁止默认路由 0.0.0.0/0（托管路由）")
	}
	return nil
}

// NormalizePeerRouteDest 将单 IP 规范为 /32，已是 CIDR 则返回规范化字符串。
//
// 关联：netutil.NormalizeCIDROrHost；入库前须先通过 ValidatePeerRouteDest。
func NormalizePeerRouteDest(cidr string) (string, error) {
	if err := ValidatePeerRouteDest(cidr); err != nil {
		return "", err
	}
	return netutil.NormalizeCIDROrHost(cidr)
}

// NormalizeMemberUserIDs 规范化访问方列表：去重；若含全部(0)则只保留 0。
func NormalizeMemberUserIDs(ids []int64) []int64 {
	seen := map[int64]struct{}{}
	var out []int64
	hasAll := false
	for _, id := range ids {
		if id < 0 {
			continue
		}
		if id == PeerRouteMemberAll {
			hasAll = true
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if hasAll {
		return []int64{PeerRouteMemberAll}
	}
	return out
}

// PeerRouteHasAllMembers 是否包含「全部」访问方。
func PeerRouteHasAllMembers(ids []int64) bool {
	for _, id := range ids {
		if id == PeerRouteMemberAll {
			return true
		}
	}
	return false
}

// UnionMemberUserIDs 合并两份访问方列表供 dirty 标记（旧∪新）。
//
// 任一侧含「全部」则结果为 [0]：须 markPeerDirtyAll，否则被踢出的在线客户端仍持旧路由。
func UnionMemberUserIDs(a, b []int64) []int64 {
	return NormalizeMemberUserIDs(append(append([]int64{}, a...), b...))
}

// validatePeerRouteMemberIDs 校验访问方：>0 须存在且为 VPN 账号；0=全部跳过。
//
// 为何要求 HasVPN：纯管理员无隧道身份，写入成员表无意义且应用生效踢线无对应会话。
func (s *Store) validatePeerRouteMemberIDs(members []int64, viaUserID int64) error {
	for _, mid := range members {
		if mid == PeerRouteMemberAll {
			continue
		}
		if mid <= 0 {
			return fmt.Errorf("访问方 user_id 无效")
		}
		if mid == viaUserID {
			return fmt.Errorf("via 不能是访问方自己")
		}
		u, err := s.GetUserByID(mid)
		if err != nil {
			return fmt.Errorf("查询访问方失败: %w", err)
		}
		if u == nil {
			return fmt.Errorf("访问方账号不存在")
		}
		if !u.HasVPN() {
			return fmt.Errorf("访问方须为 VPN 账号")
		}
	}
	return nil
}
