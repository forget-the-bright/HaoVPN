package api

import (
	"net/http"
	"strconv"

	"haovpn/internal/readmodel"
)

// handlePeerAccess GET/POST 互访白名单；DELETE 用 query（只写库，不踢线）。
func (s *Server) handlePeerAccess(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet, http.MethodPost, http.MethodDelete) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.listPeerAccess(w, r)
	case http.MethodPost:
		s.addPeerAccess(w, r)
	case http.MethodDelete:
		s.removePeerAccess(w, r)
	}
}

func (s *Server) listPeerAccess(w http.ResponseWriter, r *http.Request) {
	byID, err := s.userDirMap()
	if err != nil {
		writeInternalError(w, err)
		return
	}
	acc, err := s.store.ListAllPeerAccess()
	if err != nil {
		writeInternalError(w, err)
		return
	}
	filterUID := parseQueryInt64(r, "user_id")
	var items []readmodel.PeerAccessView
	for _, a := range acc {
		if filterUID > 0 && a.UserID != filterUID {
			continue
		}
		pv := readmodel.PeerAccessView{UserID: a.UserID, PeerUserID: a.PeerUserID}
		if u, ok := byID[a.UserID]; ok {
			pv.Username = u.Username
		}
		if p, ok := byID[a.PeerUserID]; ok {
			pv.PeerUsername = p.Username
			pv.PeerVPNIP = p.VPNIP
		}
		items = append(items, pv)
	}
	writeItems(w, items)
}

func (s *Server) addPeerAccess(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UserID     int64 `json:"user_id"`
		PeerUserID int64 `json:"peer_user_id"`
	}
	if !decodeJSONOrForm(w, r, &body, func() {
		body.UserID = parseFormInt64(r, "user_id")
		body.PeerUserID = parseFormInt64(r, "peer_user_id")
	}) {
		return
	}
	if err := s.store.AddPeerAccessPair(body.UserID, body.PeerUserID); err != nil {
		writeDomainError(w, err)
		return
	}
	s.audit.Log(s.actorFromRequest(r), "peer_access_add", "user", &body.UserID, s.clientIP(r), map[string]string{
		"peer_user_id": strconv.FormatInt(body.PeerUserID, 10), "bidirectional": "true",
	})
	s.markPeerDirtyUsers(body.UserID, body.PeerUserID)
	writePendingApply(w, nil)
}

func (s *Server) removePeerAccess(w http.ResponseWriter, r *http.Request) {
	uid := parseQueryInt64(r, "user_id")
	pid := parseQueryInt64(r, "peer_user_id")
	if uid <= 0 || pid <= 0 {
		writeAPIError(w, http.StatusBadRequest, "须提供 user_id 与 peer_user_id")
		return
	}
	if err := s.store.RemovePeerAccessPair(uid, pid); err != nil {
		writeInternalError(w, err)
		return
	}
	s.audit.Log(s.actorFromRequest(r), "peer_access_remove", "user", &uid, s.clientIP(r), map[string]string{
		"peer_user_id": strconv.FormatInt(pid, 10), "bidirectional": "true",
	})
	s.markPeerDirtyUsers(uid, pid)
	writePendingApply(w, nil)
}
