package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"haovpn/internal/logger"
	"haovpn/internal/persist"
	"haovpn/internal/timeutil"
)

// peerRouteView 托管路由列表行（展示 dest via vpn_ip + 失效/成员）。
type peerRouteView struct {
	ID            int64   `json:"id"`
	DestCIDR      string  `json:"dest_cidr"`
	ViaUserID     int64   `json:"via_user_id"`
	ViaUsername   string  `json:"via_username"`
	ViaVPNIP      string  `json:"via_vpn_ip,omitempty"`
	Display       string  `json:"display"`
	ViaOffline    bool    `json:"via_offline"`
	Stale         bool    `json:"stale"` // via 离线或注册表无匹配 dest
	Scope         string  `json:"scope"` // all | user
	MemberUserIDs []int64 `json:"member_user_ids"`
	MemberNames   string  `json:"member_names,omitempty"`
}

// peerAccessView 互访白名单一行。
type peerAccessView struct {
	UserID       int64  `json:"user_id"`
	Username     string `json:"username"`
	PeerUserID   int64  `json:"peer_user_id"`
	PeerUsername string `json:"peer_username"`
	PeerVPNIP    string `json:"peer_vpn_ip,omitempty"`
}

// lanRegistryView 客户端本地网段注册表一行。
type lanRegistryView struct {
	UserID    int64  `json:"user_id"`
	Username  string `json:"username"`
	DestCIDR  string `json:"dest_cidr"`
	VPNIP     string `json:"vpn_ip"`
	HostID    string `json:"host_id,omitempty"`
	UpdatedAt string `json:"updated_at"`
}

func (s *Server) actorFromRequest(r *http.Request) *int64 {
	se, ok := s.sessionFromRequest(r)
	if !ok {
		return nil
	}
	id := se.UserID
	return &id
}

// markPeerDirtyUsers 标记指定账号须「应用生效」后踢线刷新策略。
func (s *Server) markPeerDirtyUsers(ids ...int64) {
	s.peerDirtyMu.Lock()
	defer s.peerDirtyMu.Unlock()
	if s.peerDirtyIDs == nil {
		s.peerDirtyIDs = map[int64]struct{}{}
	}
	for _, id := range ids {
		if id > 0 {
			s.peerDirtyIDs[id] = struct{}{}
		}
	}
}

// markPeerDirtyAll 标记全部 VPN 账号须应用生效（全员托管路由变更）。
func (s *Server) markPeerDirtyAll() {
	s.peerDirtyMu.Lock()
	defer s.peerDirtyMu.Unlock()
	s.peerDirtyAll = true
}

// peerDirtyStatus 返回待应用状态（控制台黄条）。
func (s *Server) peerDirtyStatus() (pending bool, all bool, ids []int64) {
	s.peerDirtyMu.Lock()
	defer s.peerDirtyMu.Unlock()
	all = s.peerDirtyAll
	for id := range s.peerDirtyIDs {
		ids = append(ids, id)
	}
	pending = all || len(ids) > 0
	return pending, all, ids
}

// clearPeerDirty 应用生效成功后清空脏标记。
func (s *Server) clearPeerDirty() {
	s.peerDirtyMu.Lock()
	defer s.peerDirtyMu.Unlock()
	s.peerDirtyAll = false
	s.peerDirtyIDs = map[int64]struct{}{}
}

// userDirMap 轻量账号目录 id→条目（无私钥）。
func (s *Server) userDirMap() (map[int64]persist.UserDirectoryEntry, error) {
	dir, err := s.store.ListUserDirectory()
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]persist.UserDirectoryEntry, len(dir))
	for _, e := range dir {
		byID[e.ID] = e
	}
	return byID, nil
}

