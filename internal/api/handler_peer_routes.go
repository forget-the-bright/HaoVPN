package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"haovpn/internal/logger"
	"haovpn/internal/paginate"
	"haovpn/internal/persist"
	"haovpn/internal/readmodel"
	"haovpn/internal/vpnaccount"
)

// handlePeerRoutes GET 列表 / POST 新增托管路由（只写库，不踢线）。
func (s *Server) handlePeerRoutes(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet, http.MethodPost) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.listPeerRoutes(w, r)
	case http.MethodPost:
		s.createPeerRoute(w, r)
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
		id, ok := parsePathID(w, idStr)
		if !ok {
			return
		}
		s.replacePeerRouteMembers(w, r, id)
		return
	}
	id, ok := parsePathID(w, idStr)
	if !ok {
		return
	}
	if !requireMethod(w, r, http.MethodDelete) {
		return
	}
	old, err := s.peerPolicy.DeletePeerRoute(id)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	s.audit.Log(s.actorFromRequest(r), "peer_route_delete", "peer_route", &id, s.clientIP(r), map[string]string{
		"dest": old.DestCIDR, "via_user_id": strconv.FormatInt(old.ViaUserID, 10),
	})
	writePendingApply(w, nil)
}

func (s *Server) replacePeerRouteMembers(w http.ResponseWriter, r *http.Request, id int64) {
	if !requireMethod(w, r, http.MethodPut, http.MethodPost) {
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
	rt, err := s.peerPolicy.ReplacePeerRouteMembers(id, body.MemberUserIDs)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	s.audit.Log(s.actorFromRequest(r), "peer_route_members", "peer_route", &id, s.clientIP(r), nil)
	byID, err := s.userDirMap()
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writePendingApply(w, map[string]any{
		"item": s.toPeerRouteView(*rt, byID),
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
	items := make([]readmodel.PeerRouteView, 0, len(routes))
	for _, rt := range routes {
		items = append(items, s.toPeerRouteView(rt, byID))
	}
	limit, offset := paginate.ParseLimitOffset(r.URL.Query(), 50, 200)
	total := len(items)
	writePage(w, http.StatusOK, slicePage(items, limit, offset), total, limit, offset)
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
		body.ViaUserID = parseFormInt64(r, "via_user_id")
		if v, ok := paginate.ParseBoolQuery(r.FormValue("apply_all")); ok {
			body.ApplyAll = v
		}
		if uid := r.FormValue("user_id"); uid != "" && !body.ApplyAll {
			v := parseFormInt64(r, "user_id")
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
	res, err := s.peerPolicy.CreatePeerRoute(vpnaccount.CreatePeerRouteInput{
		DestCIDR: body.DestCIDR, ViaUserID: body.ViaUserID, MemberUserIDs: members,
	})
	if err != nil {
		writeDomainError(w, err)
		return
	}
	id := res.ID
	s.audit.Log(s.actorFromRequest(r), "peer_route_create", "peer_route", &id, s.clientIP(r), map[string]string{
		"dest": body.DestCIDR, "via_user_id": strconv.FormatInt(body.ViaUserID, 10),
		"apply_all": strconv.FormatBool(persist.PeerRouteHasAllMembers(members)),
	})
	rt, err := s.store.GetPeerRoute(id)
	if err != nil || rt == nil {
		logger.Warn("peer_route_create 写库成功但回读失败 id=%d: %v", id, err)
		writePendingApply(w, nil)
		return
	}
	byID, err := s.userDirMap()
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writePendingApply(w, map[string]any{
		"item": s.toPeerRouteView(*rt, byID),
	})
}

func (s *Server) toPeerRouteView(rt persist.PeerRoute, byID map[int64]persist.UserDirectoryEntry) readmodel.PeerRouteView {
	v := readmodel.PeerRouteView{
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
