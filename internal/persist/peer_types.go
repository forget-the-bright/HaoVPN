package persist

import "time"

// peer_types.go：互访白名单与托管路由的领域类型（无 SQL）。
// 与 peer_access / peer_routes 同包拆分，便于 API 层引用类型而不牵路由实现。

// PeerAccess 账号互访白名单一行：访问方可发往对方当前 VPN IP。
type PeerAccess struct {
	UserID     int64     `json:"user_id"`
	PeerUserID int64     `json:"peer_user_id"`
	CreatedAt  time.Time `json:"created_at"`
}

// PeerRouteMemberAll 访问方 user_id=0 表示「全部账号」。
const PeerRouteMemberAll int64 = 0

// PeerRoute 托管路由定义一行（dest via via_user）；访问方在 MemberUserIDs。
//
// MemberUserIDs 含 0 表示全部；解析策略时若存在 0 则忽略同路由下其它指定。
type PeerRoute struct {
	ID            int64     `json:"id"`
	DestCIDR      string    `json:"dest_cidr"`
	ViaUserID     int64     `json:"via_user_id"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	MemberUserIDs []int64   `json:"member_user_ids,omitempty"` // 0=全部
}
