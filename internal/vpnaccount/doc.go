// Package vpnaccount 封装 VPN 账号 IP 模式与 Web 开户/策略变更业务逻辑。
//
// 关键文件：
//   service.go — EnsureVPNIP、DefaultAllowedIPs、租约清理
//   provision.go — Web 开户 ProvisionWebAccount
//   patch.go — ApplyVPNPatch（校验+写库+踢线）
//   enable.go — SetAccountEnabled
//   delete.go — DeleteAccount、PlanVPNPatch
//
// 上游：api（Web 开户/PATCH）、tunnel（握手后 IP 分配）。
// 下游：persist、ippool、security（私钥）；OnKickUser 由 serverapp 注入 sessionmgr。
// 并发：Service 无内部锁；同一账号操作由调用方串行或依赖 SQLite 串行。
// 不变量：IP 分配/回收/PATCH/启禁须经本包；api 不直接 UpdateVPNFields/SetUserEnabled。
package vpnaccount
