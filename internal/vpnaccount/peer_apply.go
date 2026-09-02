package vpnaccount

import (
	"fmt"
	"sync"
	"time"

	"haovpn/internal/logger"
	"haovpn/internal/persist"
)

// PeerPolicyApplier 托管路由/互访/DNS 变更后的「待应用」脏标记与应用生效。
//
// 为何独立于 HTTP：脏集合与 bump+kick 是领域逻辑；api 只做鉴权/解析/写响应。
// 状态仅内存：进程重启后清空（serverapp 启动 WARN）；库内策略已是权威。
// 关联：api/handler_peers_*.go、persist peer_* / dns_*、sessionmgr.KickUser / ListOnline。
//
// 应用策略（防一次踢挂）：只对「脏集 ∩ 当前在线」做 IncrementPolicyVer+Kick；
// 离线账号下次握手从库读新策略，不强制 bump；大批量按批限速。
type PeerPolicyApplier struct {
	Store *persist.Store
	Kick  func(userID int64) // 通常注入 sessionmgr.KickUser；可为 nil（仅 bump）
	// ListOnline 返回当前在线 userID；nil 时不做在线过滤（单测便利）。
	ListOnline func() []int64

	mu  sync.Mutex
	all bool
	ids map[int64]struct{}
}

// applyPaceBatch / applyPaceSleep：每踢满一批后短暂休眠，避免瞬时风暴拖死数据面。
const (
	applyPaceBatch = 20
	applyPaceSleep = 50 * time.Millisecond
)

// NewPeerPolicyApplier 构造空脏集合的应用器（未注入 ListOnline 时 Apply 不过滤在线）。
func NewPeerPolicyApplier(store *persist.Store, kick func(int64)) *PeerPolicyApplier {
	return &PeerPolicyApplier{
		Store: store,
		Kick:  kick,
		ids:   map[int64]struct{}{},
	}
}

// SetListOnline 注入在线账号枚举（生产由 sessionmgr.ListOnline 注入）。
func (p *PeerPolicyApplier) SetListOnline(fn func() []int64) {
	if p == nil {
		return
	}
	p.ListOnline = fn
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

// MarkAll 标记全部 VPN 账号须应用生效（全员托管路由/全员 DNS 变更；黄条用）。
// Apply 时仍只踢当前在线，不扫离线目录做 Kick。
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
	OnlineOnly bool
	Message  string
}

// Apply 对脏账号中「当前在线」者递增 policy_ver 并 Kick，使客户端重连拿新策略。
//
// 参数：forceAll — 请求强制全员，或与内存 peerDirtyAll 合并。
// 离线脏账号：不 bump、不 Kick（库已是权威，下次握手生效），并从脏集清除以免黄条永挂。
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
		return &ApplyResult{Message: "无待应用变更", ForceAll: force, OnlineOnly: true}, nil
	}

	// 与在线求交：未注入 ListOnline 时不过滤（单测）；生产必注入。
	onlineOnly := p.ListOnline != nil
	var toKick []int64
	var offlineClear []int64
	if onlineOnly {
		online := map[int64]struct{}{}
		for _, id := range p.ListOnline() {
			if id > 0 {
				online[id] = struct{}{}
			}
		}
		for _, id := range ids {
			if _, ok := online[id]; ok {
				toKick = append(toKick, id)
			} else {
				offlineClear = append(offlineClear, id)
			}
		}
	} else {
		toKick = ids
	}

	done := make([]int64, 0, len(toKick))
	failed := 0
	paced := false
	for i, id := range toKick {
		if i > 0 && i%applyPaceBatch == 0 {
			time.Sleep(applyPaceSleep)
			paced = true
		}
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
	// 离线候选直接清脏（策略已在库；无需 Kick）
	clearIDs := append(append([]int64{}, done...), offlineClear...)
	clearAll := force && failed == 0
	p.clearDone(clearIDs, clearAll)
	kicked := len(done)
	_, stillAll, remain := p.Status()
	remainN := len(remain)
	if stillAll {
		remainN = -1 // 仍有全员脏标
	}
	logger.Info("peers_apply kicked=%d failed=%d force_all=%v online_only=%v offline_cleared=%d remaining_dirty=%d paced=%v",
		kicked, failed, force, onlineOnly, len(offlineClear), remainN, paced)
	msg := fmt.Sprintf("已踢线 %d 个在线账号以刷新策略", kicked)
	if len(offlineClear) > 0 {
		msg = fmt.Sprintf("已踢线 %d 个在线账号；%d 个离线账号下次连接自动生效", kicked, len(offlineClear))
	}
	if failed > 0 {
		msg = fmt.Sprintf("已踢线 %d 个账号，%d 个失败仍待应用", kicked, failed)
	}
	return &ApplyResult{
		Kicked: kicked, Failed: failed, UserIDs: done, ForceAll: force,
		OnlineOnly: onlineOnly, Message: msg,
	}, nil
}
