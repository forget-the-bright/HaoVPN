package api

import (
	"fmt"
	"net/http"
	"strconv"

	"haovpn/internal/logger"
	"haovpn/internal/paginate"
)

// handlePeersApply GET 待应用状态 / POST 应用生效（bump + 踢受影响账号）。
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

// applyPeerPolicy 对脏账号递增 policy_ver 并 KickUser，使在线客户端重连拿新策略。
//
// 仅清除本次成功处理的 dirty；失败与踢线过程中新产生的脏标保留，避免 TOCTOU 伪成功。
func (s *Server) applyPeerPolicy(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ForceAll bool `json:"force_all"`
	}
	// 空体视为 force_all=false；表单/JSON 均可
	if !decodeJSONOrForm(w, r, &body, func() {
		if v, ok := paginate.ParseBoolQuery(r.FormValue("force_all")); ok {
			body.ForceAll = v
		}
	}) {
		return
	}

	s.peerDirtyMu.Lock()
	forceAll := body.ForceAll || s.peerDirtyAll
	ids := make([]int64, 0, len(s.peerDirtyIDs))
	for id := range s.peerDirtyIDs {
		ids = append(ids, id)
	}
	s.peerDirtyMu.Unlock()

	if forceAll {
		dir, err := s.store.ListUserDirectory()
		if err != nil {
			writeInternalError(w, err)
			return
		}
		ids = ids[:0]
		for _, e := range dir {
			if e.IsAdmin && !e.HasVPN {
				continue
			}
			if !e.HasVPN {
				continue
			}
			ids = append(ids, e.ID)
		}
	}

	if len(ids) == 0 {
		writeOKWith(w, map[string]any{
			"kicked": 0, "message": "无待应用变更",
		})
		return
	}

	done := make([]int64, 0, len(ids))
	failed := 0
	for _, id := range ids {
		if _, err := s.store.IncrementPolicyVer(id); err != nil {
			logger.Warn("应用生效 IncrementPolicyVer 失败 user_id=%d: %v", id, err)
			failed++
			continue
		}
		if s.sessions != nil {
			s.sessions.KickUser(id)
		}
		done = append(done, id)
	}
	// forceAll 且全部成功才清 all 标志；部分失败保留 pending
	clearAll := forceAll && failed == 0
	s.clearPeerDirtyDone(done, clearAll)
	kicked := len(done)
	s.audit.Log(s.actorFromRequest(r), "peers_apply", "peer_policy", nil, s.clientIP(r), map[string]string{
		"kicked": strconv.Itoa(kicked), "failed": strconv.Itoa(failed), "force_all": strconv.FormatBool(forceAll),
	})
	logger.Info("peers_apply kicked=%d failed=%d force_all=%v", kicked, failed, forceAll)
	msg := fmt.Sprintf("已踢线 %d 个账号以刷新策略", kicked)
	if failed > 0 {
		msg = fmt.Sprintf("已踢线 %d 个账号，%d 个失败仍待应用", kicked, failed)
	}
	writeOKWith(w, map[string]any{
		"kicked": kicked, "failed": failed, "user_ids": done,
		"message": msg,
	})
}
