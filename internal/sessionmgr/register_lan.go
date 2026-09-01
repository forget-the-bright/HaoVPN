package sessionmgr

import (
	"net"
	"sort"
	"time"

	"haovpn/internal/logger"
	"haovpn/internal/netutil"
)

// register_lan.go：ExitLAN 加载/刷新、旧客户端 lan_registry 限速、via 活路由剪枝、viaIndex 重建。
// 与 RegisterVPN 同包；导出 ReloadExitLANs / AllowLANRegistrySync / PruneViaRoutesAfterRegistry。

// loadExitLANs 从临时注册表加载本账号 via 出口网段（握手前已 ReplaceClientLANRegistry）。
func (m *Manager) loadExitLANs(userID int64) []*net.IPNet {
	if m.store == nil || userID <= 0 {
		return nil
	}
	rows, err := m.store.ListClientLANRegistry(userID)
	if err != nil || len(rows) == 0 {
		return nil
	}
	var cidrs []string
	for _, r := range rows {
		cidrs = append(cidrs, r.DestCIDR)
	}
	nets, err := netutil.ParseCIDRListToNets(cidrs)
	if err != nil {
		logger.Warn("解析 exit_lans 失败 user_id=%d: %v", userID, err)
		return nil
	}
	return nets
}

// ReloadExitLANs 按当前 client_lan_registry 刷新在线会话的 ExitLANs（ICS 纠正注册表后调用）。
//
// 无在线会话时为无操作。活路由剪枝请另调 PruneViaRoutesAfterRegistry（本方法只更新 ExitLANs）。
func (m *Manager) ReloadExitLANs(userID int64) {
	if userID <= 0 {
		return
	}
	nets := m.loadExitLANs(userID)
	m.mu.Lock()
	defer m.mu.Unlock()
	ps, ok := m.sessions[userID]
	if !ok || ps == nil {
		return
	}
	ps.ExitLANs = nets
	logger.Info("exit_lans_reloaded user_id=%d count=%d", userID, len(nets))
}

// lanRegistryMinInterval 同一会话两次 lan_registry 的最短间隔（防 SQLite/ExitLAN 抖扇）。
const lanRegistryMinInterval = 30 * time.Second

// AllowLANRegistrySync 原子检查并记账：本会话是否允许再处理一条 lan_registry。
//
// 首次始终允许；之后须间隔 ≥ lanRegistryMinInterval。无在线会话返回 false。
// 通过后递增计数并刷新时间戳；调用方在 false 时打 rate_limited，不断开隧道。
func (m *Manager) AllowLANRegistrySync(userID int64) bool {
	if userID <= 0 {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	ps, ok := m.sessions[userID]
	if !ok || ps == nil {
		return false
	}
	now := time.Now()
	if ps.lanRegistrySyncCount > 0 && now.Sub(ps.lastLANRegistrySync) < lanRegistryMinInterval {
		return false
	}
	ps.lanRegistrySyncCount++
	ps.lastLANRegistrySync = now
	return true
}

// PruneViaRoutesAfterRegistry 在 via 的 client_lan_registry 收缩后剪掉失效托管转发。
//
// 从所有在线会话的 ViaRoutes 中移除「viaUserID=via 且 dest 不在当前注册表」的条目，
// 并 rebuildViaIndex。返回被剪枝的成员 userID（调用方应 Kick 使其重握刷新 AllowedIPs）。
//
// 为何必须：仅 ReloadExitLANs 不改 viaIndex/成员 ViaRoutes，成员仍会把已跳过 CIDR
// 的流量直转 via → 黑洞。HasLanRegistryMatch 只影响新握手 Stale，不修活会话。
func (m *Manager) PruneViaRoutesAfterRegistry(viaUserID int64) []int64 {
	if viaUserID <= 0 {
		return nil
	}
	allowed := map[string]struct{}{}
	if m.store != nil {
		rows, err := m.store.ListClientLANRegistry(viaUserID)
		if err != nil {
			logger.Warn("prune_via_routes list registry fail via=%d: %v", viaUserID, err)
		} else {
			for _, r := range rows {
				allowed[r.DestCIDR] = struct{}{}
			}
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	var affected []int64
	for id, ps := range m.sessions {
		if ps == nil {
			continue
		}
		kept := make([]viaRouteEntry, 0, len(ps.ViaRoutes))
		pruned := false
		for _, e := range ps.ViaRoutes {
			if e.viaUserID != viaUserID {
				kept = append(kept, e)
				continue
			}
			key := viaNetString(e)
			if _, ok := allowed[key]; ok {
				kept = append(kept, e)
				continue
			}
			pruned = true
			logger.Info("via_route_pruned member=%d via=%d dest=%s", id, viaUserID, key)
		}
		if pruned {
			ps.ViaRoutes = kept
			affected = append(affected, id)
		}
	}
	m.rebuildViaIndexLocked()
	return affected
}

// parseViaRoutes 将规格解析为 viaRouteEntry；非法 CIDR 返回错误。
func parseViaRoutes(specs []ViaRouteSpec) ([]viaRouteEntry, error) {
	if len(specs) == 0 {
		return nil, nil
	}
	out := make([]viaRouteEntry, 0, len(specs))
	for _, s := range specs {
		nets, err := netutil.ParseCIDRListToNets([]string{s.DestCIDR})
		if err != nil {
			return nil, err
		}
		if len(nets) == 0 || s.ViaUserID <= 0 {
			continue
		}
		out = append(out, viaRouteEntry{net: nets[0], viaUserID: s.ViaUserID})
	}
	return out, nil
}

// rebuildViaIndexLocked 从所有在线会话重建托管路由出站索引（调用方须持写锁）。
//
// 先按 userID 排序再扁平化，再按 dest CIDR + viaUserID 稳定排序，避免 map 迭代导致
// 重叠 CIDR 时 RouteOutbound 首次命中 via 不确定（间歇性错路）。
func (m *Manager) rebuildViaIndexLocked() {
	ids := make([]int64, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var idx []viaRouteEntry
	for _, id := range ids {
		ps := m.sessions[id]
		if ps == nil {
			continue
		}
		idx = append(idx, ps.ViaRoutes...)
	}
	sort.SliceStable(idx, func(i, j int) bool {
		si, sj := viaNetString(idx[i]), viaNetString(idx[j])
		if si != sj {
			return si < sj
		}
		return idx[i].viaUserID < idx[j].viaUserID
	})
	m.viaIndex = idx
}

// viaNetString 将 via 条目的网段转为可比较字符串（nil 视为空）。
func viaNetString(e viaRouteEntry) string {
	if e.net == nil {
		return ""
	}
	return e.net.String()
}