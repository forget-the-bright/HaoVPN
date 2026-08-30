package vpnaccount

import (
	"fmt"
	"sync"

	"haovpn/internal/logger"
	"haovpn/internal/persist"
)

// PeerPolicyApplier 托管路由/互访变更后的「待应用」脏标记与应用生效。
//
// 为何独立于 HTTP：脏集合与 bump+kick 是领域逻辑；api 只做鉴权/解析/写响应。
// 状态仅内存：进程重启后清空（serverapp 启动 WARN）；库内策略已是权威。
// 关联：api/handler_peers_*.go、persist peer_*、sessionmgr.KickUser。
type PeerPolicyApplier struct {
	Store *persist.Store
	Kick  func(userID int64) // 通常注入 sessionmgr.KickUser；可为 nil（仅 bump）

	mu  sync.Mutex
	all bool
	ids map[int64]struct{}
}

// NewPeerPolicyApplier 构造空脏集合的应用器。
func NewPeerPolicyApplier(store *persist.Store, kick func(int64)) *PeerPolicyApplier {
	return &PeerPolicyApplier{
		Store: store,
		Kick:  kick,
		ids:   map[int64]struct{}{},
	}
}

// MarkUsers 标记指定账号须「应用生效」后踢线刷新策略。
func (p *PeerPolicyApplier) MarkUsers(ids ...int64) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ids == nil {
		p.ids = map[int64]struct{}{}
	}
	for _, id := range ids {
		if id > 0 {
			p.ids[id] = struct{}{}
		}
	}
}

// MarkAll 标记全部 VPN 账号须应用生效（全员托管路由变更）。
func (p *PeerPolicyApplier) MarkAll() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.all = true
}

// MarkMembers 按成员列表打脏：含「全部」哨兵则 MarkAll，否则 MarkUsers。
func (p *PeerPolicyApplier) MarkMembers(members []int64) {
	if persist.PeerRouteHasAllMembers(members) {
		p.MarkAll()
		return
	}
	p.MarkUsers(members...)
}

// MarkMembersUnion 对旧∪新访问方打脏（成员收窄时被移除方也须踢线）。
func (p *PeerPolicyApplier) MarkMembersUnion(oldMembers, newMembers []int64) {
	p.MarkMembers(persist.UnionMemberUserIDs(oldMembers, newMembers))
}

// Status 返回待应用状态（控制台黄条 / GET apply）。
func (p *PeerPolicyApplier) Status() (pending bool, all bool, ids []int64) {
	if p == nil {
		return false, false, nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	all = p.all
	for id := range p.ids {
		ids = append(ids, id)
	}
	pending = all || len(ids) > 0
	return pending, all, ids
}

// Clear 清空全部脏标记（测试或无失败全量成功场景）。
func (p *PeerPolicyApplier) Clear() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.all = false
	p.ids = map[int64]struct{}{}
}

// clearDone 仅清除本次成功 bump+kick 的脏标，保留失败与并发新增。
func (p *PeerPolicyApplier) clearDone(done []int64, clearAll bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if clearAll {
		p.all = false
	}
	for _, id := range done {
		delete(p.ids, id)
	}
}

// ApplyResult 应用生效结果（供 API 写 JSON / 审计）。
type ApplyResult struct {
	Kicked   int
	Failed   int
	UserIDs  []int64
	ForceAll bool
	Message  string
}

// Apply 对脏账号递增 policy_ver 并 Kick，使在线客户端重连拿新策略。
//
// 参数：forceAll — 请求强制全员，或与内存 peerDirtyAll 合并。
// 仅清除本次成功处理的 dirty；失败与踢线过程中新产生的脏标保留，避免 TOCTOU 伪成功。
func (p *PeerPolicyApplier) Apply(forceAll bool) (*ApplyResult, error) {
	if p == nil || p.Store == nil {
		return nil, fmt.Errorf("PeerPolicyApplier 未初始化")
	}

	p.mu.Lock()
	force := forceAll || p.all
	ids := make([]int64, 0, len(p.ids))
	for id := range p.ids {
		ids = append(ids, id)
	}
	p.mu.Unlock()

	if force {
		dir, err := p.Store.ListUserDirectory()
		if err != nil {
			return nil, err
		}
		ids = ids[:0]
		for _, e := range dir {
			if !e.HasVPN {
				continue
			}
			ids = append(ids, e.ID)
		}
	}

	if len(ids) == 0 {
		return &ApplyResult{Message: "无待应用变更", ForceAll: force}, nil
	}

	done := make([]int64, 0, len(ids))
	failed := 0
	for _, id := range ids {
		if _, err := p.Store.IncrementPolicyVer(id); err != nil {
			logger.Warn("应用生效 IncrementPolicyVer 失败 user_id=%d: %v", id, err)
			failed++
			continue
		}
		if p.Kick != nil {
			p.Kick(id)
		}
		done = append(done, id)
	}
	clearAll := force && failed == 0
	p.clearDone(done, clearAll)
	kicked := len(done)
	logger.Info("peers_apply kicked=%d failed=%d force_all=%v", kicked, failed, force)
	msg := fmt.Sprintf("已踢线 %d 个账号以刷新策略", kicked)
	if failed > 0 {
		msg = fmt.Sprintf("已踢线 %d 个账号，%d 个失败仍待应用", kicked, failed)
	}
	return &ApplyResult{
		Kicked: kicked, Failed: failed, UserIDs: done, ForceAll: force, Message: msg,
	}, nil
}
