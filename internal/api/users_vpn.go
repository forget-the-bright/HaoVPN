package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"haovpn/internal/auth"
	"haovpn/internal/vpnaccount"
)

// handleUserVPNPatch 更新账号 VPN 策略（PATCH /api/v1/users/{id}/vpn）。
func (s *Server) handleUserVPNPatch(w http.ResponseWriter, r *http.Request, id int64, se auth.SessionEntry) {
	var body struct {
		AllowedIPs *[]string `json:"allowed_ips"`
		IPMode     string    `json:"ip_mode"`
		IPLeaseSec int       `json:"ip_lease_sec"`
		VPNIP      *string   `json:"vpn_ip"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeAPIError(w, http.StatusBadRequest, "无效 JSON")
		return
	}
	result, err := s.vpnSvc.ApplyVPNPatch(id, vpnaccount.VPNPatchInput{
		AllowedIPs: body.AllowedIPs, IPMode: body.IPMode, IPLeaseSec: body.IPLeaseSec, VPNIP: body.VPNIP,
	})
	if err != nil {
		if strings.Contains(err.Error(), "不存在") {
			writeAPIError(w, http.StatusNotFound, err.Error())
			return
		}
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.audit.Log(&se.UserID, "policy_change_kick", "user", &id, clientIP(r), map[string]string{
		"policy_ver": fmt.Sprintf("%d", result.PolicyVer), "vpn_ip": result.NewIP, "ip_mode": result.NewMode,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "policy_ver": result.PolicyVer, "vpn_ip": result.NewIP, "ip_mode": result.NewMode,
	})
}

// handleUserPasswordReset 管理员重置指定账号登录密码。
func (s *Server) handleUserPasswordReset(w http.ResponseWriter, r *http.Request, id int64, se auth.SessionEntry) {
	_ = parseRequestForm(r)
	newPass := r.FormValue("new_password")
	if err := s.auth.ResetPasswordByAdmin(id, newPass); err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	u, err := s.store.GetUserByID(id)
	if err != nil {
		writeAPIError(w, http.StatusNotFound, "账号不存在")
		return
	}
	s.sessions.KickUser(id)
	s.audit.Log(&se.UserID, "admin_reset_password", "user", &id, clientIP(r), map[string]string{"username": u.Username})
	writeOK(w)
}
