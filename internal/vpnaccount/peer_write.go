package vpnaccount

import (
	"fmt"

	"haovpn/internal/persist"
)

// CreatePeerRouteInput 新增托管路由的入参（HTTP 解码后传入）。
type CreatePeerRouteInput struct {
	DestCIDR      string
	ViaUserID     int64
	MemberUserIDs []int64
}

// CreatePeerRouteResult 新增成功后的路由 id 与规范化成员列表。
type CreatePeerRouteResult struct {
	ID            int64
	MemberUserIDs []int64
}

// CreatePeerRoute 校验 via 为 VPN 账号、写库并标脏（不踢线；须应用生效）。
//
// 参数：in — DestCIDR / ViaUserID / 成员（可含 PeerRouteMemberAll）。
// 返回：新行 id；领域错误见 ErrVia* / persist 校验错误。
// 关联：api/handler_peer_routes.go createPeerRoute；persist.InsertPeerRoute。
func (p *PeerPolicyApplier) CreatePeerRoute(in CreatePeerRouteInput) (*CreatePeerRouteResult, error) {
	if p == nil || p.Store == nil {
		return nil, fmt.Errorf("peer policy 未初始化")
	}
	if in.ViaUserID <= 0 {
		return nil, ErrViaUserRequired
	}
	via, err := p.Store.GetUserByID(in.ViaUserID)
	if err != nil {
		return nil, err
	}
	if via == nil {
		return nil, ErrViaUserNotFound
	}
	if !via.HasVPN() {
		return nil, ErrViaNotVPN
	}
	id, err := p.Store.InsertPeerRoute(in.DestCIDR, in.ViaUserID, in.MemberUserIDs)
	if err != nil {
		return nil, err
	}
	members := persist.NormalizeMemberUserIDs(in.MemberUserIDs)
	p.MarkMembers(members)
	return &CreatePeerRouteResult{ID: id, MemberUserIDs: members}, nil
}

// DeletePeerRoute 删除托管路由并对其成员标脏。
//
// 返回：删除前快照（供审计）；不存在时 ErrPeerRouteNotFound。
func (p *PeerPolicyApplier) DeletePeerRoute(id int64) (*persist.PeerRoute, error) {
	if p == nil || p.Store == nil {
		return nil, fmt.Errorf("peer policy 未初始化")
	}
	old, err := p.Store.GetPeerRoute(id)
	if err != nil {
		return nil, err
	}
	if old == nil {
		return nil, ErrPeerRouteNotFound
	}
	if err := p.Store.DeletePeerRoute(id); err != nil {
		return nil, err
	}
	p.MarkMembers(old.MemberUserIDs)
	return old, nil
}

// ReplacePeerRouteMembers 替换访问方并标脏（旧∪新）。
//
// 返回：写库后的最新路由；不存在时 ErrPeerRouteNotFound。
func (p *PeerPolicyApplier) ReplacePeerRouteMembers(id int64, memberIDs []int64) (*persist.PeerRoute, error) {
	if p == nil || p.Store == nil {
		return nil, fmt.Errorf("peer policy 未初始化")
	}
	old, err := p.Store.GetPeerRoute(id)
	if err != nil {
		return nil, err
	}
	if old == nil {
		return nil, ErrPeerRouteNotFound
	}
	if err := p.Store.ReplacePeerRouteMembers(id, memberIDs); err != nil {
		return nil, err
	}
	p.MarkMembersUnion(old.MemberUserIDs, memberIDs)
	rt, err := p.Store.GetPeerRoute(id)
	if err != nil {
		return nil, err
	}
	if rt == nil {
		return nil, ErrPeerRouteNotFound
	}
	return rt, nil
}

// AddPeerAccess 双向互访白名单写库并标脏两侧账号。
func (p *PeerPolicyApplier) AddPeerAccess(userID, peerUserID int64) error {
	if p == nil || p.Store == nil {
		return fmt.Errorf("peer policy 未初始化")
	}
	if err := p.Store.AddPeerAccessPair(userID, peerUserID); err != nil {
		return err
	}
	p.MarkUsers(userID, peerUserID)
	return nil
}

// RemovePeerAccess 删除双向互访并标脏。
func (p *PeerPolicyApplier) RemovePeerAccess(userID, peerUserID int64) error {
	if p == nil || p.Store == nil {
		return fmt.Errorf("peer policy 未初始化")
	}
	if userID <= 0 || peerUserID <= 0 {
		return ErrPeerAccessArgs
	}
	if err := p.Store.RemovePeerAccessPair(userID, peerUserID); err != nil {
		return err
	}
	p.MarkUsers(userID, peerUserID)
	return nil
}
