package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"haovpn/internal/auth"
	"haovpn/internal/logger"
	"haovpn/internal/netutil"
	"haovpn/internal/persist"
	"haovpn/internal/readmodel"
	"haovpn/internal/vpnaccount"
)

// handleUsers VPN 账号列表与新建（GET/POST /api/v1/users）。
//
// GET 支持 q、enabled、online、limit、offset 筛选；POST 经 vpnSvc.ProvisionWebAccount 开户并写审计。
// 返回：分页 items 或新建账号 id/vpn_ip/export_zip_url。
func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		limit := clampLimit(parseIntDefault(q.Get("limit"), 50), 50, 500)
		offset := parseIntDefault(q.Get("offset"), 0)
		enabled := -1
		useEnabled := false
		if v := q.Get("enabled"); v == "1" || v == "true" {
			enabled = 1
			useEnabled = true
		} else if v == "0" || v == "false" {
			enabled = 0
			useEnabled = true
		}
		items, total, err := s.store.ListUsersPage(readmodel.UserListFilter{
			Q: q.Get("q"), Enabled: enabled, UseEnabled: useEnabled, Limit: limit, Offset: offset,
		})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		onlineOnly := q.Get("online") == "1" || q.Get("online") == "true"
		var out []readmodel.UserListAccountView
		for _, u := range items {
			_, online := s.sessions.GetSession(u.ID)
			if onlineOnly && !online {
				continue
			}
			out = append(out, readmodel.UserListItemToAccountView(u, online))
		}
		if onlineOnly {
			total = len(out)
		}
		writePage(w, http.StatusOK, out, total, limit, offset)
	case http.MethodPost:
		_ = parseRequestForm(r)
		username := r.FormValue("username")
		password := r.FormValue("password")
		ipMode := r.FormValue("ip_mode")
		if ipMode == "" {
			ipMode = persist.IPModeFixed
		}
		ipLeaseSec, _ := strconv.Atoi(r.FormValue("ip_lease_sec"))
		requestedIP := strings.TrimSpace(r.FormValue("vpn_ip"))
		hash, err := auth.HashPassword(password)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		res, err := s.vpnSvc.ProvisionWebAccount(vpnaccount.ProvisionInput{
			Username:     username,
			PasswordHash: hash,
			IPMode:       ipMode,
			IPLeaseSec:   ipLeaseSec,
			AllowedIPs:   nil,
			RequestedIP:  requestedIP,
			KeyEnc:       s.keyEnc,
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		id, vpnIP := res.UserID, res.VPNIP
		se, _ := s.sessionFromRequest(r)
		s.audit.Log(&se.UserID, "account_create", "user", &id, clientIP(r), map[string]string{"username": username, "vpn_ip": vpnIP})
		writeJSON(w, http.StatusOK, map[string]any{
			"id": id, "username": username, "vpn_ip": vpnIP, "ip_mode": ipMode,
			"policy_ver": 1, "export_zip_url": fmt.Sprintf("/api/v1/users/%d/export.zip", id),
		})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, nil)
	}
}

// handleUserByID 单账号子路由分发（/api/v1/users/{id}/…）。
//
// 支持：DELETE 删号、POST disable/enable、POST password（管理员改密）、GET export.zip/export、POST kick、PATCH vpn 策略。
// 副作用：禁用/改策略/删除时可能 KickUser 并写审计。
func (s *Server) handleUserByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/users/")
	parts := strings.Split(path, "/")
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效 ID"})
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
		s.audit.Log(&se.UserID, "kick_account", "user", &id, clientIP(r), nil)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
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
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		s.audit.Log(&se.UserID, "account_delete", "user", &id, clientIP(r), nil)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case http.MethodPost:
		_ = parseRequestForm(r)
		action := r.FormValue("action")
		if action == "disable" {
			if err := s.store.SetUserEnabled(id, false); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			s.sessions.KickUser(id)
			logger.Info("账号已禁用并踢线: user_id=%d", id)
			s.audit.Log(&se.UserID, "user_disable", "user", &id, clientIP(r), nil)
		} else if action == "enable" {
			if err := s.store.SetUserEnabled(id, true); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			s.audit.Log(&se.UserID, "user_enable", "user", &id, clientIP(r), nil)
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, nil)
	}
}