// handleLANRegistry GET /api/v1/lan-registry 只读列表。
func (s *Server) handleLANRegistry(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	viaFilter, _ := strconv.ParseInt(r.URL.Query().Get("user_id"), 10, 64)
	rows, err := s.store.ListClientLANRegistry(viaFilter)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	byID, err := s.userDirMap()
	if err != nil {
		writeInternalError(w, err)
		return
	}
	items := make([]lanRegistryView, 0, len(rows))
	for _, row := range rows {
		v := lanRegistryView{
			UserID: row.UserID, DestCIDR: row.DestCIDR, VPNIP: row.VPNIP,
			HostID: row.HostID, UpdatedAt: timeutil.FormatRFC3339(row.UpdatedAt),
		}
		if u, ok := byID[row.UserID]; ok {
			v.Username = u.Username
		}
		items = append(items, v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// handlePeerRoutes GET 列表 / POST 新增托管路由（只写库，不踢线）。
func (s *Server) handlePeerRoutes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listPeerRoutes(w, r)
	case http.MethodPost:
		s.createPeerRoute(w, r)
	default:
		writeMethodNotAllowed(w)
	}
}

// handlePeerRouteByID DELETE 或 PUT members：/api/v1/peer-routes/{id}
func (s *Server) handlePeerRouteByID(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/v1/peer-routes/")
	idStr = strings.Trim(idStr, "/")
	// 子路径 members
	if strings.HasSuffix(idStr, "/members") {
		idStr = strings.TrimSuffix(idStr, "/members")
		idStr = strings.Trim(idStr, "/")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || id <= 0 {
			writeAPIError(w, http.StatusBadRequest, "无效路由 id")
			return
		}
		s.replacePeerRouteMembers(w, r, id)
		return
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeAPIError(w, http.StatusBadRequest, "无效路由 id")
		return
	}
	if r.Method != http.MethodDelete {
		writeMethodNotAllowed(w)
		return
	}
	old, err := s.store.GetPeerRoute(id)
	if err != nil || old == nil {
		writeAPIError(w, http.StatusNotFound, "路由不存在")
		return
	}
	if err := s.store.DeletePeerRoute(id); err != nil {
		writeInternalError(w, err)
		return
	}
	s.audit.Log(s.actorFromRequest(r), "peer_route_delete", "peer_route", &id, s.clientIP(r), map[string]string{
		"dest": old.DestCIDR, "via_user_id": strconv.FormatInt(old.ViaUserID, 10),
	})
	s.markDirtyForMembers(old.MemberUserIDs)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "pending_apply": true})
}

func (s *Server) markDirtyForMembers(members []int64) {
	if persist.PeerRouteHasAllMembers(members) {
		s.markPeerDirtyAll()
		return
	}
	s.markPeerDirtyUsers(members...)
}

func (s *Server) replacePeerRouteMembers(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var body struct {
		MemberUserIDs []int64 `json:"member_user_ids"`
		ApplyAll      bool    `json:"apply_all"`
	}
	if !decodeJSONBody(w, r, &body) {
		return
	}
	if body.ApplyAll {
		body.MemberUserIDs = []int64{persist.PeerRouteMemberAll}
	}
	if err := s.store.ReplacePeerRouteMembers(id, body.MemberUserIDs); err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.audit.Log(s.actorFromRequest(r), "peer_route_members", "peer_route", &id, s.clientIP(r), nil)
	s.markDirtyForMembers(persist.NormalizeMemberUserIDs(body.MemberUserIDs))
	rt, _ := s.store.GetPeerRoute(id)
	byID, _ := s.userDirMap()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "pending_apply": true, "item": s.toPeerRouteView(*rt, byID),
	})
}

func (s *Server) listPeerRoutes(w http.ResponseWriter, r *http.Request) {
	routes, err := s.store.ListPeerRoutes()
	if err != nil {
		writeInternalError(w, err)
		return
	}
	byID, err := s.userDirMap()
	if err != nil {
		writeInternalError(w, err)
		return
	}
	items := make([]peerRouteView, 0, len(routes))
	for _, rt := range routes {
		items = append(items, s.toPeerRouteView(rt, byID))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) createPeerRoute(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DestCIDR      string  `json:"dest_cidr"`
		ViaUserID     int64   `json:"via_user_id"`
		UserID        *int64  `json:"user_id"` // 兼容旧字段：单访问方
		MemberUserIDs []int64 `json:"member_user_ids"`
		ApplyAll      bool    `json:"apply_all"`
	}
	if !decodeJSONOrForm(w, r, &body, func() {
		body.DestCIDR = r.FormValue("dest_cidr")
		body.ViaUserID, _ = strconv.ParseInt(r.FormValue("via_user_id"), 10, 64)
		body.ApplyAll = r.FormValue("apply_all") == "1" || r.FormValue("apply_all") == "true"
		if uid := r.FormValue("user_id"); uid != "" && !body.ApplyAll {
			v, _ := strconv.ParseInt(uid, 10, 64)
			body.UserID = &v
		}
	}) {
		return
	}
	members := body.MemberUserIDs
	if body.ApplyAll {
		members = []int64{persist.PeerRouteMemberAll}
	} else if len(members) == 0 && body.UserID != nil {
		members = []int64{*body.UserID}
	}
	if body.ViaUserID <= 0 {
		writeAPIError(w, http.StatusBadRequest, "须指定 via 账号")
		return
	}
	via, err := s.store.GetUserByID(body.ViaUserID)
	if err != nil || via == nil {
		writeAPIError(w, http.StatusBadRequest, "via 账号不存在")
		return
	}
	id, err := s.store.InsertPeerRoute(body.DestCIDR, body.ViaUserID, members)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.audit.Log(s.actorFromRequest(r), "peer_route_create", "peer_route", &id, s.clientIP(r), map[string]string{
		"dest": body.DestCIDR, "via_user_id": strconv.FormatInt(body.ViaUserID, 10),
		"apply_all": strconv.FormatBool(persist.PeerRouteHasAllMembers(members)),
	})
	s.markDirtyForMembers(persist.NormalizeMemberUserIDs(members))
	rt, _ := s.store.GetPeerRoute(id)
	byID, _ := s.userDirMap()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "pending_apply": true, "item": s.toPeerRouteView(*rt, byID),
	})
}

