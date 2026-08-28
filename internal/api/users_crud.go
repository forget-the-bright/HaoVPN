package api

import (
	"fmt"
	"net/http"
	"strconv"
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
		items, total, err := s.store.ListUsersPage(readmodel.UserListFilter{
			Q: q.Get("q"), Enabled: enabled, UseEnabled: useEnabled, Limit: limit, Offset: offset,
		})
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, err.Error())
			return
		}
		onlineOnly, _ := paginate.ParseBoolQuery(q.Get("online"))
		online := s.onlineUserSet()
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
		}
		writePage(w, http.StatusOK, out, total, limit, offset)
	case http.MethodPost:
		if err := parseRequestForm(r); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid form data")
			return
		}
		username := r.FormValue("username")
		password := r.FormValue("password")
		ipMode := r.FormValue("ip_mode")
		if ipMode == "" {
			ipMode = persist.IPModeFixed
		}
		ipLeaseSec := paginate.ParseIntDefault(r.FormValue("ip_lease_sec"), 0)
		requestedIP := strings.TrimSpace(r.FormValue("vpn_ip"))
		hash, err := auth.HashPassword(password)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, err.Error())
			return
		}
		res, err := s.vpnSvc.ProvisionWebAccount(vpnaccount.ProvisionInput{
			Username: username, PasswordHash: hash, IPMode: ipMode, IPLeaseSec: ipLeaseSec,
			RequestedIP: requestedIP, KeyEnc: s.keyEnc,
		})
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, err.Error())
			return
		}
		id, vpnIP := res.UserID, res.VPNIP
		se, _ := s.sessionFromRequest(r)
		s.audit.Log(&se.UserID, "account_create", "user", &id, s.clientIP(r), map[string]string{"username": username, "vpn_ip": vpnIP})
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
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "无效 ID")
		return
	}
	se, _ := s.sessionFromRequest(r)

	if len(parts) > 1 && parts[1] == "export.zip" && r.Method == http.MethodGet {
		s.handleUserExportZip(w, r, id, se)
		return
	}
	if len(parts) > 1 && parts[1] == "export" && r.Method == http.MethodGet {
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
			writeAPIError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.audit.Log(&se.UserID, "account_delete", "user", &id, s.clientIP(r), nil)
		writeOK(w)
	case http.MethodPost:
		if err := parseRequestForm(r); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid form data")
			return
		}
		action := r.FormValue("action")
		if action == "disable" {
			if err := s.vpnSvc.SetAccountEnabled(id, false); err != nil {
				writeAPIError(w, http.StatusInternalServerError, err.Error())
				return
			}
			logger.Info("账号已禁用并踢线: user_id=%d", id)
			s.audit.Log(&se.UserID, "user_disable", "user", &id, s.clientIP(r), nil)
		} else if action == "enable" {
			if err := s.vpnSvc.SetAccountEnabled(id, true); err != nil {
				writeAPIError(w, http.StatusInternalServerError, err.Error())
				return
			}
			s.audit.Log(&se.UserID, "user_enable", "user", &id, s.clientIP(r), nil)
		}
		writeOK(w)
	default:
		writeMethodNotAllowed(w)
	}
}
