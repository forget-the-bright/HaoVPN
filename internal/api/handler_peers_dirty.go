package api

import (
	"net/http"

	"haovpn/internal/persist"
)

func (s *Server) actorFromRequest(r *http.Request) *int64 {
	se, ok := s.sessionFromRequest(r)
	if !ok {
		return nil
	}
	id := se.UserID
	return &id
}

// markPeerDirtyUsers 标记指定账号须「应用生效」（委托 vpnaccount.PeerPolicyApplier）。
func (s *Server) markPeerDirtyUsers(ids ...int64) {
	s.peerPolicy.MarkUsers(ids...)
}

// markPeerDirtyAll 标记全部 VPN 账号须应用生效。
func (s *Server) markPeerDirtyAll() {
	s.peerPolicy.MarkAll()
}

// peerDirtyStatus 返回待应用状态（控制台黄条）。
func (s *Server) peerDirtyStatus() (pending bool, all bool, ids []int64) {
	return s.peerPolicy.Status()
}

// clearPeerDirty 应用生效成功后清空全部脏标记（仅无失败场景或测试）。
func (s *Server) clearPeerDirty() {
	s.peerPolicy.Clear()
}

// userDirMap 轻量账号目录 id→条目（无私钥）。
func (s *Server) userDirMap() (map[int64]persist.UserDirectoryEntry, error) {
	dir, err := s.store.ListUserDirectory()
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]persist.UserDirectoryEntry, len(dir))
	for _, e := range dir {
		byID[e.ID] = e
	}
	return byID, nil
}