func (s *Server) toPeerRouteView(rt persist.PeerRoute, byID map[int64]persist.UserDirectoryEntry) peerRouteView {
	v := peerRouteView{
		ID: rt.ID, DestCIDR: rt.DestCIDR, ViaUserID: rt.ViaUserID,
		MemberUserIDs: rt.MemberUserIDs, Scope: "user",
	}
	if persist.PeerRouteHasAllMembers(rt.MemberUserIDs) {
		v.Scope = "all"
		v.MemberNames = "全部账号"
	} else {
		var names []string
		for _, mid := range rt.MemberUserIDs {
			if u, ok := byID[mid]; ok {
				names = append(names, u.Username)
			} else {
				names = append(names, fmt.Sprintf("#%d", mid))
			}
		}
		v.MemberNames = strings.Join(names, ", ")
	}
	via := byID[rt.ViaUserID]
	v.ViaUsername = via.Username
	v.ViaVPNIP = strings.TrimSpace(via.VPNIP)
	regOK := false
	if ok, err := s.store.HasLanRegistryMatch(rt.ViaUserID, rt.DestCIDR); err == nil {
		regOK = ok
	}
	v.ViaOffline = v.ViaVPNIP == ""
	v.Stale = v.ViaOffline || !regOK
	name := v.ViaUsername
	if name == "" {
		name = fmt.Sprintf("user#%d", rt.ViaUserID)
	}
	switch {
	case v.Stale && v.ViaOffline:
		v.Display = fmt.Sprintf("%s via %s(离线·失效)", rt.DestCIDR, name)
	case v.Stale:
		v.Display = fmt.Sprintf("%s via %s(注册失效)", rt.DestCIDR, v.ViaVPNIP)
	default:
		v.Display = fmt.Sprintf("%s via %s", rt.DestCIDR, v.ViaVPNIP)
	}
	return v
}

// handlePeersApply GET 待应用状态 / POST 应用生效（bump + 踢受影响账号）。
func (s *Server) handlePeersApply(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		pending, all, ids := s.peerDirtyStatus()
		writeJSON(w, http.StatusOK, map[string]any{
			"pending_apply": pending, "all": all, "user_ids": ids,
		})
	case http.MethodPost:
		s.applyPeerPolicy(w, r)
	default:
		writeMethodNotAllowed(w)
	}
}

// applyPeerPolicy 对脏账号递增 policy_ver 并 KickUser，使在线客户端重连拿新策略。
func (s *Server) applyPeerPolicy(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ForceAll bool `json:"force_all"`
	}
	// 空体视为 force_all=false；表单/JSON 均可
	if !decodeJSONOrForm(w, r, &body, func() {
		body.ForceAll = r.FormValue("force_all") == "1" || r.FormValue("force_all") == "true"
	}) {
		return
	}

	s.peerDirtyMu.Lock()
	forceAll := body.ForceAll || s.peerDirtyAll
	ids := make([]int64, 0, len(s.peerDirtyIDs))
	for id := range s.peerDirtyIDs {
		ids = append(ids, id)
	}
	s.peerDirtyMu.Unlock()

	if forceAll {
		dir, err := s.store.ListUserDirectory()
		if err != nil {
			writeInternalError(w, err)
			return
		}
		ids = ids[:0]
		for _, e := range dir {
			if e.IsAdmin && !e.HasVPN {
				continue
			}
			if !e.HasVPN {
				continue
			}
			ids = append(ids, e.ID)
		}
	}

	if len(ids) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": true, "kicked": 0, "message": "无待应用变更",
		})
		return
	}

	kicked := 0
	for _, id := range ids {
		if _, err := s.store.IncrementPolicyVer(id); err != nil {
			logger.Warn("应用生效 IncrementPolicyVer 失败 user_id=%d: %v", id, err)
			continue
		}
		if s.sessions != nil {
			s.sessions.KickUser(id)
		}
		kicked++
	}
	s.clearPeerDirty()
	s.audit.Log(s.actorFromRequest(r), "peers_apply", "peer_policy", nil, s.clientIP(r), map[string]string{
		"kicked": strconv.Itoa(kicked), "force_all": strconv.FormatBool(forceAll),
	})
	logger.Info("peers 应用生效 kicked=%d force_all=%v", kicked, forceAll)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "kicked": kicked, "user_ids": ids,
		"message": fmt.Sprintf("已踢线 %d 个账号以刷新策略", kicked),
	})
}

