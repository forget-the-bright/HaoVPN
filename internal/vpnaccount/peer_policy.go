package vpnaccount

import (
	"net"
	"sort"
	"strings"

	"haovpn/internal/logger"
	"haovpn/internal/persist"
)

// ManagedRouteInfo 握手下发的托管路由展示项（对齐 ZeroTier：dest via vpn_ip）。
//
// Stale 为 true 表示 via 离线或注册表无匹配 dest，不下发到 AllowedIPs。
type ManagedRouteInfo struct {
	DestCIDR    string `json:"dest"`
	ViaIP       string `json:"via_ip"`
	ViaUserID   int64  `json:"via_user_id"`
	ViaUsername string `json:"via_username,omitempty"`
	Stale       bool   `json:"stale,omitempty"`
}

// ClientPolicy 握手用完整客户端策略：AllowedIPs（含 peer/托管合并）+ ManagedRoutes 元数据。
type ClientPolicy struct {
	AllowedIPs    []string
	ManagedRoutes []ManagedRouteInfo
	PeerAccessIDs []int64 // 可互访的 peer user_id（横向策略，服务端会话用）
	ViaUserIDs    []int64 // 本账号托管路由的 via 账号（服务端亦可直连其 VPN IP）
}

// ResolveClientPolicy 合并默认/账号 AllowedIPs、互访与托管路由 dest，并生成 managed_routes。
//
// AllowedIPs 语义：经**服务端网关/NAT**或经 via 会话可达的目的前缀（客户端装路由）。
// ManagedRoutes：供 GUI/托盘展示；失效路由 Stale=true 且不进入 AllowedIPs / 会话 ViaRoutes。
// 失效条件：via 无 VPN IP，或 client_lan_registry 无匹配 (via, dest)。
// PeerAccessIDs / ViaUserIDs：写入服务端会话做横向放行；peer/via 的 /32 仅在**未被**已有 CIDR 覆盖时才下发。
func (s *Service) ResolveClientPolicy(u *persist.User) (*ClientPolicy, error) {
	if u == nil {
		return &ClientPolicy{}, nil
	}
	base := s.ResolveAllowedIPs(u)
	seen := map[string]struct{}{}
	var allowed []string
	var nets []*net.IPNet

	// addCIDR 追加一条目的前缀；skipIfCovered 为 true 时若已被已有网段包含则跳过（用于 peer/via /32）。
	addCIDR := func(c string, skipIfCovered bool) {
		c = strings.TrimSpace(c)
		if c == "" {
			return
		}
		var n *net.IPNet
		if _, parsed, err := net.ParseCIDR(c); err == nil {
			n = parsed
			c = parsed.String()
		} else if ip := net.ParseIP(c); ip != nil && ip.To4() != nil {
			_, n, _ = net.ParseCIDR(ip.String() + "/32")
			c = n.String()
		} else {
			return
		}
		if _, ok := seen[c]; ok {
			return
		}
		if skipIfCovered {
			ip := n.IP
			for _, exist := range nets {
				if exist.Contains(ip) {
					return
				}
			}
		}
		seen[c] = struct{}{}
		allowed = append(allowed, c)
		nets = append(nets, n)
	}
	for _, c := range base {
		addCIDR(c, false)
	}

	out := &ClientPolicy{}

	if s.Store != nil {
		peerIDs, err := s.Store.ListPeerAccessPeerIDs(u.ID)
		if err != nil {
			return nil, err
		}
		out.PeerAccessIDs = peerIDs
		for _, pid := range peerIDs {
			pu, err := s.Store.GetUserByID(pid)
			if err != nil || pu == nil || strings.TrimSpace(pu.VPNIP) == "" {
				continue
			}
			addCIDR(pu.VPNIP+"/32", true)
		}

		routes, err := s.Store.ListPeerRoutesForAccessor(u.ID)
		if err != nil {
			return nil, err
		}
		viaSeen := map[int64]struct{}{}
		for _, r := range routes {
			info := ManagedRouteInfo{
				DestCIDR:  r.DestCIDR,
				ViaUserID: r.ViaUserID,
			}
			viaOnline := false
			if vu, err := s.Store.GetUserByID(r.ViaUserID); err == nil && vu != nil {
				info.ViaUsername = vu.Username
				info.ViaIP = strings.TrimSpace(vu.VPNIP)
				viaOnline = info.ViaIP != ""
			}
			regOK, err := s.Store.HasLanRegistryMatch(r.ViaUserID, r.DestCIDR)
			if err != nil {
				return nil, err
			}
			info.Stale = !viaOnline || !regOK
			if info.Stale {
				logger.Info("peer_route_stale skipped accessor=%d dest=%s via=%d online=%v registry=%v",
					u.ID, r.DestCIDR, r.ViaUserID, viaOnline, regOK)
				out.ManagedRoutes = append(out.ManagedRoutes, info)
				continue
			}

			// 有效托管路由：装 dest；会话 ViaRoutes 仅含有效项（见握手组装）
			addCIDR(r.DestCIDR, false)
			out.ManagedRoutes = append(out.ManagedRoutes, info)
			if _, ok := viaSeen[r.ViaUserID]; !ok {
				viaSeen[r.ViaUserID] = struct{}{}
				out.ViaUserIDs = append(out.ViaUserIDs, r.ViaUserID)
				if info.ViaIP != "" {
					addCIDR(info.ViaIP+"/32", true)
				}
			}
		}
	}

	sort.Strings(allowed)
	out.AllowedIPs = allowed
	return out, nil
}
