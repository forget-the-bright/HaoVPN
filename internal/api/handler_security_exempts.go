package api

import (
	"net/http"
	"strings"

	"haovpn/internal/paginate"
	"haovpn/internal/persist"
)

// handleSecurityExempts 封禁豁免列表或新增（GET/POST /api/v1/security/exempts）。
func (s *Server) handleSecurityExempts(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet, http.MethodPost) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		limit, offset := paginate.ParseLimitOffset(q, 50, 500)
		only := paginate.ParseOnlyEnabled(q)
		items, total, err := s.store.ListBanExempt(persist.BanExemptFilter{
			OnlyEnabled: only,
			Limit:       limit,
			Offset:      offset,
		})
		if err != nil {
			writeInternalError(w, err)
			return
		}
		writePage(w, http.StatusOK, toExemptViews(items), total, limit, offset)
	case http.MethodPost:
		var body struct {
			IP   string `json:"ip"`
			Note string `json:"note"`
		}
		if !decodeJSONBody(w, r, &body) {
			return
		}
		ip := strings.TrimSpace(body.IP)
		if ip == "" {
			writeAPIError(w, http.StatusBadRequest, "ip 不能为空")
			return
		}
		if err := validateBanExemptIP(ip); err != nil {
			writeDomainError(w, err)
			return
		}
		if err := s.store.UpsertBanExempt(ip, body.Note, "manual"); err != nil {
			writeInternalError(w, err)
			return
		}
		if err := s.reloadProbeBanExempt(); err != nil {
			writeInternalError(w, err)
			return
		}
		if s.probeGuard != nil {
			_ = s.probeGuard.Unban(ip)
		}
		s.audit.Log(s.actorFromRequest(r), "probe_exempt_add", "ip", nil, s.clientIP(r), map[string]string{
			"ip": ip, "note": strings.TrimSpace(body.Note),
		})
		writeOKWith(w, map[string]any{"ip": ip})
	}
}

// handleSecurityExemptByIP 移除封禁豁免（DELETE /api/v1/security/exempts/{ip}）。
func (s *Server) handleSecurityExemptByIP(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodDelete) {
		return
	}
	ip, ok := parsePathSuffixIP(w, r, "/api/v1/security/exempts/", true)
	if !ok {
		return
	}
	if err := s.store.DisableBanExempt(ip); err != nil {
		writeInternalError(w, err)
		return
	}
	if err := s.reloadProbeBanExempt(); err != nil {
		writeInternalError(w, err)
		return
	}
	s.audit.Log(s.actorFromRequest(r), "probe_exempt_remove", "ip", nil, s.clientIP(r), map[string]string{"ip": ip})
	writeOKWith(w, map[string]any{"ip": ip})
}
