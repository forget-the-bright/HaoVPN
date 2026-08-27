// Package ippool 管理 VPN 虚拟 IPv4 地址池的分配、释放与网关预留。
//
// 与 persist IP 占用表、vpnaccount 开户/握手协同；服务端重启时由 serverapp 恢复占用。
package ippool
