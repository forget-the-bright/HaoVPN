package api

import (
	"fmt"
	"net/http"

	"haovpn/internal/auth"
	"haovpn/internal/logger"
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
	if !decodeJSONBody(w, r, &body) {
		return
	}
	result, err := s.vpnSvc.ApplyVPNPatch(id, vpnaccount.VPNPatchInput{
		AllowedIPs: body.AllowedIPs, IPMode: body.IPMode, IPLeaseSec: body.IPLeaseSec, VPNIP: body.VPNIP,
	})
	if err != nil {
		if writeAccountNotFound(w, err) {
			return
		}
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.audit.Log(&se.UserID, "policy_change_kick", "user", &id, s.clientIP(r), map[string]string{
		"policy_ver": fmt.Sprintf("%d", result.PolicyVer), "vpn_ip": result.NewIP, "ip_mode": result.NewMode,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "policy_ver": result.PolicyVer, "vpn_ip": result.NewIP, "ip_mode": result.NewMode,
	})
}

// handleUserPasswordReset 管理员重置指定账号登录密码。
func (s *Server) handleUserPasswordReset(w http.ResponseWriter, r *http.Request, id int64, se auth.SessionEntry) {
	if !parseFormOrError(w, r) {
		return
	}
	u, err := s.store.GetUserByID(id)
	if err != nil || u == nil {
		writeAPIError(w, http.StatusNotFound, vpnaccount.ErrAccountNotFound.Error())
		return
	}
	newPass := r.FormValue("new_password")
	if err := s.auth.ResetPasswordByAdmin(id, newPass); err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.sessions.KickUser(id)
	revoked := s.auth.LogoutAllForUser(id)
	logger.Info("管理员重置密码并吊销 Web 会话 target=%d revoked=%d", id, revoked)
	s.audit.Log(&se.UserID, "admin_reset_password", "user", &id, s.clientIP(r), map[string]string{"username": u.Username})
	writeOK(w)
}