// handlePeerAccess GET/POST 互访白名单；DELETE 用 query（只写库，不踢线）。
func (s *Server) handlePeerAccess(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listPeerAccess(w, r)
	case http.MethodPost:
		s.addPeerAccess(w, r)
	case http.MethodDelete:
		s.removePeerAccess(w, r)
	default:
		writeMethodNotAllowed(w)
	}
}

func (s *Server) listPeerAccess(w http.ResponseWriter, r *http.Request) {
	byID, err := s.userDirMap()
	if err != nil {
		writeInternalError(w, err)
		return
	}
	acc, err := s.store.ListAllPeerAccess()
	if err != nil {
		writeInternalError(w, err)
		return
	}
	filterUID, _ := strconv.ParseInt(r.URL.Query().Get("user_id"), 10, 64)
	var items []peerAccessView
	for _, a := range acc {
		if filterUID > 0 && a.UserID != filterUID {
			continue
		}
		pv := peerAccessView{UserID: a.UserID, PeerUserID: a.PeerUserID}
		if u, ok := byID[a.UserID]; ok {
			pv.Username = u.Username
		}
		if p, ok := byID[a.PeerUserID]; ok {
			pv.PeerUsername = p.Username
			pv.PeerVPNIP = p.VPNIP
		}
		items = append(items, pv)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) addPeerAccess(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UserID     int64 `json:"user_id"`
		PeerUserID int64 `json:"peer_user_id"`
	}
	if !decodeJSONOrForm(w, r, &body, func() {
		body.UserID, _ = strconv.ParseInt(r.FormValue("user_id"), 10, 64)
		body.PeerUserID, _ = strconv.ParseInt(r.FormValue("peer_user_id"), 10, 64)
	}) {
		return
	}
	if err := s.store.AddPeerAccessPair(body.UserID, body.PeerUserID); err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.audit.Log(s.actorFromRequest(r), "peer_access_add", "user", &body.UserID, s.clientIP(r), map[string]string{
		"peer_user_id": strconv.FormatInt(body.PeerUserID, 10), "bidirectional": "true",
	})
	s.markPeerDirtyUsers(body.UserID, body.PeerUserID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "pending_apply": true})
}

func (s *Server) removePeerAccess(w http.ResponseWriter, r *http.Request) {
	uid, _ := strconv.ParseInt(r.URL.Query().Get("user_id"), 10, 64)
	pid, _ := strconv.ParseInt(r.URL.Query().Get("peer_user_id"), 10, 64)
	if uid <= 0 || pid <= 0 {
		writeAPIError(w, http.StatusBadRequest, "须提供 user_id 与 peer_user_id")
		return
	}
	if err := s.store.RemovePeerAccessPair(uid, pid); err != nil {
		writeInternalError(w, err)
		return
	}
	s.audit.Log(s.actorFromRequest(r), "peer_access_remove", "user", &uid, s.clientIP(r), map[string]string{
		"peer_user_id": strconv.FormatInt(pid, 10), "bidirectional": "true",
	})
	s.markPeerDirtyUsers(uid, pid)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "pending_apply": true})
}

// handleVPNPeersPolicy GET/PUT 全局互访开关（内存即时生效，无需踢线）。
func (s *Server) handleVPNPeersPolicy(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		allow := s.cfg.Security.AllowAllVPNPeers
		if v, ok, err := s.store.GetAllowAllVPNPeersSetting(); err == nil && ok {
			allow = v
		}
		writeJSON(w, http.StatusOK, map[string]any{"allow_all_vpn_peers": allow})
	case http.MethodPut, http.MethodPost:
		var body struct {
			AllowAllVPNPeers bool `json:"allow_all_vpn_peers"`
		}
		if !decodeJSONOrForm(w, r, &body, func() {
			body.AllowAllVPNPeers = r.FormValue("allow_all_vpn_peers") == "1" || r.FormValue("allow_all_vpn_peers") == "true"
		}) {
			return
		}
		if err := s.store.SetAllowAllVPNPeersSetting(body.AllowAllVPNPeers); err != nil {
			writeInternalError(w, err)
			return
		}
		s.cfg.Security.AllowAllVPNPeers = body.AllowAllVPNPeers
		if s.sessions != nil {
			s.sessions.SetAllowAllVPNPeers(body.AllowAllVPNPeers)
		}
		s.audit.Log(s.actorFromRequest(r), "vpn_peers_policy", "security", nil, s.clientIP(r), map[string]string{
			"allow_all_vpn_peers": strconv.FormatBool(body.AllowAllVPNPeers),
		})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "allow_all_vpn_peers": body.AllowAllVPNPeers})
	default:
		writeMethodNotAllowed(w)
	}
}

