package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"haovpn/internal/netutil"
	"haovpn/internal/paginate"
	"haovpn/internal/persist"
	"haovpn/internal/probedefense"
)

// handleSecurityBlocks 列出或手动新增封禁（GET/POST /api/v1/security/blocks）。
func (s *Server) handleSecurityBlocks(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet, http.MethodPost) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		limit, offset := paginate.ParseLimitOffset(q, 50, 500)
		only := paginate.ParseOnlyEnabled(q)
		items, total, err := s.store.ListIPBlocks(persist.IPBlockFilter{
			OnlyEnabled: only,
			Limit:       limit,
			Offset:      offset,
		})
		if err != nil {
			writeInternalError(w, err)
			return
		}
		writePage(w, http.StatusOK, toBlockViews(items), total, limit, offset)
	case http.MethodPost:
		var body struct {
			IP          string `json:"ip"`
			Reason      string `json:"reason"`
			DurationSec *int   `json:"duration_sec"`
		}
		if !decodeJSONBody(w, r, &body) {
			return
		}
		ip := strings.TrimSpace(body.IP)
		if ip == "" {
			writeAPIError(w, http.StatusBadRequest, "ip 不能为空")
			return
		}
		if err := netutil.ValidateIPOrCIDR("ip", ip, false); err != nil {
			writeAPIError(w, http.StatusBadRequest, err.Error())
			return
		}
		reason := strings.TrimSpace(body.Reason)
		if reason == "" {
			reason = "管理员手动封禁"
		}
		durationSec := probedefense.BanDurationUseDefault
		if body.DurationSec != nil {
			durationSec = *body.DurationSec
			if err := probedefense.ValidateManualBanDuration(durationSec); err != nil {
				writeAPIError(w, http.StatusBadRequest, err.Error())
				return
			}
		}
		if err := s.manualBanIP(ip, reason, durationSec); err != nil {
			if errors.Is(err, probedefense.ErrProbeGuardNotReady) {
				writeAPIError(w, http.StatusServiceUnavailable, err.Error())
				return
			}
			if errors.Is(err, probedefense.ErrBanExempt) || errors.Is(err, probedefense.ErrInvalidBanIP) {
				writeAPIError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeInternalError(w, err)
			return
		}
		auditMeta := map[string]string{"ip": ip, "reason": reason}
		if body.DurationSec != nil {
			auditMeta["duration_sec"] = fmt.Sprintf("%d", durationSec)
			if durationSec == 0 {
				auditMeta["permanent"] = "true"
			}
		} else {
			auditMeta["duration_sec"] = "default"
		}
		s.audit.Log(s.actorFromRequest(r), "probe_ban_manual", "ip", nil, s.clientIP(r), auditMeta)
		resp := map[string]any{"ip": ip}
		if body.DurationSec != nil {
			resp["duration_sec"] = durationSec
		}
		writeOKWith(w, resp)
	}
}

// handleSecurityBlockByIP 解封（DELETE /api/v1/security/blocks/{ip}）。
func (s *Server) handleSecurityBlockByIP(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodDelete) {
		return
	}
	ip, ok := parsePathSuffixIP(w, r, "/api/v1/security/blocks/", false)
	if !ok {
		return
	}
	if err := s.unbanIP(ip); err != nil {
		if errors.Is(err, probedefense.ErrProbeGuardNotReady) {
			writeAPIError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		writeInternalError(w, err)
		return
	}
	s.audit.Log(s.actorFromRequest(r), "probe_unban", "ip", nil, s.clientIP(r), map[string]string{"ip": ip})
	writeOKWith(w, map[string]any{"ip": ip})
}

// manualBanIP 手动封禁；须注入 probeGuard（含豁免检查与审计事件）。
func (s *Server) manualBanIP(ip, reason string, durationSec int) error {
	if s.probeGuard == nil {
		return probedefense.ErrProbeGuardNotReady
	}
	return s.probeGuard.ManualBan(ip, reason, durationSec)
}

func (s *Server) unbanIP(ip string) error {
	if s.probeGuard == nil {
		return probedefense.ErrProbeGuardNotReady
	}
	return s.probeGuard.Unban(ip)
}