// handleUserVPNPatch 更新账号 VPN 策略（PATCH /api/v1/users/{id}/vpn）。
//
// 参数：JSON allowed_ips、ip_mode、ip_lease_sec、vpn_ip；经 vpnSvc.PlanVPNPatch 校验后写 DB。
// 副作用：递增 policy_ver、KickUser 使新策略生效、写审计 policy_change_kick。
func (s *Server) handleUserVPNPatch(w http.ResponseWriter, r *http.Request, id int64, se auth.SessionEntry) {
	var body struct {
		AllowedIPs *[]string `json:"allowed_ips"`
		IPMode     string    `json:"ip_mode"`
		IPLeaseSec int       `json:"ip_lease_sec"`
		VPNIP      *string   `json:"vpn_ip"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效 JSON"})
		return
	}
	u, err := s.store.GetUserByID(id)
	if err != nil || !u.HasVPN() {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "账号不存在或无 VPN 身份"})
		return
	}
	plan, err := s.vpnSvc.PlanVPNPatch(u, vpnaccount.VPNPatchInput{
		AllowedIPs: body.AllowedIPs,
		IPMode:     body.IPMode,
		IPLeaseSec: body.IPLeaseSec,
		VPNIP:      body.VPNIP,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	newIP, newMode, allowed, leaseSec := plan.NewIP, plan.NewMode, plan.AllowedIPs, plan.IPLeaseSec
	if norm, err := netutil.NormalizeIPv4(newIP); err == nil && newIP != "" {
		newIP = norm
	}

	pv, err := s.store.IncrementPolicyVer(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := s.store.UpdateVPNFields(id, newIP, allowed, newMode, leaseSec, pv); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.sessions.KickUser(id)
	s.audit.Log(&se.UserID, "policy_change_kick", "user", &id, clientIP(r), map[string]string{
		"policy_ver": fmt.Sprintf("%d", pv), "vpn_ip": newIP, "ip_mode": newMode,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "policy_ver": pv, "vpn_ip": newIP, "ip_mode": newMode})
}

// handleUserPasswordReset 管理员重置指定账号登录密码（POST /api/v1/users/{id}/password）。
//
// 参数：表单 new_password（≥8 位）；须已登录 Web 管理员（当前仅 admin 可登录）。
// 副作用：更新 password_hash、清除 must_change_password、KickUser、写审计 admin_reset_password。
func (s *Server) handleUserPasswordReset(w http.ResponseWriter, r *http.Request, id int64, se auth.SessionEntry) {
	_ = parseRequestForm(r)
	newPass := r.FormValue("new_password")
	hash, err := auth.HashPassword(newPass)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	u, err := s.store.GetUserByID(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "账号不存在"})
		return
	}
	if err := s.store.UpdateUserPassword(id, hash, true); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "更新失败"})
		return
	}
	s.sessions.KickUser(id)
	s.audit.Log(&se.UserID, "admin_reset_password", "user", &id, clientIP(r), map[string]string{"username": u.Username})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// loadExportAccount 加载可导出账号并解密私钥（ZIP/YAML 导出共用）。
//
// 返回：用户记录、明文私钥；账号不存在、无 VPN 身份或解密失败时 err 非 nil。
func (s *Server) loadExportAccount(id int64) (*persist.User, string, error) {
	u, err := s.store.GetUserByID(id)
	if err != nil || !u.HasVPN() {
		return nil, "", fmt.Errorf("账号不存在或无 VPN 配置")
	}
	plainKey, err := vpnaccount.OpenAccountPrivateKey(u, s.keyEnc)
	if err != nil {
		return nil, "", fmt.Errorf("私钥解密失败")
	}
	return u, plainKey, nil
}

// handleUserExportZip 下载客户端配置 ZIP（GET /api/v1/users/{id}/export.zip）。
//
// 副作用：写审计 config_export；响应 Content-Disposition 附件。
func (s *Server) handleUserExportZip(w http.ResponseWriter, r *http.Request, id int64, se auth.SessionEntry) {
	u, plainKey, err := s.loadExportAccount(id)
	if err != nil {
		writeAPIError(w, http.StatusNotFound, err.Error())
		return
	}
	zipBytes, err := buildAccountExportZip(s.cfg, u, plainKey, s.serverPK)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.audit.Log(&se.UserID, "config_export", "user", &id, clientIP(r), map[string]string{"format": "zip"})
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=haovpn-client-%s.zip", u.Username))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(zipBytes)
}

// handleUserExportYAML 下载客户端 YAML 配置（GET /api/v1/users/{id}/export）。
//
// 副作用：写审计 config_export；YAML 不含明文私钥（登录后握手下发）。
func (s *Server) handleUserExportYAML(w http.ResponseWriter, r *http.Request, id int64, se auth.SessionEntry) {
	u, plainKey, err := s.loadExportAccount(id)
	if err != nil {
		writeAPIError(w, http.StatusNotFound, err.Error())
		return
	}
	caFile := s.cfg.Server.TLS.CertFile
	if caFile == "" {
		caFile = "./certs/server.crt"
	}
	yaml := buildClientExportYAML(s.cfg, u, plainKey, s.serverPK, caFile)
	s.audit.Log(&se.UserID, "config_export", "user", &id, clientIP(r), nil)
	w.Header().Set("Content-Type", "application/x-yaml")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=client-%s.yaml", u.Username))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(yaml))
}

