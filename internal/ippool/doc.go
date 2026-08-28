// Package ippool 管理 VPN 虚拟 IPv4 池分配与释放（与 persist.ip_allocations 同步）。
//
// 上游：vpnaccount、serverapp 启动恢复占用。
// 下游：netutil CIDR 解析；无 DB 直接访问（由 Service 协调 Store）。
// 并发：Pool 持 mutex；Allocate/Release 须与 Store 同事务语义一致。
// 不变量：网关 IP 须 Reserve；AllocateSpecific 用于 fixed 模式与启动恢复。
package ippool
