package api

import (
	"net/http"
	"strconv"
	"strings"

	"haovpn/internal/paginate"
	"haovpn/internal/persist"
	"haovpn/internal/readmodel"
	"haovpn/internal/vpnaccount"
)

// handler_dns_servers.go：托管 DNS HTTP 薄层（CRUD / 备注 / 成员 / 排除）。
// 写经 vpnaccount.PeerPolicyApplier；与托管路由共用 pending_apply「应用生效」。

// handleDNSServers GET 分页列表 / POST 新增手工托管 DNS。
func (s *Server) handleDNSServers(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet, http.MethodPost) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.listDNSServers(w, r)
	case http.MethodPost:
		s.createDNSServer(w, r)
	}
}

// handleDNSServerByID：DELETE；或 /remark /members /excludes 子路径。
func (s *Server) handleDNSServerByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/dns-servers/")
	rest = strings.Trim(rest, "/")
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "缺少 id"})
		return
	}
	id, ok := parsePathID(w, parts[0])
	if !ok {
		return
	}
	if len(parts) == 1 {
		if !requireMethod(w, r, http.MethodDelete) {
			return
		}
		old, err := s.peerPolicy.DeleteDNSServer(id)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		s.audit.Log(s.actorFromRequest(r), "dns_server_delete", "dns_server", &id, s.clientIP(r), map[string]string{
			"dns_ip": old.DNSIP,
		})
		writePendingApply(w, nil)
		return
	}
	switch parts[1] {
	case "remark":
		s.updateDNSRemark(w, r, id)
	case "members":
		s.replaceDNSMembers(w, r, id)
	case "excludes":
		s.replaceDNSExcludes(w, r, id)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) listDNSServers(w http.ResponseWriter, r *http.Request) {
	limit, offset := paginate.ParseLimitOffset(r.URL.Query(), 50, 200)
	rows, total, err := s.store.ListDNSServersPage(limit, offset)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	byID, err := s.userDirMap()
	if err != nil {
		writeInternalError(w, err)
		return
	}
	out := make([]readmodel.DNSServerView, len(rows))
	for i, d := range rows {
		out[i] = s.toDNSServerView(d, byID)
	}
	writePage(w, http.StatusOK, out, total, limit, offset)
}

func (s *Server) createDNSServer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DNSIP          string  `json:"dns_ip"`
		Remark         string  `json:"remark"`
		MemberUserIDs  []int64 `json:"member_user_ids"`
		ExcludeUserIDs []int64 `json:"exclude_user_ids"`
		ApplyAll       bool    `json:"apply_all"`
	}
	if !decodeJSONBody(w, r, &body) {
		return
	}
	d, err := s.peerPolicy.CreateDNSServer(vpnaccount.CreateDNSServerInput{
		DNSIP: body.DNSIP, Remark: body.Remark,
		MemberUserIDs: body.MemberUserIDs, ExcludeUserIDs: body.ExcludeUserIDs,
		ApplyAll: body.ApplyAll,
	})
	if err != nil {
		writeDomainError(w, err)
		return
	}
	s.audit.Log(s.actorFromRequest(r), "dns_server_create", "dns_server", &d.ID, s.clientIP(r), map[string]string{
		"dns_ip": d.DNSIP,
	})
	byID, err := s.userDirMap()
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writePendingApply(w, map[string]any{"item": s.toDNSServerView(*d, byID)})
}

func (s *Server) updateDNSRemark(w http.ResponseWriter, r *http.Request, id int64) {
	if !requireMethod(w, r, http.MethodPut, http.MethodPost) {
		return
	}
	var body struct {
		Remark string `json:"remark"`
	}
	if !decodeJSONBody(w, r, &body) {
		return
	}
	d, err := s.peerPolicy.UpdateDNSServerRemark(id, body.Remark)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	s.audit.Log(s.actorFromRequest(r), "dns_server_remark", "dns_server", &id, s.clientIP(r), nil)
	byID, err := s.userDirMap()
	if err != nil {
		writeInternalError(w, err)
		return
	}
	// 备注不进握手策略、不标脏；即时生效无需应用
	writeOKWith(w, map[string]any{
		"item":           s.toDNSServerView(*d, byID),
		"pending_apply":  false,
		"remark_instant": true,
		"message":        "备注已保存，无需应用生效",
	})
}

func (s *Server) replaceDNSMembers(w http.ResponseWriter, r *http.Request, id int64) {
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
		body.MemberUserIDs = []int64{persist.DNSMemberAll}
	}
	d, err := s.peerPolicy.ReplaceDNSServerMembers(id, body.MemberUserIDs)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	s.audit.Log(s.actorFromRequest(r), "dns_server_members", "dns_server", &id, s.clientIP(r), nil)
	byID, err := s.userDirMap()
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writePendingApply(w, map[string]any{"item": s.toDNSServerView(*d, byID)})
}

func (s *Server) replaceDNSExcludes(w http.ResponseWriter, r *http.Request, id int64) {
	if !requireMethod(w, r, http.MethodPut, http.MethodPost) {
		return
	}
	var body struct {
		ExcludeUserIDs []int64 `json:"exclude_user_ids"`
	}
	if !decodeJSONBody(w, r, &body) {
		return
	}
	d, err := s.peerPolicy.ReplaceDNSServerExcludes(id, body.ExcludeUserIDs)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	s.audit.Log(s.actorFromRequest(r), "dns_server_excludes", "dns_server", &id, s.clientIP(r), nil)
	byID, err := s.userDirMap()
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writePendingApply(w, map[string]any{"item": s.toDNSServerView(*d, byID)})
}

// toDNSServerView 组装展示字段（适用范围文案、排除名单名）。
func (s *Server) toDNSServerView(d persist.DNSServer, byID map[int64]persist.UserDirectoryEntry) readmodel.DNSServerView {
	v := readmodel.DNSServerView{
		ID:             d.ID,
		DNSIP:          d.DNSIP,
		Remark:         d.Remark,
		Source:         d.Source,
		MemberUserIDs:  d.MemberUserIDs,
		ExcludeUserIDs: d.ExcludeUserIDs,
		ReadonlyIP:     d.IsConfigSource(),
	}
	if persist.PeerRouteHasAllMembers(d.MemberUserIDs) {
		v.Scope = "all"
		v.MemberNames = "全部账号"
		v.CanEditExcludes = true
	} else {
		v.Scope = "user"
		v.MemberNames = joinUserNames(d.MemberUserIDs, byID)
	}
	v.ExcludeNames = joinUserNames(d.ExcludeUserIDs, byID)
	return v
}

func joinUserNames(ids []int64, byID map[int64]persist.UserDirectoryEntry) string {
	if len(ids) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if u, ok := byID[id]; ok {
			parts = append(parts, u.Username)
		} else {
			parts = append(parts, strconv.FormatInt(id, 10))
		}
	}
	return strings.Join(parts, ", ")
}
