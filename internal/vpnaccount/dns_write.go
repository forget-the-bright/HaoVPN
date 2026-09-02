package vpnaccount

import (
	"fmt"

	"haovpn/internal/persist"
)

// dns_write.go：托管 DNS 写用例 + 脏标记（复用 PeerPolicyApplier，应用生效同一套 Kick）。

// CreateDNSServerInput 新增手工托管 DNS 入参。
type CreateDNSServerInput struct {
	DNSIP          string
	Remark         string
	MemberUserIDs  []int64
	ExcludeUserIDs []int64
	ApplyAll       bool // true 时成员强制为全部
}

// CreateDNSServer 写库并标脏（不踢线）。
func (p *PeerPolicyApplier) CreateDNSServer(in CreateDNSServerInput) (*persist.DNSServer, error) {
	if p == nil || p.Store == nil {
		return nil, fmt.Errorf("dns policy 未初始化")
	}
	members := in.MemberUserIDs
	if in.ApplyAll {
		members = []int64{persist.DNSMemberAll}
	}
	id, err := p.Store.CreateDNSServer(in.DNSIP, in.Remark, members, in.ExcludeUserIDs)
	if err != nil {
		return nil, err
	}
	d, err := p.Store.GetDNSServer(id)
	if err != nil || d == nil {
		return nil, fmt.Errorf("创建后读取失败: %w", err)
	}
	// 新建：all 范围 MarkAll（黄条）；指定成员则脏标成员∪排除
	p.markDNSDirtyCreateOrDelete(d.MemberUserIDs, d.ExcludeUserIDs)
	return d, nil
}

// UpdateDNSServerRemark 仅更新备注，不标脏（备注不进握手策略，无需踢线）。
func (p *PeerPolicyApplier) UpdateDNSServerRemark(id int64, remark string) (*persist.DNSServer, error) {
	if p == nil || p.Store == nil {
		return nil, fmt.Errorf("dns policy 未初始化")
	}
	old, err := p.Store.GetDNSServer(id)
	if err != nil {
		return nil, err
	}
	if old == nil {
		return nil, fmt.Errorf("托管 DNS 不存在")
	}
	if err := p.Store.UpdateDNSServerRemark(id, remark); err != nil {
		return nil, err
	}
	return p.Store.GetDNSServer(id)
}

// ReplaceDNSServerMembers 替换包含集（config 禁止）。
func (p *PeerPolicyApplier) ReplaceDNSServerMembers(id int64, memberIDs []int64) (*persist.DNSServer, error) {
	if p == nil || p.Store == nil {
		return nil, fmt.Errorf("dns policy 未初始化")
	}
	old, err := p.Store.GetDNSServer(id)
	if err != nil {
		return nil, err
	}
	if old == nil {
		return nil, fmt.Errorf("托管 DNS 不存在")
	}
	if err := p.Store.ReplaceDNSServerMembers(id, memberIDs); err != nil {
		return nil, err
	}
	neu, err := p.Store.GetDNSServer(id)
	if err != nil || neu == nil {
		return nil, err
	}
	// 成员收窄/扩大：旧∪新；排除名单未改不额外 MarkAll
	p.MarkMembersUnion(old.MemberUserIDs, neu.MemberUserIDs)
	return neu, nil
}

// ReplaceDNSServerExcludes 替换排除集：只脏标排除对称差，禁止因 all 成员 MarkAll。
func (p *PeerPolicyApplier) ReplaceDNSServerExcludes(id int64, excludeIDs []int64) (*persist.DNSServer, error) {
	if p == nil || p.Store == nil {
		return nil, fmt.Errorf("dns policy 未初始化")
	}
	old, err := p.Store.GetDNSServer(id)
	if err != nil {
		return nil, err
	}
	if old == nil {
		return nil, fmt.Errorf("托管 DNS 不存在")
	}
	if err := p.Store.ReplaceDNSServerExcludes(id, excludeIDs); err != nil {
		return nil, err
	}
	neu, err := p.Store.GetDNSServer(id)
	if err != nil || neu == nil {
		return nil, err
	}
	diff := persist.SymmetricDiffUserIDs(old.ExcludeUserIDs, neu.ExcludeUserIDs)
	p.MarkUsers(diff...)
	return neu, nil
}

// DeleteDNSServer 删除手工 DNS 并标脏。
func (p *PeerPolicyApplier) DeleteDNSServer(id int64) (*persist.DNSServer, error) {
	if p == nil || p.Store == nil {
		return nil, fmt.Errorf("dns policy 未初始化")
	}
	old, err := p.Store.DeleteDNSServer(id)
	if err != nil {
		return nil, err
	}
	p.markDNSDirtyCreateOrDelete(old.MemberUserIDs, old.ExcludeUserIDs)
	return old, nil
}

// markDNSDirtyCreateOrDelete：新建/删除时，all 范围 MarkAll；否则 MarkUsers(成员∪排除)。
func (p *PeerPolicyApplier) markDNSDirtyCreateOrDelete(members, excludes []int64) {
	if persist.PeerRouteHasAllMembers(members) {
		p.MarkAll()
		return
	}
	var ids []int64
	ids = append(ids, members...)
	ids = append(ids, excludes...)
	p.MarkUsers(ids...)
}
