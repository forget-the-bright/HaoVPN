package api

import (
	"net/http"
	"time"

	"haovpn/internal/netutil"
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

// ipBanExemptView 封禁豁免 API 视图。
type ipBanExemptView struct {
	ID        int64     `json:"id"`
	IP        string    `json:"ip"`
	Note      string    `json:"note"`
	Source    string    `json:"source"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
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

func toExemptViews(items []persist.IPBanExempt) []ipBanExemptView {
	out := make([]ipBanExemptView, 0, len(items))
	for _, e := range items {
		out = append(out, ipBanExemptView{
			ID: e.ID, IP: e.IP, Note: e.Note, Source: e.Source,
			Enabled: e.Enabled, CreatedAt: e.CreatedAt,
		})
	}
	return out
}

func (s *Server) reloadProbeBanExempt() error {
	if s.probeGuard == nil {
		return nil
	}
	return s.probeGuard.ReloadBanExempt()
}

func (s *Server) handleSecurityPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, "security_probe.html", nil)
}

// validateBanExemptIP 校验豁免条目为合法 IP 或 CIDR。
func validateBanExemptIP(ip string) error {
	return netutil.ValidateIPOrCIDR("ip", ip, true)
}
