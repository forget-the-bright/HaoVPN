package api

import (
	"net/http"
	"strconv"

	"haovpn/internal/paginate"
)

// handleVPNPeersPolicy GET/PUT 全局互访开关（内存即时生效，无需踢线）。
func (s *Server) handleVPNPeersPolicy(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet, http.MethodPut, http.MethodPost) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		allow := s.cfg.Security.AllowAllVPNPeers
		if v, ok, err := s.store.GetAllowAllVPNPeersSetting(); err == nil && ok {
			allow = v
		}
		writeJSON(w, http.StatusOK, map[string]any{"allow_all_vpn_peers": allow})
	case http.MethodPut, http.MethodPost:
		var body struct {
			AllowAllVPNPeers bool `json:"allow_all_vpn_peers"`
		}
		if !decodeJSONOrForm(w, r, &body, func() {
			if v, ok := paginate.ParseBoolQuery(r.FormValue("allow_all_vpn_peers")); ok {
				body.AllowAllVPNPeers = v
			}
		}) {
			return
		}
		if err := s.store.SetAllowAllVPNPeersSetting(body.AllowAllVPNPeers); err != nil {
			writeInternalError(w, err)
			return
		}
		s.cfg.Security.AllowAllVPNPeers = body.AllowAllVPNPeers
		if s.sessions != nil {
			s.sessions.SetAllowAllVPNPeers(body.AllowAllVPNPeers)
		}
		s.audit.Log(s.actorFromRequest(r), "vpn_peers_policy", "security", nil, s.clientIP(r), map[string]string{
			"allow_all_vpn_peers": strconv.FormatBool(body.AllowAllVPNPeers),
		})
		writeOKWith(w, map[string]any{"allow_all_vpn_peers": body.AllowAllVPNPeers})
	}
}
