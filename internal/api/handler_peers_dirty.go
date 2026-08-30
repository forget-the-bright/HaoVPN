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

// markPeerDirtyUsers 标记指定账号须「应用生效」后踢线刷新策略。
func (s *Server) markPeerDirtyUsers(ids ...int64) {
	s.peerDirtyMu.Lock()
	defer s.peerDirtyMu.Unlock()
	if s.peerDirtyIDs == nil {
		s.peerDirtyIDs = map[int64]struct{}{}
	}
	for _, id := range ids {
		if id > 0 {
			s.peerDirtyIDs[id] = struct{}{}
		}
	}
}

// markPeerDirtyAll 标记全部 VPN 账号须应用生效（全员托管路由变更）。
func (s *Server) markPeerDirtyAll() {
	s.peerDirtyMu.Lock()
	defer s.peerDirtyMu.Unlock()
	s.peerDirtyAll = true
}

// peerDirtyStatus 返回待应用状态（控制台黄条）。
func (s *Server) peerDirtyStatus() (pending bool, all bool, ids []int64) {
	s.peerDirtyMu.Lock()
	defer s.peerDirtyMu.Unlock()
	all = s.peerDirtyAll
	for id := range s.peerDirtyIDs {
		ids = append(ids, id)
	}
	pending = all || len(ids) > 0
	return pending, all, ids
}

// clearPeerDirty 应用生效成功后清空全部脏标记（仅无失败场景或测试）。
func (s *Server) clearPeerDirty() {
	s.peerDirtyMu.Lock()
	defer s.peerDirtyMu.Unlock()
	s.peerDirtyAll = false
	s.peerDirtyIDs = map[int64]struct{}{}
}

// clearPeerDirtyDone 仅清除本次成功 bump+kick 的脏标，保留失败与并发新增。
//
// 参数：
//   done — 已成功 IncrementPolicyVer 并 Kick 的 user_id；
//   clearAll — 本次为 forceAll 且全部目标成功时清 peerDirtyAll。
// 为何不全清：踢线循环中新产生的 dirty、以及 Increment 失败的 ID 须保留 pending，避免 UI 伪「已应用」。
func (s *Server) clearPeerDirtyDone(done []int64, clearAll bool) {
	s.peerDirtyMu.Lock()
	defer s.peerDirtyMu.Unlock()
	if clearAll {
		s.peerDirtyAll = false
	}
	for _, id := range done {
		delete(s.peerDirtyIDs, id)
	}
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
