package persist

import "time"

// dns_types.go：托管 DNS 领域类型与常量（无 SQL）。

// DNSSourceConfig 表示该行由 server.yaml vpn.dns_servers 种子同步（禁删、禁改 IP、members 锁 all）。
const DNSSourceConfig = "config"

// DNSSourceManual 表示管理端手工新增（可改可删，可绑指定账号或 all）。
const DNSSourceManual = "manual"

// DNSMemberAll 包含集 user_id=0 表示「全部账号」（与 PeerRouteMemberAll 同值）。
const DNSMemberAll int64 = PeerRouteMemberAll

// DNSServer 托管 DNS 定义一行；包含集与排除集分别在 MemberUserIDs / ExcludeUserIDs。
//
// 生效语义：members 命中且不在 excludes → 握手下发给该账号。
// source=config 时 members 须为 [0]；excludes 可配。
type DNSServer struct {
	ID             int64     `json:"id"`
	DNSIP          string    `json:"dns_ip"`
	Remark         string    `json:"remark"`
	Source         string    `json:"source"` // config | manual
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	MemberUserIDs  []int64   `json:"member_user_ids,omitempty"`  // 0=全部
	ExcludeUserIDs []int64   `json:"exclude_user_ids,omitempty"` // 仅 >0
}

// IsConfigSource 是否为 YAML 种子行（只读 IP / 不可删）。
func (d DNSServer) IsConfigSource() bool {
	return d.Source == DNSSourceConfig
}
