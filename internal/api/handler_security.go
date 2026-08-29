package api

import (
	"net"
	"net/http"
	"strings"
	"time"

	"haovpn/internal/paginate"
	"haovpn/internal/persist"
	"haovpn/internal/probedefense"
)

// securityEventView 安全事件 API 视图：英文码 + 中文标签（与 hardening 对照表同源）。
type securityEventView struct {
	ID          int64     `json:"id"`
	ClientIP    string    `json:"client_ip"`
	ClientPort  string    `json:"client_port,omitempty"`
	Phase       string    `json:"phase"`
	PhaseZH     string    `json:"phase_zh"`
	Signature   string    `json:"signature"`
	SignatureZH string    `json:"signature_zh"`
	Action      string    `json:"action"`
	ActionZH    string    `json:"action_zh"`
	DetailJSON  string    `json:"detail_json,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// ipBlockView 封禁列表视图（特征带中文）。
type ipBlockView struct {
	ID          int64      `json:"id"`
	IP          string     `json:"ip"`
	Reason      string     `json:"reason"`
	Source      string     `json:"source"`
	Signature   string     `json:"signature,omitempty"`
	SignatureZH string     `json:"signature_zh,omitempty"`
	Hits        int        `json:"hits"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	Enabled     bool       `json:"enabled"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	LastHitAt   *time.Time `json:"last_hit_at,omitempty"`
}

func toEventViews(items []persist.SecurityEvent) []securityEventView {
	out := make([]securityEventView, 0, len(items))
	for _, e := range items {
		out = append(out, securityEventView{
			ID: e.ID, ClientIP: e.ClientIP, ClientPort: e.ClientPort,
			Phase: e.Phase, PhaseZH: probedefense.PhaseLabel(e.Phase),
			Signature: e.Signature, SignatureZH: probedefense.SignatureLabel(e.Signature),
			Action: e.Action, ActionZH: probedefense.ActionLabel(e.Action),
			DetailJSON: e.DetailJSON, CreatedAt: e.CreatedAt,
		})
	}
	return out
}

func toBlockViews(items []persist.IPBlock) []ipBlockView {
	out := make([]ipBlockView, 0, len(items))
	for _, b := range items {
		out = append(out, ipBlockView{
			ID: b.ID, IP: b.IP, Reason: b.Reason, Source: b.Source,
			Signature: b.Signature, SignatureZH: probedefense.SignatureLabel(b.Signature),
			Hits: b.Hits, ExpiresAt: b.ExpiresAt, Enabled: b.Enabled,
			CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt, LastHitAt: b.LastHitAt,
		})
	}
	return out
}

// handleSecurityEvents 分页查询探针安全事件（GET /api/v1/security/events）。
func (s *Server) handleSecurityEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	q := r.URL.Query()
	limit, offset := paginate.ParseLimitOffset(q, 50, 500)
	items, total, err := s.store.ListSecurityEvents(persist.SecurityEventFilter{
		ClientIP:  q.Get("ip"),
		Signature: q.Get("signature"),
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writePage(w, http.StatusOK, toEventViews(items), total, limit, offset)
}

// handleSecurityBlocks 列出或手动新增封禁（GET/POST /api/v1/security/blocks）。
func (s *Server) handleSecurityBlocks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		limit, offset := paginate.ParseLimitOffset(q, 50, 500)
		only := q.Get("enabled") != "0" && q.Get("all") != "1"
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
		se, ok := s.sessionFromRequest(r)
		if !ok {
			writeAPIError(w, http.StatusUnauthorized, "未登录")
			return
		}
		var body struct {
			IP     string `json:"ip"`
			Reason string `json:"reason"`
		}
		if !decodeJSONBody(w, r, &body) {
			return
		}
		ip := strings.TrimSpace(body.IP)
		if ip == "" {
			writeAPIError(w, http.StatusBadRequest, "ip 不能为空")
			return
		}
		if net.ParseIP(ip) == nil {
			writeAPIError(w, http.StatusBadRequest, "无效 IP 地址")
			return
		}
		reason := strings.TrimSpace(body.Reason)
		if reason == "" {
			reason = "管理员手动封禁"
		}
		if s.probeGuard == nil {
			writeAPIError(w, http.StatusServiceUnavailable, "探针防御未启用")
			return
		}
		if err := s.probeGuard.ManualBan(ip, reason); err != nil {
			writeInternalError(w, err)
			return
		}
		s.audit.Log(&se.UserID, "probe_ban_manual", "ip", nil, s.clientIP(r), map[string]string{
			"ip": ip, "reason": reason,
		})
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "ip": ip})
	default:
		writeMethodNotAllowed(w)
	}
}

// handleSecurityBlockByIP 解封（DELETE /api/v1/security/blocks/{ip}）。
func (s *Server) handleSecurityBlockByIP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeMethodNotAllowed(w)
		return
	}
	se, ok := s.sessionFromRequest(r)
	if !ok {
		writeAPIError(w, http.StatusUnauthorized, "未登录")
		return
	}
	ip := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/v1/security/blocks/"))
	if ip == "" || strings.Contains(ip, "/") {
		writeAPIError(w, http.StatusBadRequest, "无效 IP")
		return
	}
	if s.probeGuard == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "探针防御未启用")
		return
	}
	if err := s.probeGuard.Unban(ip); err != nil {
		writeInternalError(w, err)
		return
	}
	s.audit.Log(&se.UserID, "probe_unban", "ip", nil, s.clientIP(r), map[string]string{"ip": ip})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "ip": ip})
}

func (s *Server) handleSecurityPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, "security_probe.html", nil)
}

