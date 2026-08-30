package api

import (
	"net/http"
	"strconv"

	"haovpn/internal/paginate"
)

// handlePeersApply GET 待应用状态 / POST 应用生效（领域逻辑在 vpnaccount.PeerPolicyApplier）。
func (s *Server) handlePeersApply(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet, http.MethodPost) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		pending, all, ids := s.peerDirtyStatus()
		writeJSON(w, http.StatusOK, map[string]any{
			"pending_apply": pending, "all": all, "user_ids": ids,
		})
	case http.MethodPost:
		s.applyPeerPolicy(w, r)
	}
}

// applyPeerPolicy HTTP 薄层：解析 force_all → PeerPolicyApplier.Apply → 审计与 JSON。
func (s *Server) applyPeerPolicy(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ForceAll bool `json:"force_all"`
	}
	if !decodeJSONOrForm(w, r, &body, func() {
		if v, ok := paginate.ParseBoolQuery(r.FormValue("force_all")); ok {
			body.ForceAll = v
		}
	}) {
		return
	}

	res, err := s.peerPolicy.Apply(body.ForceAll)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	s.audit.Log(s.actorFromRequest(r), "peers_apply", "peer_policy", nil, s.clientIP(r), map[string]string{
		"kicked": strconv.Itoa(res.Kicked), "failed": strconv.Itoa(res.Failed), "force_all": strconv.FormatBool(res.ForceAll),
	})
	writeOKWith(w, map[string]any{
		"kicked": res.Kicked, "failed": res.Failed, "user_ids": res.UserIDs,
		"message": res.Message,
	})
}
