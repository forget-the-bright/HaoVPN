package api

import (
	"fmt"
	"net/http"
	"strings"

	"haovpn/internal/auth"
	"haovpn/internal/logger"
	"haovpn/internal/paginate"
	"haovpn/internal/persist"
	"haovpn/internal/readmodel"
	"haovpn/internal/vpnaccount"
)

// handleUsers VPN 账号列表与新建（GET/POST /api/v1/users）。
func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet, http.MethodPost) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		limit, offset := paginate.ParseLimitOffset(q, 50, 500)
		enabled := -1
		useEnabled := false
		if v, ok := paginate.ParseBoolQuery(q.Get("enabled")); ok {
			useEnabled = true
			if v {
				enabled = 1
			} else {
				enabled = 0
			}
		}
		onlineOnly, _ := paginate.ParseBoolQuery(q.Get("online"))
		online := s.onlineUserSet()

		// online=1 时须在全量筛选结果上计 total 再切片；不能先 DB 分页再过滤（否则 total 变成页长）。
		dbLimit, dbOffset := limit, offset
		if onlineOnly {
			dbLimit, dbOffset = 500, 0 // ClampLimit 上限；现场账号量远小于此
		}
		items, total, err := s.store.ListUsersPage(readmodel.UserListFilter{
			Q: q.Get("q"), Enabled: enabled, UseEnabled: useEnabled, Limit: dbLimit, Offset: dbOffset,
		})
		if err != nil {
			writeInternalError(w, err)
			return
		}
		var out []readmodel.UserListAccountView
		for _, u := range items {
			isOnline := online[u.ID]
			if onlineOnly && !isOnline {
				continue
			}
			out = append(out, readmodel.UserListItemToAccountView(u, isOnline))
		}
		if onlineOnly {
			total = len(out)
			if offset > total {
				offset = total
			}
			end := offset + limit
			if end > total {
				end = total
			}
			out = out[offset:end]
		}
		writePage(w, http.StatusOK, out, total, limit, offset)
	case http.MethodPost:
		if !parseFormOrError(w, r) {
			return
		}
		username := strings.TrimSpace(r.FormValue("username"))
		// 领域层 ProvisionWebAccount 仍会校验；此处提前返回便于 API 文案一致
		if err := auth.ValidateUsername(username); err != nil {
			writeDomainError(w, err)
			return
		}
		password := r.FormValue("password")
		ipMode := r.FormValue("ip_mode")
		if ipMode == "" {
			ipMode = persist.IPModeFixed
		}
		ipLeaseSec := paginate.ParseIntDefault(r.FormValue("ip_lease_sec"), 0)
		requestedIP := strings.TrimSpace(r.FormValue("vpn_ip"))
		hash, err := auth.HashPassword(password)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		res, err := s.vpnSvc.ProvisionWebAccount(vpnaccount.ProvisionInput{
			Username: username, PasswordHash: hash, IPMode: ipMode, IPLeaseSec: ipLeaseSec,
			RequestedIP: requestedIP, KeyEnc: s.keyEnc,
		})
		if err != nil {
			writeDomainError(w, err)
			return
		}
		id, vpnIP := res.UserID, res.VPNIP
		s.audit.Log(s.actorFromRequest(r), "account_create", "user", &id, s.clientIP(r), map[string]string{"username": username, "vpn_ip": vpnIP})
		writeJSON(w, http.StatusOK, map[string]any{
			"id": id, "username": username, "vpn_ip": vpnIP, "ip_mode": ipMode,
			"policy_ver": 1, "export_zip_url": fmt.Sprintf("/api/v1/users/%d/export.zip", id),
		})
	default:
		writeMethodNotAllowed(w)
	}
}

// handleUserByID 单账号子路由分发（/api/v1/users/{id}/…）。
func (s *Server) handleUserByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/users/")
	parts := strings.Split(path, "/")
	id, ok := parsePathID(w, parts[0])
	if !ok {
		return
	}
	seID := s.actorFromRequest(r)
	var se auth.SessionEntry
	if seID != nil {
		se.UserID = *seID
	}

	if len(parts) > 1 && parts[1] == "export.zip" && r.Method == http.MethodPost {
		s.handleUserExportZip(w, r, id, se)
		return
	}
	if len(parts) > 1 && parts[1] == "export" && r.Method == http.MethodPost {
		s.handleUserExportYAML(w, r, id, se)
		return
	}
	if len(parts) > 1 && parts[1] == "kick" && r.Method == http.MethodPost {
		s.sessions.KickUser(id)
		s.audit.Log(&se.UserID, "kick_account", "user", &id, s.clientIP(r), nil)
		writeOK(w)
		return
	}
	if len(parts) > 1 && parts[1] == "vpn" && r.Method == http.MethodPatch {
		s.handleUserVPNPatch(w, r, id, se)
		return
	}
	if len(parts) > 1 && parts[1] == "password" && r.Method == http.MethodPost {
		s.handleUserPasswordReset(w, r, id, se)
		return
	}

	switch r.Method {
	case http.MethodDelete:
		if err := s.vpnSvc.DeleteAccount(id); err != nil {
			writeDomainError(w, err)
			return
		}
		revoked := s.auth.LogoutAllForUser(id)
		logger.Info("账号已删除: user_id=%d web_sessions_revoked=%d", id, revoked)
		s.audit.Log(&se.UserID, "account_delete", "user", &id, s.clientIP(r), nil)
		writeOK(w)
	case http.MethodPost:
		if !parseFormOrError(w, r) {
			return
		}
		action := r.FormValue("action")
		if action == "disable" {
			if err := s.vpnSvc.SetAccountEnabled(id, false); err != nil {
				writeDomainError(w, err)
				return
			}
			revoked := s.auth.LogoutAllForUser(id)
			logger.Info("账号已禁用并踢线: user_id=%d web_sessions_revoked=%d", id, revoked)
			s.audit.Log(&se.UserID, "user_disable", "user", &id, s.clientIP(r), nil)
		} else if action == "enable" {
			if err := s.vpnSvc.SetAccountEnabled(id, true); err != nil {
				writeDomainError(w, err)
				return
			}
			s.audit.Log(&se.UserID, "user_enable", "user", &id, s.clientIP(r), nil)
		}
		writeOK(w)
	default:
		writeMethodNotAllowed(w)
	}
}
