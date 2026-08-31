package vpnaccount

import "errors"

// ErrAccountNotFound 表示 users.id 不存在或账号无 VPN 隧道身份。
var ErrAccountNotFound = errors.New("账号不存在或无 VPN 身份")

// ErrPeerRouteNotFound 托管路由 id 不存在。
var ErrPeerRouteNotFound = errors.New("路由不存在")

// ErrViaUserRequired 创建托管路由时未指定 via。
var ErrViaUserRequired = errors.New("须指定 via 账号")

// ErrViaUserNotFound via 账号 id 在 users 表不存在。
var ErrViaUserNotFound = errors.New("via 账号不存在")

// ErrViaNotVPN via 账号无 VPN 身份（不可作出口）。
var ErrViaNotVPN = errors.New("via 须为 VPN 账号")

// ErrPeerAccessArgs 互访删除缺少 user_id/peer_user_id。
var ErrPeerAccessArgs = errors.New("须提供 user_id 与 peer_user_id")
